package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/lojasmm/laia/internal/glpi"
	"github.com/lojasmm/laia/internal/store"
	"google.golang.org/genai"
)

const (
	maxToolIterations = 5
	rateLimitWindow   = time.Minute
	rateLimitMax      = 10
	geminiModel       = "gemini-2.5-flash"
)

// RegistryBuilder creates a tool registry for a given GLPI session.
type RegistryBuilder func(g *glpi.Client, sessionToken string, userID int) *Registry

type Agent struct {
	client   *genai.Client
	glpi     *glpi.Client
	store    store.Store
	buildReg RegistryBuilder

	mu       sync.Mutex
	counters map[string]*rateBucket
}

type rateBucket struct {
	count  int
	window time.Time
}

func NewAgent(client *genai.Client, g *glpi.Client, s store.Store, buildReg RegistryBuilder) *Agent {
	return &Agent{
		client:   client,
		glpi:     g,
		store:    s,
		buildReg: buildReg,
		counters: make(map[string]*rateBucket),
	}
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

	genaiHistory := toGenaiContents(history)

	config := &genai.GenerateContentConfig{
		Tools:             registry.GenaiTools(),
		Temperature:       ptrFloat(0.3),
		SystemInstruction: BuildSystemPrompt(user.Name, user.GLPIUserID),
	}

	// Append user message
	userContent := genai.NewContentFromText(text, genai.RoleUser)
	genaiHistory = append(genaiHistory, userContent)

	var allTurns []store.ConversationTurn
	allTurns = append(allTurns, history...)
	allTurns = append(allTurns, store.ConversationTurn{
		Role:  "user",
		Parts: []store.TurnPart{{Text: text}},
	})

	// Agent loop
	for range maxToolIterations {
		resp, err := a.client.Models.GenerateContent(ctx, geminiModel, genaiHistory, config)
		if err != nil {
			return "", fmt.Errorf("generateContent: %w", err)
		}

		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			return "Desculpe, não consegui processar sua mensagem. Tente novamente.", nil
		}

		modelContent := resp.Candidates[0].Content

		// Record model turn
		modelTurn := contentToTurn(modelContent)
		allTurns = append(allTurns, modelTurn)
		genaiHistory = append(genaiHistory, modelContent)

		// Check for function calls
		functionCalls := extractFunctionCalls(modelContent)
		if len(functionCalls) == 0 {
			// No tool calls — extract text response and return
			responseText := extractText(modelContent)
			if responseText == "" {
				responseText = "Desculpe, não consegui gerar uma resposta."
			}
			a.saveHistory(phone, allTurns)
			return responseText, nil
		}

		// Execute tool calls and build response
		funcParts := make([]*genai.Part, 0, len(functionCalls))
		var funcTurnParts []store.TurnPart

		for _, fc := range functionCalls {
			tool, toolErr := registry.Get(fc.Name)
			var result map[string]any
			if toolErr != nil {
				result = map[string]any{"error": toolErr.Error()}
			} else {
				log.Printf("agent: calling tool %s for %s", fc.Name, phone)
				result, toolErr = tool.Execute(ctx, fc.Args)
				if toolErr != nil {
					result = map[string]any{"error": toolErr.Error()}
				}
			}

			funcParts = append(funcParts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     fc.Name,
					Response: result,
				},
			})
			funcTurnParts = append(funcTurnParts, store.TurnPart{
				FunctionResponse: &store.FunctionRespPart{
					Name:     fc.Name,
					Response: result,
				},
			})
		}

		// Record function response turn
		funcContent := &genai.Content{
			Role:  "user",
			Parts: funcParts,
		}
		allTurns = append(allTurns, store.ConversationTurn{
			Role:  "user",
			Parts: funcTurnParts,
		})
		genaiHistory = append(genaiHistory, funcContent)
	}

	a.saveHistory(phone, allTurns)
	return "Desculpe, a operação ficou complexa demais. Tente reformular seu pedido.", nil
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

func toGenaiContents(turns []store.ConversationTurn) []*genai.Content {
	contents := make([]*genai.Content, 0, len(turns))
	for _, t := range turns {
		c := &genai.Content{Role: t.Role}
		for _, p := range t.Parts {
			if p.Text != "" {
				c.Parts = append(c.Parts, genai.NewPartFromText(p.Text))
			}
			if p.FunctionCall != nil {
				c.Parts = append(c.Parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						Name: p.FunctionCall.Name,
						Args: p.FunctionCall.Args,
					},
				})
			}
			if p.FunctionResponse != nil {
				c.Parts = append(c.Parts, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						Name:     p.FunctionResponse.Name,
						Response: p.FunctionResponse.Response,
					},
				})
			}
		}
		if len(c.Parts) > 0 {
			contents = append(contents, c)
		}
	}
	return contents
}

func contentToTurn(c *genai.Content) store.ConversationTurn {
	turn := store.ConversationTurn{Role: c.Role}
	for _, p := range c.Parts {
		if p.Text != "" {
			turn.Parts = append(turn.Parts, store.TurnPart{Text: p.Text})
		}
		if p.FunctionCall != nil {
			// Marshal args to map[string]any for storage
			args := make(map[string]any)
			if p.FunctionCall.Args != nil {
				raw, _ := json.Marshal(p.FunctionCall.Args)
				json.Unmarshal(raw, &args)
			}
			turn.Parts = append(turn.Parts, store.TurnPart{
				FunctionCall: &store.FunctionCallPart{
					Name: p.FunctionCall.Name,
					Args: args,
				},
			})
		}
	}
	return turn
}

func extractFunctionCalls(c *genai.Content) []*genai.FunctionCall {
	var calls []*genai.FunctionCall
	for _, p := range c.Parts {
		if p.FunctionCall != nil {
			calls = append(calls, p.FunctionCall)
		}
	}
	return calls
}

func extractText(c *genai.Content) string {
	for _, p := range c.Parts {
		if p.Text != "" {
			return p.Text
		}
	}
	return ""
}

func ptrFloat(f float32) *float32 { return &f }
