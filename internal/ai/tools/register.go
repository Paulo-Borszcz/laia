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
