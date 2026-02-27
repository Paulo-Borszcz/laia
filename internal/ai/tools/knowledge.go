package tools

import (
	"context"
	"fmt"

	"github.com/lojasmm/laia/internal/ai"
	"github.com/lojasmm/laia/internal/glpi"
	"google.golang.org/genai"
)

// --- SearchKnowledgeBase ---

type SearchKnowledgeBase struct {
	glpi         *glpi.Client
	sessionToken string
}

func NewSearchKnowledgeBase(g *glpi.Client, token string) *SearchKnowledgeBase {
	return &SearchKnowledgeBase{glpi: g, sessionToken: token}
}

func (t *SearchKnowledgeBase) Name() string { return "search_knowledge_base" }
func (t *SearchKnowledgeBase) Description() string {
	return "Busca artigos na base de conhecimento do Nexus/GLPI por palavra-chave"
}
func (t *SearchKnowledgeBase) Parameters() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"query": {Type: genai.TypeString, Description: "Termo de busca (ex: VPN, email, impressora)"},
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
		items[i] = map[string]any{
			"id":   item["2"],
			"nome": item["1"],
		}
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

func (t *GetKBArticle) Name() string { return "get_kb_article" }
func (t *GetKBArticle) Description() string {
	return "Retorna o conteúdo completo de um artigo da base de conhecimento"
}
func (t *GetKBArticle) Parameters() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"article_id": {Type: genai.TypeInteger, Description: "ID do artigo"},
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
