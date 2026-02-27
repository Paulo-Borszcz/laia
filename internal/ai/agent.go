package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/lojasmm/laia/internal/glpi"
	"github.com/lojasmm/laia/internal/store"
)

const (
	maxToolIterations = 5
	rateLimitWindow   = time.Minute
	rateLimitMax      = 10
	openAIModel       = "gpt-4.1-mini"
	openAIEndpoint    = "https://api.openai.com/v1/chat/completions"
	maxTokens         = 2048

	// Retry settings (exponential backoff, inspired by opencode)
	retryMaxAttempts  = 3
	retryInitialDelay = 2 * time.Second
	retryMaxDelay     = 30 * time.Second

	// History pruning: tool responses older than this many turns get compressed
	pruneKeepRecent = 4

	// Doom loop: max consecutive calls to the same tool with same args
	doomLoopThreshold = 2
)

// RegistryBuilder creates a tool registry for a given GLPI session.
type RegistryBuilder func(g *glpi.Client, sessionToken string, userID int) *Registry

type Agent struct {
	apiKey   string
	glpi     *glpi.Client
	store    store.Store
	buildReg RegistryBuilder
	http     *http.Client

	mu       sync.Mutex
	counters map[string]*rateBucket
}

type rateBucket struct {
	count  int
	window time.Time
}

func NewAgent(apiKey string, g *glpi.Client, s store.Store, buildReg RegistryBuilder) *Agent {
	return &Agent{
		apiKey:   apiKey,
		glpi:     g,
		store:    s,
		buildReg: buildReg,
		http:     &http.Client{Timeout: 60 * time.Second},
		counters: make(map[string]*rateBucket),
	}
}

// --- OpenAI API types ---

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []any         `json:"tools,omitempty"`
	Temperature float32       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

// Handle processes one user message through the AI agent loop.
func (a *Agent) Handle(ctx context.Context, user *store.User, phone, text string) (*Response, error) {
	if !a.allowRequest(phone) {
		return &Response{Text: "Você está enviando mensagens muito rápido. Aguarde um minuto e tente novamente."}, nil
	}

	history, err := a.store.GetHistory(phone)
	if err != nil {
		log.Printf("agent: failed to load history for %s: %v", phone, err)
	}

	sessionToken, err := a.glpi.InitSession(user.UserToken)
	if err != nil {
		return nil, fmt.Errorf("initSession: %w", err)
	}
	defer a.glpi.KillSession(sessionToken)

	registry := a.buildReg(a.glpi, sessionToken, user.GLPIUserID)

	messages := []chatMessage{{
		Role:    "system",
		Content: BuildSystemPrompt(user.Name, user.GLPIUserID),
	}}
	messages = append(messages, toOpenAIMessages(history)...)
	messages = append(messages, chatMessage{Role: "user", Content: text})

	var allTurns []store.ConversationTurn
	allTurns = append(allTurns, history...)
	allTurns = append(allTurns, store.ConversationTurn{
		Role:  "user",
		Parts: []store.TurnPart{{Text: text}},
	})

	tools := registry.OpenAITools()

	// Convert to []any for JSON serialization
	toolsAny := make([]any, len(tools))
	for i, t := range tools {
		toolsAny[i] = t
	}

	// Doom loop detection: track consecutive identical tool calls
	var lastToolSig string
	var sameToolCount int

	for range maxToolIterations {
		resp, err := a.chatCompletion(ctx, messages, toolsAny)
		if err != nil {
			return nil, fmt.Errorf("chatCompletion: %w", err)
		}

		if len(resp.Choices) == 0 {
			return &Response{Text: "Desculpe, não consegui processar sua mensagem. Tente novamente."}, nil
		}

		msg := resp.Choices[0].Message
		allTurns = append(allTurns, messageToTurn(msg))
		messages = append(messages, msg)

		if len(msg.ToolCalls) == 0 {
			responseText := msg.Content
			if responseText == "" {
				responseText = "Desculpe, não consegui gerar uma resposta."
			}
			a.saveHistory(phone, allTurns)
			return &Response{Text: responseText}, nil
		}

		for _, tc := range msg.ToolCalls {
			// Intercept respond_interactive pseudo-tool
			if tc.Function.Name == "respond_interactive" {
				var args map[string]any
				json.Unmarshal([]byte(tc.Function.Arguments), &args)
				r := parseInteractiveResponse(args)
				a.saveHistory(phone, allTurns)
				return r, nil
			}

			// Doom loop check
			sig := tc.Function.Name + ":" + tc.Function.Arguments
			if sig == lastToolSig {
				sameToolCount++
			} else {
				lastToolSig = sig
				sameToolCount = 1
			}
			if sameToolCount > doomLoopThreshold {
				log.Printf("agent: doom loop detected for tool %s (%s)", tc.Function.Name, phone)
				a.saveHistory(phone, allTurns)
				return &Response{Text: "Desculpe, encontrei um problema ao processar sua solicitação. Tente novamente ou reformule seu pedido."}, nil
			}

			var args map[string]any
			json.Unmarshal([]byte(tc.Function.Arguments), &args)

			log.Printf("agent: calling tool %s for %s", tc.Function.Name, phone)
			result, toolErr := registry.ExecuteTool(ctx, tc.Function.Name, args)
			if toolErr != nil {
				result = map[string]any{"error": toolErr.Error()}
			}

			resultJSON, _ := json.Marshal(result)
			messages = append(messages, chatMessage{
				Role:       "tool",
				Content:    string(resultJSON),
				ToolCallID: tc.ID,
			})
			allTurns = append(allTurns, store.ConversationTurn{
				Role: "tool",
				Parts: []store.TurnPart{{
					FunctionResponse: &store.FunctionRespPart{
						ToolCallID: tc.ID,
						Name:       tc.Function.Name,
						Response:   result,
					},
				}},
			})
		}
	}

	a.saveHistory(phone, allTurns)
	return &Response{Text: "Desculpe, a operação ficou complexa demais. Tente reformular seu pedido."}, nil
}

// parseInteractiveResponse converts respond_interactive tool args into a Response.
func parseInteractiveResponse(args map[string]any) *Response {
	resp := &Response{}

	if text, ok := args["text"].(string); ok {
		resp.Text = text
	}

	msgType, _ := args["message_type"].(string)

	switch msgType {
	case "buttons":
		if buttons, ok := args["buttons"].([]any); ok {
			for _, b := range buttons {
				btn, ok := b.(map[string]any)
				if !ok {
					continue
				}
				id, _ := btn["id"].(string)
				title, _ := btn["title"].(string)
				resp.Buttons = append(resp.Buttons, ButtonOption{ID: id, Title: title})
			}
		}
	case "list":
		list := &ListOption{}
		if bt, ok := args["list_button_text"].(string); ok {
			list.ButtonText = bt
		}
		if list.ButtonText == "" {
			list.ButtonText = "Ver opções"
		}
		if sections, ok := args["sections"].([]any); ok {
			for _, s := range sections {
				sec, ok := s.(map[string]any)
				if !ok {
					continue
				}
				section := ListSection{}
				section.Title, _ = sec["title"].(string)
				if rows, ok := sec["rows"].([]any); ok {
					for _, r := range rows {
						row, ok := r.(map[string]any)
						if !ok {
							continue
						}
						lr := ListRow{}
						lr.ID, _ = row["id"].(string)
						lr.Title, _ = row["title"].(string)
						lr.Description, _ = row["description"].(string)
						section.Rows = append(section.Rows, lr)
					}
				}
				list.Sections = append(list.Sections, section)
			}
		}
		resp.List = list
	}

	return resp
}

// retryableStatus returns true for HTTP status codes worth retrying.
func retryableStatus(code int) bool {
	return code == 429 || code == 500 || code == 502 || code == 503
}

func (a *Agent) chatCompletion(ctx context.Context, messages []chatMessage, tools []any) (*chatResponse, error) {
	reqBody := chatRequest{
		Model:       openAIModel,
		Messages:    messages,
		Temperature: 0.3,
		MaxTokens:   maxTokens,
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	delay := retryInitialDelay
	var lastErr error

	for attempt := range retryMaxAttempts {
		req, err := http.NewRequestWithContext(ctx, "POST", openAIEndpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+a.apiKey)

		resp, err := a.http.Do(req)
		if err != nil {
			lastErr = err
			if attempt < retryMaxAttempts-1 {
				log.Printf("agent: request error (attempt %d/%d): %v", attempt+1, retryMaxAttempts, err)
				time.Sleep(delay)
				delay = min(delay*2, retryMaxDelay)
				continue
			}
			return nil, err
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if retryableStatus(resp.StatusCode) && attempt < retryMaxAttempts-1 {
			lastErr = fmt.Errorf("openai: status %d: %s", resp.StatusCode, string(respBody))
			log.Printf("agent: retryable error (attempt %d/%d): %v", attempt+1, retryMaxAttempts, lastErr)
			time.Sleep(delay)
			delay = min(delay*2, retryMaxDelay)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("openai: status %d: %s", resp.StatusCode, string(respBody))
		}

		var chatResp chatResponse
		if err := json.Unmarshal(respBody, &chatResp); err != nil {
			return nil, fmt.Errorf("openai: unmarshal: %w", err)
		}

		return &chatResp, nil
	}

	return nil, fmt.Errorf("openai: max retries exceeded: %w", lastErr)
}

func (a *Agent) allowRequest(phone string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	b, ok := a.counters[phone]
	if !ok || now.Sub(b.window) > rateLimitWindow {
		a.counters[phone] = &rateBucket{count: 1, window: now}
		return true
	}
	if b.count >= rateLimitMax {
		return false
	}
	b.count++
	return true
}

func (a *Agent) saveHistory(phone string, turns []store.ConversationTurn) {
	if err := a.store.SaveHistory(phone, turns); err != nil {
		log.Printf("agent: failed to save history for %s: %v", phone, err)
	}
}

// --- conversion helpers ---

// toOpenAIMessages converts stored conversation turns to OpenAI chat messages.
// Drops incompatible old Gemini-format history (role "model").
// Compresses old tool responses to save tokens (keeps only recent ones full).
func toOpenAIMessages(turns []store.ConversationTurn) []chatMessage {
	for _, t := range turns {
		if t.Role == "model" {
			return nil
		}
	}

	var messages []chatMessage
	for i, t := range turns {
		turnsFromEnd := len(turns) - i

		switch t.Role {
		case "user":
			for _, p := range t.Parts {
				if p.Text != "" {
					messages = append(messages, chatMessage{Role: "user", Content: p.Text})
				}
			}
		case "assistant":
			msg := chatMessage{Role: "assistant"}
			for _, p := range t.Parts {
				if p.Text != "" {
					msg.Content = p.Text
				}
				if p.FunctionCall != nil {
					argsJSON, _ := json.Marshal(p.FunctionCall.Args)
					msg.ToolCalls = append(msg.ToolCalls, toolCall{
						ID:   p.FunctionCall.ID,
						Type: "function",
						Function: functionCall{
							Name:      p.FunctionCall.Name,
							Arguments: string(argsJSON),
						},
					})
				}
			}
			messages = append(messages, msg)
		case "tool":
			for _, p := range t.Parts {
				if p.FunctionResponse == nil {
					continue
				}
				content := ""
				if turnsFromEnd <= pruneKeepRecent {
					// Recent: keep full response
					resultJSON, _ := json.Marshal(p.FunctionResponse.Response)
					content = string(resultJSON)
				} else {
					// Old: compress to just tool name + status
					content = compressToolResponse(p.FunctionResponse)
				}
				messages = append(messages, chatMessage{
					Role:       "tool",
					Content:    content,
					ToolCallID: p.FunctionResponse.ToolCallID,
				})
			}
		}
	}
	return messages
}

// compressToolResponse reduces an old tool response to a short summary to save tokens.
func compressToolResponse(resp *store.FunctionRespPart) string {
	if errMsg, ok := resp.Response["error"]; ok {
		return fmt.Sprintf(`{"tool":"%s","status":"error","error":"%v"}`, resp.Name, errMsg)
	}
	// Summarize: keep key counts/IDs but drop full content
	summary := map[string]any{"tool": resp.Name, "status": "ok"}
	if total, ok := resp.Response["total"]; ok {
		summary["total"] = total
	}
	if id, ok := resp.Response["id"]; ok {
		summary["id"] = id
	}
	if msg, ok := resp.Response["mensagem"]; ok {
		summary["mensagem"] = msg
	}
	out, _ := json.Marshal(summary)
	return string(out)
}

func messageToTurn(msg chatMessage) store.ConversationTurn {
	turn := store.ConversationTurn{Role: msg.Role}
	if msg.Content != "" {
		turn.Parts = append(turn.Parts, store.TurnPart{Text: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		var args map[string]any
		json.Unmarshal([]byte(tc.Function.Arguments), &args)
		turn.Parts = append(turn.Parts, store.TurnPart{
			FunctionCall: &store.FunctionCallPart{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			},
		})
	}
	return turn
}
