package tools

import (
	"fmt"
	"math"

	"github.com/lojasmm/laia/internal/ai"
	"github.com/lojasmm/laia/internal/glpi"
)

// BuildRegistry creates a Registry with all GLPI tools configured for this session.
func BuildRegistry(g *glpi.Client, sessionToken string, userID int) *ai.Registry {
	r := ai.NewRegistry()
	r.Register(NewListMyTickets(g, sessionToken))
	r.Register(NewGetTicket(g, sessionToken, userID))
	r.Register(NewCreateTicket(g, userID))
	r.Register(NewUpdateTicket(g, sessionToken, userID))
	r.Register(NewAddFollowup(g, sessionToken, userID))
	r.Register(NewGetFollowups(g, sessionToken, userID))
	r.Register(NewSearchTicketsAdvanced(g, sessionToken))
	r.Register(NewGetTicketTasks(g, sessionToken, userID))
	r.Register(NewAddTicketTask(g, sessionToken, userID))
	r.Register(NewApproveTicket(g, sessionToken))
	r.Register(NewRateTicket(g, sessionToken))
	r.Register(NewGetTicketHistory(g, sessionToken, userID))
	r.Register(NewSearchKnowledgeBase(g, sessionToken))
	r.Register(NewGetKBArticle(g, sessionToken))
	r.Register(NewSearchAssets(g, sessionToken))
	r.Register(NewGetDepartments(g, sessionToken))
	r.Register(NewGetDepartmentCategories(g, sessionToken))
	r.Register(NewGetSubCategories(g))
	r.Register(NewRespondInteractive())
	return r
}

// --- arg extraction helpers ---

func intArg(args map[string]any, key string) (int, error) {
	v, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("parâmetro obrigatório ausente: %s", key)
	}
	switch n := v.(type) {
	case float64:
		if n != math.Trunc(n) {
			return 0, fmt.Errorf("parâmetro %s deve ser inteiro", key)
		}
		return int(n), nil
	case int:
		return n, nil
	default:
		return 0, fmt.Errorf("parâmetro %s deve ser numérico", key)
	}
}

func stringArg(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("parâmetro obrigatório ausente: %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("parâmetro %s deve ser string", key)
	}
	return s, nil
}

// optionalStringArg extracts an optional string parameter, returning "" if absent.
func optionalStringArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

// optionalIntArg extracts an optional int parameter, returning 0 if absent.
func optionalIntArg(args map[string]any, key string) int {
	switch n := args[key].(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

// clarification builds a response asking the LLM to clarify with the user.
func clarification(question string, options []string, context string) map[string]any {
	result := map[string]any{
		"need_clarification": true,
		"question":           question,
	}
	if len(options) > 0 {
		result["options"] = options
	}
	if context != "" {
		result["context"] = context
	}
	return result
}

func intInSlice(val int, slice []int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func truncateText(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}
