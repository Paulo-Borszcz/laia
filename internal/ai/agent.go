package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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

	// Doom loop: exact-match threshold and per-tool-name threshold
	doomLoopExactThreshold = 2
	doomLoopNameThreshold  = 4

	// Incremental history pruning: max attempts before full clear
	maxPruneAttempts = 3

	// Proactive token budget for messages before sending to OpenAI.
	// Leaves room for system prompt (~1500 tokens) + output (maxTokens).
	maxMessageTokenBudget = 6000
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
	Usage   *usageInfo   `json:"usage,omitempty"`
}

type usageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
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

	// Doom loop detection state
	var lastToolSig string
	var sameExactCount int
	toolNameCounts := map[string]int{} // track per-tool-name call count
	var pruneAttempt int

	for range maxToolIterations {
		// Proactive token budget check: drop oldest non-system turns if too large
		estimated := estimateMessagesTokens(messages)
		if estimated > maxMessageTokenBudget {
			log.Printf("agent: proactive prune for %s (estimated %d tokens > %d budget)", phone, estimated, maxMessageTokenBudget)
			messages = pruneMessages(messages, user.Name, user.GLPIUserID)
			allTurns = rebuildTurns(messages)
		}

		resp, err := a.chatCompletion(ctx, messages, toolsAny)
		if err != nil {
			errMsg := err.Error()
			isContextOverflow := strings.Contains(errMsg, "context_length_exceeded") ||
				strings.Contains(errMsg, "maximum context length")
			is400 := strings.Contains(errMsg, "status 400")

			if (is400 || isContextOverflow) && pruneAttempt < maxPruneAttempts {
				pruneAttempt++
				// Context overflow: more aggressive pruning (drop 2x attempt turns)
				dropCount := pruneAttempt
				if isContextOverflow {
					dropCount = pruneAttempt * 2
				}
				log.Printf("agent: format error for %s, pruning attempt %d/%d (dropping %d turns, overflow=%v)",
					phone, pruneAttempt, maxPruneAttempts, dropCount, isContextOverflow)
				for range dropCount {
					if len(allTurns) > 1 {
						allTurns = allTurns[1:]
					}
				}
				messages = []chatMessage{{
					Role:    "system",
					Content: BuildSystemPrompt(user.Name, user.GLPIUserID),
				}}
				messages = append(messages, toOpenAIMessages(allTurns)...)
				continue
			}
			// Last resort: clear everything
			if is400 || isContextOverflow {
				log.Printf("agent: incremental prune failed for %s, clearing history", phone)
				a.store.ClearHistory(phone)
				messages = []chatMessage{
					{Role: "system", Content: BuildSystemPrompt(user.Name, user.GLPIUserID)},
					{Role: "user", Content: text},
				}
				allTurns = []store.ConversationTurn{
					{Role: "user", Parts: []store.TurnPart{{Text: text}}},
				}
				continue
			}
			return nil, fmt.Errorf("chatCompletion: %w", err)
		}
		pruneAttempt = 0

		// Log actual token usage from API response
		if resp.Usage != nil {
			log.Printf("agent: tokens for %s — prompt=%d completion=%d total=%d",
				phone, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
		}

		if len(resp.Choices) == 0 {
			return &Response{Text: "Não recebi resposta do sistema de IA. Tente novamente em alguns segundos."}, nil
		}

		msg := resp.Choices[0].Message
		allTurns = append(allTurns, messageToTurn(msg))
		messages = append(messages, msg)

		if len(msg.ToolCalls) == 0 {
			responseText := msg.Content
			if responseText == "" {
				responseText = "Não consegui formular uma resposta. Pode repetir ou reformular sua pergunta?"
			}
			a.saveHistory(phone, allTurns)
			return &Response{Text: responseText}, nil
		}

		// Check for respond_interactive first (returns immediately)
		for _, tc := range msg.ToolCalls {
			if tc.Function.Name == "respond_interactive" {
				var args map[string]any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					log.Printf("agent: invalid JSON from respond_interactive for %s: %v", phone, err)
					args = map[string]any{"text": "Desculpe, houve um erro ao montar a resposta. Tente novamente."}
				}
				r := parseInteractiveResponse(args)
				allTurns = append(allTurns, store.ConversationTurn{
					Role: "tool",
					Parts: []store.TurnPart{{
						FunctionResponse: &store.FunctionRespPart{
							ToolCallID: tc.ID,
							Name:       tc.Function.Name,
							Response:   map[string]any{"status": "delivered"},
						},
					}},
				})
				a.saveHistory(phone, allTurns)
				return r, nil
			}
		}

		// Doom loop checks before executing tools
		for _, tc := range msg.ToolCalls {
			sig := tc.Function.Name + ":" + tc.Function.Arguments
			if sig == lastToolSig {
				sameExactCount++
			} else {
				lastToolSig = sig
				sameExactCount = 1
			}
			toolNameCounts[tc.Function.Name]++

			if sameExactCount > doomLoopExactThreshold || toolNameCounts[tc.Function.Name] > doomLoopNameThreshold {
				log.Printf("agent: doom loop detected for tool %s (exact=%d, name=%d) (%s)",
					tc.Function.Name, sameExactCount, toolNameCounts[tc.Function.Name], phone)
				a.saveHistory(phone, allTurns)
				return &Response{Text: fmt.Sprintf("A ferramenta %s travou em um loop. Tente reformular seu pedido ou dividir em perguntas menores.", tc.Function.Name)}, nil
			}
		}

		// Execute tools — parallel if all are read-only, sequential otherwise
		allReadOnly := true
		for _, tc := range msg.ToolCalls {
			if !registry.IsReadOnly(tc.Function.Name) {
				allReadOnly = false
				break
			}
		}

		if allReadOnly && len(msg.ToolCalls) > 1 {
			// Parallel execution for read-only tools
			type toolResult struct {
				idx    int
				tc     toolCall
				result map[string]any
			}
			results := make([]toolResult, len(msg.ToolCalls))
			var wg sync.WaitGroup
			for i, tc := range msg.ToolCalls {
				wg.Add(1)
				go func(i int, tc toolCall) {
					defer wg.Done()
					var args map[string]any
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						log.Printf("agent: invalid JSON args for %s: %v", tc.Function.Name, err)
						results[i] = toolResult{idx: i, tc: tc, result: map[string]any{
							"status": "error",
							"error":  map[string]any{"type": string(ErrValidation), "message": fmt.Sprintf("Argumentos inválidos para %s. Verifique e tente novamente.", tc.Function.Name)},
						}}
						return
					}
					log.Printf("agent: calling tool %s (parallel) for %s", tc.Function.Name, phone)
					result, toolErr := registry.ExecuteTool(ctx, tc.Function.Name, args)
					if toolErr != nil {
						te := ClassifyError(toolErr)
						// Retry once if retryable
						if te.Retryable {
							log.Printf("agent: retrying tool %s for %s after error: %v", tc.Function.Name, phone, te.RawError)
							time.Sleep(2 * time.Second)
							result, toolErr = registry.ExecuteTool(ctx, tc.Function.Name, args)
							if toolErr != nil {
								te = ClassifyError(toolErr)
							}
						}
						if toolErr != nil {
							result = map[string]any{
								"status": "error",
								"error":  map[string]any{"type": string(te.Type), "message": te.Message},
							}
						}
					}
					results[i] = toolResult{idx: i, tc: tc, result: result}
				}(i, tc)
			}
			wg.Wait()

			// Check for auth errors — abort early and trigger re-auth
			for _, r := range results {
				if errMap, ok := r.result["error"].(map[string]any); ok {
					if errMap["type"] == string(ErrAuth) {
						log.Printf("agent: auth error in parallel tool %s for %s", r.tc.Function.Name, phone)
						a.saveHistory(phone, allTurns)
						return nil, fmt.Errorf("auth_error: %v", errMap["message"])
					}
				}
			}

			for _, r := range results {
				resultJSON, _ := json.Marshal(r.result)
				messages = append(messages, chatMessage{
					Role: "tool", Content: string(resultJSON), ToolCallID: r.tc.ID,
				})
				allTurns = append(allTurns, store.ConversationTurn{
					Role: "tool",
					Parts: []store.TurnPart{{
						FunctionResponse: &store.FunctionRespPart{
							ToolCallID: r.tc.ID, Name: r.tc.Function.Name, Response: r.result,
						},
					}},
				})
			}
		} else {
			// Sequential execution (mutating tools or single call)
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					log.Printf("agent: invalid JSON args for %s: %v", tc.Function.Name, err)
					errResult := map[string]any{
						"status": "error",
						"error":  map[string]any{"type": string(ErrValidation), "message": fmt.Sprintf("Argumentos inválidos para %s. Verifique e tente novamente.", tc.Function.Name)},
					}
					resultJSON, _ := json.Marshal(errResult)
					messages = append(messages, chatMessage{
						Role: "tool", Content: string(resultJSON), ToolCallID: tc.ID,
					})
					allTurns = append(allTurns, store.ConversationTurn{
						Role: "tool",
						Parts: []store.TurnPart{{
							FunctionResponse: &store.FunctionRespPart{
								ToolCallID: tc.ID, Name: tc.Function.Name, Response: errResult,
							},
						}},
					})
					continue
				}

				log.Printf("agent: calling tool %s for %s", tc.Function.Name, phone)
				result, toolErr := registry.ExecuteTool(ctx, tc.Function.Name, args)
				if toolErr != nil {
					te := ClassifyError(toolErr)
					// Retry once if retryable
					if te.Retryable {
						log.Printf("agent: retrying tool %s for %s after error: %v", tc.Function.Name, phone, te.RawError)
						time.Sleep(2 * time.Second)
						result, toolErr = registry.ExecuteTool(ctx, tc.Function.Name, args)
						if toolErr != nil {
							te = ClassifyError(toolErr)
						}
					}
					if toolErr != nil {
						if te.Type == ErrAuth {
							log.Printf("agent: auth error in tool %s for %s", tc.Function.Name, phone)
							a.saveHistory(phone, allTurns)
							return nil, fmt.Errorf("auth_error: %s", te.RawError)
						}
						result = map[string]any{
							"status": "error",
							"error":  map[string]any{"type": string(te.Type), "message": te.Message},
						}
					}
				}

				resultJSON, _ := json.Marshal(result)
				messages = append(messages, chatMessage{
					Role: "tool", Content: string(resultJSON), ToolCallID: tc.ID,
				})
				allTurns = append(allTurns, store.ConversationTurn{
					Role: "tool",
					Parts: []store.TurnPart{{
						FunctionResponse: &store.FunctionRespPart{
							ToolCallID: tc.ID, Name: tc.Function.Name, Response: result,
						},
					}},
				})
			}
		}
	}

	a.saveHistory(phone, allTurns)
	return &Response{Text: "Sua solicitação precisou de muitas etapas. Tente dividir em perguntas menores."}, nil
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

	// Skip leading tool turns left over from history trimming
	start := 0
	for start < len(turns) && turns[start].Role == "tool" {
		start++
	}

	var messages []chatMessage
	for i, t := range turns[start:] {
		turnsFromEnd := len(turns) - start - i

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
// Preserves entity IDs and titles from list results so follow-up references
// ("the 3rd one", "ticket #123") still resolve correctly.
func compressToolResponse(resp *store.FunctionRespPart) string {
	if errMsg, ok := resp.Response["error"]; ok {
		return fmt.Sprintf(`{"tool":"%s","status":"error","error":"%v"}`, resp.Name, errMsg)
	}

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

	// Preserve entity summaries from list results (id + name/title + status)
	for _, listKey := range []string{"chamados", "ativos", "artigos", "tarefas", "comentarios", "categorias", "departamentos", "historico"} {
		items, ok := resp.Response[listKey]
		if !ok {
			continue
		}
		// items can be []map[string]any or []any (from JSON unmarshal)
		var compressed []map[string]any
		switch v := items.(type) {
		case []map[string]any:
			for _, item := range v {
				compressed = append(compressed, compressItem(item))
			}
		case []any:
			for _, raw := range v {
				if item, ok := raw.(map[string]any); ok {
					compressed = append(compressed, compressItem(item))
				}
			}
		}
		if len(compressed) > 0 {
			summary[listKey] = compressed
		}
	}

	out, _ := json.Marshal(summary)
	return string(out)
}

// compressItem keeps only id, name/title, and status from a list item.
func compressItem(item map[string]any) map[string]any {
	c := map[string]any{}
	if id, ok := item["id"]; ok {
		c["id"] = id
	}
	// Keep the first available name field
	for _, nameKey := range []string{"nome", "titulo", "nome_completo"} {
		if name, ok := item[nameKey]; ok {
			c[nameKey] = name
			break
		}
	}
	if status, ok := item["status"]; ok {
		c["status"] = status
	}
	return c
}

func messageToTurn(msg chatMessage) store.ConversationTurn {
	turn := store.ConversationTurn{Role: msg.Role}
	if msg.Content != "" {
		turn.Parts = append(turn.Parts, store.TurnPart{Text: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			log.Printf("agent: invalid JSON in tool call %s args: %v", tc.Function.Name, err)
			args = map[string]any{}
		}
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

// estimateMessagesTokens approximates total token count for a messages array.
func estimateMessagesTokens(messages []chatMessage) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Arguments) + len(tc.Function.Name)
		}
	}
	return int(float64(total) / 3.5 * 1.1)
}

// pruneMessages drops oldest non-system turns until under budget.
func pruneMessages(messages []chatMessage, userName string, userID int) []chatMessage {
	for estimateMessagesTokens(messages) > maxMessageTokenBudget && len(messages) > 2 {
		// Drop the first non-system message
		if len(messages) > 1 {
			messages = append(messages[:1], messages[2:]...)
		}
	}
	// Fix orphaned tool messages at the start
	for len(messages) > 1 && messages[1].Role == "tool" {
		messages = append(messages[:1], messages[2:]...)
	}
	return messages
}

// rebuildTurns converts pruned messages back to conversation turns (drops system message).
func rebuildTurns(messages []chatMessage) []store.ConversationTurn {
	var turns []store.ConversationTurn
	for _, m := range messages {
		if m.Role == "system" {
			continue
		}
		turn := store.ConversationTurn{Role: m.Role}
		if m.Content != "" {
			if m.Role == "tool" && m.ToolCallID != "" {
				var resp map[string]any
				if err := json.Unmarshal([]byte(m.Content), &resp); err == nil {
					turn.Parts = append(turn.Parts, store.TurnPart{
						FunctionResponse: &store.FunctionRespPart{
							ToolCallID: m.ToolCallID,
							Response:   resp,
						},
					})
				}
			} else {
				turn.Parts = append(turn.Parts, store.TurnPart{Text: m.Content})
			}
		}
		for _, tc := range m.ToolCalls {
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
		turns = append(turns, turn)
	}
	return turns
}
