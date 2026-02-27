package tools

import (
	"context"

	"github.com/lojasmm/laia/internal/ai"
)

// RespondInteractive is a pseudo-tool: the agent intercepts it before
// Execute runs, so Execute should never be called.
type RespondInteractive struct{}

func NewRespondInteractive() *RespondInteractive { return &RespondInteractive{} }

func (t *RespondInteractive) Name() string { return "respond_interactive" }
func (t *RespondInteractive) Description() string {
	return "Envia mensagem interativa com botões ou lista de opções clicáveis. Use no lugar de resposta de texto quando quiser oferecer opções ao usuário."
}

func (t *RespondInteractive) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"message_type": {
				Type:        "string",
				Description: "Tipo de mensagem interativa",
				Enum:        []string{"buttons", "list"},
			},
			"text": {
				Type:        "string",
				Description: "Corpo da mensagem exibido ao usuário",
			},
			"buttons": {
				Type:        "array",
				Description: "Botões clicáveis (quando message_type=buttons). Máximo 3 botões, título máx 20 caracteres.",
				Items: &ai.ParamSchema{
					Type: "object",
					Properties: map[string]*ai.ParamSchema{
						"id":    {Type: "string", Description: "Identificador único do botão"},
						"title": {Type: "string", Description: "Texto exibido no botão (máx 20 chars)"},
					},
					Required: []string{"id", "title"},
				},
			},
			"list_button_text": {
				Type:        "string",
				Description: "Texto do botão que abre a lista (quando message_type=list, máx 20 chars)",
			},
			"sections": {
				Type:        "array",
				Description: "Seções da lista (quando message_type=list)",
				Items: &ai.ParamSchema{
					Type: "object",
					Properties: map[string]*ai.ParamSchema{
						"title": {Type: "string", Description: "Título da seção (máx 24 chars)"},
						"rows": {
							Type:        "array",
							Description: "Itens da seção",
							Items: &ai.ParamSchema{
								Type: "object",
								Properties: map[string]*ai.ParamSchema{
									"id":          {Type: "string", Description: "Identificador único da opção"},
									"title":       {Type: "string", Description: "Texto da opção (máx 24 chars)"},
									"description": {Type: "string", Description: "Descrição da opção (máx 72 chars)"},
								},
								Required: []string{"id", "title"},
							},
						},
					},
					Required: []string{"title", "rows"},
				},
			},
		},
		Required: []string{"message_type", "text"},
	}
}

// Execute should never be called — the agent loop intercepts this tool.
func (t *RespondInteractive) Execute(_ context.Context, _ map[string]any) (map[string]any, error) {
	return map[string]any{"status": "intercepted"}, nil
}
