package tools

import (
	"context"
	"fmt"

	"github.com/lojasmm/laia/internal/ai"
	"github.com/lojasmm/laia/internal/glpi"
)

type SearchAssets struct {
	glpi         *glpi.Client
	sessionToken string
}

func NewSearchAssets(g *glpi.Client, token string) *SearchAssets {
	return &SearchAssets{glpi: g, sessionToken: token}
}

func (t *SearchAssets) Name() string { return "search_assets" }
func (t *SearchAssets) Description() string {
	return "Busca ativos de TI (computadores, monitores, impressoras) por nome ou número de série"
}
func (t *SearchAssets) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"type":  {Type: "string", Description: "Tipo de ativo: Computer, Monitor, Printer, Phone, NetworkEquipment"},
			"query": {Type: "string", Description: "Termo de busca (nome, serial, etc.)"},
		},
		Required: []string{"type", "query"},
	}
}

func (t *SearchAssets) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	assetType, _ := stringArg(args, "type")
	query, _ := stringArg(args, "query")
	if assetType == "" || query == "" {
		return nil, fmt.Errorf("tipo e termo de busca são obrigatórios")
	}

	allowed := map[string]bool{
		"Computer": true, "Monitor": true, "Printer": true,
		"Phone": true, "NetworkEquipment": true,
	}
	if !allowed[assetType] {
		return nil, fmt.Errorf("tipo de ativo inválido: %s", assetType)
	}

	result, err := t.glpi.SearchAssets(t.sessionToken, assetType, query)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar ativos: %w", err)
	}

	items := make([]map[string]any, len(result.Data))
	for i, item := range result.Data {
		items[i] = map[string]any{
			"id":     item["2"],
			"nome":   item["1"],
			"status": item["31"],
		}
	}
	return map[string]any{"total": result.TotalCount, "ativos": items}, nil
}

var _ ai.Tool = (*SearchAssets)(nil)
