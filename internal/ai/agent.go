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
func (a *Agent) Handle(ctx context.Context, user *store.User, phone, text string) (string, error) {
	if !a.allowRequest(phone) {
		return "Você está enviando mensagens muito rápido. Aguarde um minuto e tente novamente.", nil
	}

	history, err := a.store.GetHistory(phone)
	if err != nil {
		log.Printf("agent: failed to load history for %s: %v", phone, err)
	}

	sessionToken, err := a.glpi.InitSession(user.UserToken)
	if err != nil {
		return "", fmt.Errorf("initSession: %w", err)
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

	for range maxToolIterations {
		resp, err := a.chatCompletion(ctx, messages, toolsAny)
		if err != nil {
			return "", fmt.Errorf("chatCompletion: %w", err)
		}

		if len(resp.Choices) == 0 {
			return "Desculpe, não consegui processar sua mensagem. Tente novamente.", nil
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
			return responseText, nil
		}

		for _, tc := range msg.ToolCalls {
			var args map[string]any
			json.Unmarshal([]byte(tc.Function.Arguments), &args)

			tool, toolErr := registry.Get(tc.Function.Name)
			var result map[string]any
			if toolErr != nil {
				result = map[string]any{"error": toolErr.Error()}
			} else {
				log.Printf("agent: calling tool %s for %s", tc.Function.Name, phone)
				result, toolErr = tool.Execute(ctx, args)
				if toolErr != nil {
					result = map[string]any{"error": toolErr.Error()}
				}
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
	return "Desculpe, a operação ficou complexa demais. Tente reformular seu pedido.", nil
}

func (a *Agent) chatCompletion(ctx context.Context, messages []chatMessage, tools []any) (*chatResponse, error) {
	reqBody := chatRequest{
		Model:       openAIModel,
		Messages:    messages,
		Temperature: 0.3,
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", openAIEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
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
func toOpenAIMessages(turns []store.ConversationTurn) []chatMessage {
	for _, t := range turns {
		if t.Role == "model" {
			return nil
		}
	}

	var messages []chatMessage
	for _, t := range turns {
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
				if p.FunctionResponse != nil {
					resultJSON, _ := json.Marshal(p.FunctionResponse.Response)
					messages = append(messages, chatMessage{
						Role:       "tool",
						Content:    string(resultJSON),
						ToolCallID: p.FunctionResponse.ToolCallID,
					})
				}
			}
		}
	}
	return messages
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
