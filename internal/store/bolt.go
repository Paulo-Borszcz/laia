package store

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	usersBucket         = []byte("users")
	conversationsBucket = []byte("conversations")
)

const (
	maxConversationTurns = 50
	// Token budget for conversation history (leaves room for system prompt + output).
	// Estimated via len(text)/3.5 heuristic for multilingual content.
	maxHistoryTokens = 3500
)

// TurnPart represents a single part of a conversation turn (text or function call/response).
type TurnPart struct {
	Text             string            `json:"text,omitempty"`
	FunctionCall     *FunctionCallPart `json:"function_call,omitempty"`
	FunctionResponse *FunctionRespPart `json:"function_response,omitempty"`
}

type FunctionCallPart struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type FunctionRespPart struct {
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name"`
	Response   map[string]any `json:"response"`
}

// ConversationTurn is one message in the conversation (user or model).
type ConversationTurn struct {
	Role  string     `json:"role"`
	Parts []TurnPart `json:"parts"`
}

type User struct {
	Phone           string    `json:"phone"`
	UserToken       string    `json:"user_token"`
	GLPIUserID      int       `json:"glpi_user_id"`
	Name            string    `json:"name"`
	AuthenticatedAt time.Time `json:"authenticated_at"`
}

type Store interface {
	SaveUser(u User) error
	GetUser(phone string) (*User, error)
	DeleteUser(phone string) error
	GetHistory(phone string) ([]ConversationTurn, error)
	SaveHistory(phone string, turns []ConversationTurn) error
	ClearHistory(phone string) error
	Close() error
}

type BoltStore struct {
	db *bolt.DB
}

func NewBoltStore(path string) (*BoltStore, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening bolt db: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(usersBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(conversationsBucket)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("creating users bucket: %w", err)
	}

	return &BoltStore{db: db}, nil
}

func (s *BoltStore) SaveUser(u User) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(u)
		if err != nil {
			return err
		}
		return tx.Bucket(usersBucket).Put([]byte(u.Phone), data)
	})
}

func (s *BoltStore) GetUser(phone string) (*User, error) {
	var u User
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(usersBucket).Get([]byte(phone))
		if v == nil {
			return nil
		}
		return json.Unmarshal(v, &u)
	})
	if err != nil {
		return nil, err
	}
	if u.Phone == "" {
		return nil, nil
	}
	return &u, nil
}

func (s *BoltStore) DeleteUser(phone string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(usersBucket).Delete([]byte(phone))
	})
}

func (s *BoltStore) GetHistory(phone string) ([]ConversationTurn, error) {
	var turns []ConversationTurn
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(conversationsBucket).Get([]byte(phone))
		if v == nil {
			return nil
		}
		return json.Unmarshal(v, &turns)
	})
	return turns, err
}

func (s *BoltStore) SaveHistory(phone string, turns []ConversationTurn) error {
	// Hard cap to prevent unbounded growth
	if len(turns) > maxConversationTurns {
		turns = turns[len(turns)-maxConversationTurns:]
	}

	// Compress old tool responses before token pruning
	for i := range turns {
		if i >= len(turns)-4 {
			break // keep recent turns untouched
		}
		compressTurnToolResponses(&turns[i])
	}

	// Token-aware pruning: drop oldest turns until under budget
	for len(turns) > 2 && estimateTokens(turns) > maxHistoryTokens {
		turns = turns[1:]
	}

	// Trimming may leave orphaned tool turns at the start (their matching
	// assistant with tool_calls was cut). Drop them to avoid OpenAI 400 errors.
	for len(turns) > 0 && turns[0].Role == "tool" {
		turns = turns[1:]
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(turns)
		if err != nil {
			return err
		}
		return tx.Bucket(conversationsBucket).Put([]byte(phone), data)
	})
}

// estimateTokens approximates token count for multilingual text.
// Uses len/3.5 heuristic with 10% overhead for JSON structure.
func estimateTokens(turns []ConversationTurn) int {
	total := 0
	for _, t := range turns {
		for _, p := range t.Parts {
			total += len(p.Text)
			if p.FunctionCall != nil {
				data, _ := json.Marshal(p.FunctionCall)
				total += len(data)
			}
			if p.FunctionResponse != nil {
				data, _ := json.Marshal(p.FunctionResponse)
				total += len(data)
			}
		}
	}
	return int(float64(total) / 3.5 * 1.1)
}

// compressTurnToolResponses strips verbose fields from old tool responses to save tokens
// while preserving entity identifiers (id, name, status) in list items.
func compressTurnToolResponses(turn *ConversationTurn) {
	for i, p := range turn.Parts {
		if p.FunctionResponse == nil {
			continue
		}
		resp := p.FunctionResponse.Response

		// Truncate long text fields
		for _, key := range []string{"descricao", "conteudo", "preview", "answer", "content"} {
			if v, ok := resp[key].(string); ok && len(v) > 100 {
				resp[key] = v[:100] + "…[truncado]"
			}
		}

		// Compress list items to {id, nome/titulo, status} only
		for _, listKey := range []string{"chamados", "ativos", "artigos", "tarefas", "comentarios", "categorias", "departamentos", "historico"} {
			items, ok := resp[listKey]
			if !ok {
				continue
			}
			switch v := items.(type) {
			case []any:
				compressed := make([]any, 0, len(v))
				for _, raw := range v {
					if item, ok := raw.(map[string]any); ok {
						c := map[string]any{}
						if id, ok := item["id"]; ok {
							c["id"] = id
						}
						for _, nameKey := range []string{"nome", "titulo"} {
							if name, ok := item[nameKey]; ok {
								c[nameKey] = name
								break
							}
						}
						if status, ok := item["status"]; ok {
							c["status"] = status
						}
						compressed = append(compressed, c)
					}
				}
				if len(compressed) > 0 {
					resp[listKey] = compressed
				}
			}
		}

		turn.Parts[i].FunctionResponse.Response = resp
	}
}

func (s *BoltStore) ClearHistory(phone string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(conversationsBucket).Delete([]byte(phone))
	})
}

func (s *BoltStore) Close() error {
	return s.db.Close()
}
