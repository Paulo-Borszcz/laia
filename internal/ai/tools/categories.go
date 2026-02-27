package tools

import (
	"context"
	"fmt"

	"github.com/lojasmm/laia/internal/ai"
	"github.com/lojasmm/laia/internal/glpi"
	"google.golang.org/genai"
)

// --- GetDepartments ---

type GetDepartments struct {
	glpi         *glpi.Client
	sessionToken string
}

func NewGetDepartments(g *glpi.Client, token string) *GetDepartments {
	return &GetDepartments{glpi: g, sessionToken: token}
}

func (t *GetDepartments) Name() string { return "get_departments" }
func (t *GetDepartments) Description() string {
	return "Lista os departamentos/setores disponíveis para chamados (formulários do Nexus)"
}
func (t *GetDepartments) Parameters() *genai.Schema { return nil }

func (t *GetDepartments) Execute(_ context.Context, _ map[string]any) (map[string]any, error) {
	forms, err := t.glpi.GetForms(t.sessionToken)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar departamentos: %w", err)
	}

	items := make([]map[string]any, len(forms))
	for i, f := range forms {
		items[i] = map[string]any{
			"id":   f.ID,
			"nome": f.Name,
		}
	}
	return map[string]any{"total": len(forms), "departamentos": items}, nil
}

// --- GetDepartmentCategories ---

type GetDepartmentCategories struct {
	glpi         *glpi.Client
	sessionToken string
}

func NewGetDepartmentCategories(g *glpi.Client, token string) *GetDepartmentCategories {
	return &GetDepartmentCategories{glpi: g, sessionToken: token}
}

func (t *GetDepartmentCategories) Name() string { return "get_department_categories" }
func (t *GetDepartmentCategories) Description() string {
	return "Lista as seções e perguntas de um formulário/departamento para entender as categorias disponíveis"
}
func (t *GetDepartmentCategories) Parameters() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"department_id": {Type: genai.TypeInteger, Description: "ID do departamento/formulário"},
		},
		Required: []string{"department_id"},
	}
}

func (t *GetDepartmentCategories) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	formID, err := intArg(args, "department_id")
	if err != nil {
		return nil, err
	}

	sections, err := t.glpi.GetFormSections(t.sessionToken, formID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar seções do formulário: %w", err)
	}

	result := make([]map[string]any, 0, len(sections))
	for _, s := range sections {
		questions, err := t.glpi.GetSectionQuestions(t.sessionToken, s.ID)
		if err != nil {
			continue
		}

		qItems := make([]map[string]any, len(questions))
		for j, q := range questions {
			qItems[j] = map[string]any{
				"id":   q.ID,
				"nome": q.Name,
				"tipo": q.FieldType,
			}
		}

		result = append(result, map[string]any{
			"secao_id":   s.ID,
			"secao_nome": s.Name,
			"perguntas":  qItems,
		})
	}

	return map[string]any{"total_secoes": len(result), "secoes": result}, nil
}

// --- GetITILCategories ---

type GetITILCategories struct {
	glpi         *glpi.Client
	sessionToken string
}

func NewGetITILCategories(g *glpi.Client, token string) *GetITILCategories {
	return &GetITILCategories{glpi: g, sessionToken: token}
}

func (t *GetITILCategories) Name() string { return "get_itil_categories" }
func (t *GetITILCategories) Description() string {
	return "Lista as categorias ITIL filtrando por categoria pai. Use parent_id=0 para categorias raiz"
}
func (t *GetITILCategories) Parameters() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"parent_id": {Type: genai.TypeInteger, Description: "ID da categoria pai (0 para categorias raiz)"},
		},
		Required: []string{"parent_id"},
	}
}

func (t *GetITILCategories) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	parentID, err := intArg(args, "parent_id")
	if err != nil {
		return nil, err
	}

	categories, err := t.glpi.GetCategories(t.sessionToken, parentID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar categorias ITIL: %w", err)
	}

	items := make([]map[string]any, len(categories))
	for i, c := range categories {
		items[i] = map[string]any{
			"id":   c.ID,
			"nome": c.Name,
		}
	}
	return map[string]any{"total": len(categories), "categorias": items}, nil
}

var _ ai.Tool = (*GetDepartments)(nil)
var _ ai.Tool = (*GetDepartmentCategories)(nil)
var _ ai.Tool = (*GetITILCategories)(nil)
