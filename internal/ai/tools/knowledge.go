package tools

import (
	"context"
	"fmt"

	"github.com/lojasmm/laia/internal/ai"
	"github.com/lojasmm/laia/internal/glpi"
)

// --- SearchKnowledgeBase ---

type SearchKnowledgeBase struct {
	glpi         *glpi.Client
	sessionToken string
}

func NewSearchKnowledgeBase(g *glpi.Client, token string) *SearchKnowledgeBase {
	return &SearchKnowledgeBase{glpi: g, sessionToken: token}
}

func (t *SearchKnowledgeBase) Name() string     { return "search_knowledge_base" }
func (t *SearchKnowledgeBase) ReadOnly() bool { return true }
func (t *SearchKnowledgeBase) Description() string {
	return `Busca artigos na base de conhecimento do Nexus/GLPI.
Quando usar: quando o usuario perguntar "como faz...", "tem tutorial de...", "como configurar...", ou buscar solucoes para problemas conhecidos.
O preview do conteudo e truncado a 200 caracteres — use get_kb_article para ler o artigo completo.
Se nenhum artigo for encontrado, sugira ao usuario abrir um chamado para obter ajuda.
Retorna: {total, artigos: [{id, nome, preview}]}.`
}
func (t *SearchKnowledgeBase) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"query": {Type: "string", Description: "Termo de busca (ex: VPN, email, impressora, como configurar)"},
		},
		Required: []string{"query"},
	}
}

func (t *SearchKnowledgeBase) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	query, _ := stringArg(args, "query")
	if query == "" {
		return nil, fmt.Errorf("termo de busca é obrigatório")
	}

	result, err := t.glpi.SearchKnowledgeBase(t.sessionToken, query)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar na base de conhecimento: %w", err)
	}

	items := make([]map[string]any, len(result.Data))
	for i, item := range result.Data {
		entry := map[string]any{
			"id":   item["2"],
			"nome": item["6"], // Field 6 = Subject/name
		}
		// Field 7 = Content/answer; include a truncated preview
		if body, ok := item["7"].(string); ok && body != "" {
			entry["preview"] = truncateText(body, 200)
		}
		items[i] = entry
	}
	return map[string]any{"total": result.TotalCount, "artigos": items}, nil
}

// --- GetKBArticle ---

type GetKBArticle struct {
	glpi         *glpi.Client
	sessionToken string
}

func NewGetKBArticle(g *glpi.Client, token string) *GetKBArticle {
	return &GetKBArticle{glpi: g, sessionToken: token}
}

func (t *GetKBArticle) Name() string     { return "get_kb_article" }
func (t *GetKBArticle) ReadOnly() bool { return true }
func (t *GetKBArticle) Description() string {
	return `Retorna o conteudo completo de um artigo da base de conhecimento.
Quando usar: apos search_knowledge_base encontrar um artigo relevante, use esta ferramenta para ler o conteudo completo.
O conteudo vem em formato HTML. Ao apresentar ao usuario, converta para formatacao WhatsApp: *negrito*, _italico_, listas com •.
Retorna: {id, titulo, conteudo} onde conteudo e o HTML do artigo.`
}
func (t *GetKBArticle) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"article_id": {Type: "integer", Description: "ID do artigo"},
		},
		Required: []string{"article_id"},
	}
}

func (t *GetKBArticle) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	articleID, err := intArg(args, "article_id")
	if err != nil {
		return nil, err
	}

	article, err := t.glpi.GetKBArticle(t.sessionToken, articleID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar artigo: %w", err)
	}

	return map[string]any{
		"id":       article.ID,
		"titulo":   article.Name,
		"conteudo": article.Answer,
	}, nil
}

var _ ai.Tool = (*SearchKnowledgeBase)(nil)
var _ ai.Tool = (*GetKBArticle)(nil)
