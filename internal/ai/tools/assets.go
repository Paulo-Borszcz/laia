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
	return `Busca ativos de TI por nome ou numero de serie.
Quando usar: quando o usuario perguntar sobre equipamentos, patrimonio, ativos. Ex: "meu computador", "impressora do 2o andar", "monitor serial XYZ".
Tipos disponiveis (mapeamento PT→EN): computador→Computer, monitor→Monitor, impressora→Printer, telefone→Phone, equipamento de rede→NetworkEquipment.
Se o tipo nao for informado, pedira esclarecimento.
Retorna: lista com id, nome e status do ativo.`
}
func (t *SearchAssets) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"type": {
				Type:        "string",
				Description: "Tipo de ativo (em ingles): Computer, Monitor, Printer, Phone, NetworkEquipment. Converta do portugues se necessario.",
				Enum:        []string{"Computer", "Monitor", "Printer", "Phone", "NetworkEquipment"},
			},
			"query": {Type: "string", Description: "Termo de busca (nome, serial, patrimonio)"},
		},
		Required: []string{"query"},
	}
}

func (t *SearchAssets) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	assetType := optionalStringArg(args, "type")
	query := optionalStringArg(args, "query")
	if query == "" {
		return nil, fmt.Errorf("termo de busca é obrigatório")
	}

	if assetType == "" {
		return clarification(
			"Qual tipo de ativo voce quer buscar?",
			[]string{"Computador", "Monitor", "Impressora", "Telefone", "Equipamento de rede"},
			"Use respond_interactive com botoes para apresentar as opcoes ao usuario. Mapeie a resposta: Computador→Computer, Monitor→Monitor, Impressora→Printer, Telefone→Phone, Equipamento de rede→NetworkEquipment.",
		), nil
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
