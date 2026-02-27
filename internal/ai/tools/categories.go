package tools

import (
	"context"
	"fmt"

	"github.com/lojasmm/laia/internal/ai"
	"github.com/lojasmm/laia/internal/glpi"
	"google.golang.org/genai"
)

type GetCategories struct {
	glpi         *glpi.Client
	sessionToken string
}

func NewGetCategories(g *glpi.Client, token string) *GetCategories {
	return &GetCategories{glpi: g, sessionToken: token}
}

func (t *GetCategories) Name() string { return "get_categories" }
func (t *GetCategories) Description() string {
	return "Lista as categorias de chamados disponíveis no Nexus/GLPI"
}
func (t *GetCategories) Parameters() *genai.Schema { return nil }

func (t *GetCategories) Execute(_ context.Context, _ map[string]any) (map[string]any, error) {
	categories, err := t.glpi.GetCategories(t.sessionToken)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar categorias: %w", err)
	}

	items := make([]map[string]any, len(categories))
	for i, c := range categories {
		items[i] = map[string]any{
			"id":   c.ID,
			"nome": c.Completename,
		}
	}
	return map[string]any{"total": len(categories), "categorias": items}, nil
}

var _ ai.Tool = (*GetCategories)(nil)
