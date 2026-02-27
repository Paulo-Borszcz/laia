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
	return "Lista os departamentos/setores disponíveis para chamados (categorias raiz)"
}
func (t *GetDepartments) Parameters() *genai.Schema { return nil }

func (t *GetDepartments) Execute(_ context.Context, _ map[string]any) (map[string]any, error) {
	categories, err := t.glpi.GetCategories(t.sessionToken, 0)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar departamentos: %w", err)
	}

	items := make([]map[string]any, len(categories))
	for i, c := range categories {
		items[i] = map[string]any{
			"id":   c.ID,
			"nome": c.Name,
		}
	}
	return map[string]any{"total": len(categories), "departamentos": items}, nil
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
	return "Lista as sub-categorias de um departamento/categoria pai"
}
func (t *GetDepartmentCategories) Parameters() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"department_id": {Type: genai.TypeInteger, Description: "ID do departamento ou categoria pai"},
		},
		Required: []string{"department_id"},
	}
}

func (t *GetDepartmentCategories) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	parentID, err := intArg(args, "department_id")
	if err != nil {
		return nil, err
	}

	categories, err := t.glpi.GetCategories(t.sessionToken, parentID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar categorias do departamento: %w", err)
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
