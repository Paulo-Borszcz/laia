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

const maxConversationTurns = 50

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
	if len(turns) > maxConversationTurns {
		turns = turns[len(turns)-maxConversationTurns:]
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(turns)
		if err != nil {
			return err
		}
		return tx.Bucket(conversationsBucket).Put([]byte(phone), data)
	})
}

func (s *BoltStore) ClearHistory(phone string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(conversationsBucket).Delete([]byte(phone))
	})
}

func (s *BoltStore) Close() error {
	return s.db.Close()
}
