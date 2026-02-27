package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lojasmm/laia/internal/ai"
	"github.com/lojasmm/laia/internal/glpi"
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
func (t *GetDepartments) Parameters() *ai.ParamSchema { return nil }

func (t *GetDepartments) Execute(_ context.Context, _ map[string]any) (map[string]any, error) {
	forms, err := t.glpi.GetForms(t.sessionToken)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar departamentos: %w", err)
	}

	items := make([]map[string]any, 0, len(forms))
	for _, f := range forms {
		if f.Name == "Abro chamado a quem? GUIA" || f.Name == "Abrir Chamado Loja" {
			continue
		}
		items = append(items, map[string]any{
			"id":   f.ID,
			"nome": f.Name,
		})
	}
	return map[string]any{"total": len(items), "departamentos": items}, nil
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
	return "Lista as categorias de chamado disponíveis para um departamento/formulário. Retorna as categorias ITIL que o usuário pode selecionar."
}
func (t *GetDepartmentCategories) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"department_id": {Type: "integer", Description: "ID do departamento/formulário"},
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

	for _, s := range sections {
		questions, err := t.glpi.GetSectionQuestions(t.sessionToken, s.ID)
		if err != nil {
			continue
		}

		for _, q := range questions {
			if q.FieldType != "dropdown" || q.ItemType != "ITILCategory" {
				continue
			}

			var vals dropdownValues
			if err := json.Unmarshal([]byte(q.Values), &vals); err != nil {
				continue
			}

			rootID := 0
			if vals.ShowTreeRoot != "" {
				fmt.Sscanf(vals.ShowTreeRoot, "%d", &rootID)
			}

			adminSession, err := t.glpi.AdminSession()
			if err != nil {
				return nil, fmt.Errorf("erro ao criar sessão admin: %w", err)
			}
			defer t.glpi.KillSession(adminSession)

			categories, err := t.glpi.GetCategories(adminSession, rootID)
			if err != nil {
				return nil, fmt.Errorf("erro ao buscar categorias: %w", err)
			}

			items := make([]map[string]any, len(categories))
			for i, c := range categories {
				items[i] = map[string]any{
					"id":   c.ID,
					"nome": c.Name,
				}
			}
			return map[string]any{
				"total":      len(categories),
				"categorias": items,
			}, nil
		}
	}

	return map[string]any{
		"total":      0,
		"categorias": []map[string]any{},
		"erro":       "nenhuma categoria encontrada para este formulário",
	}, nil
}

// dropdownValues extracts the tree root config from FormCreator question values.
type dropdownValues struct {
	ShowTreeRoot string `json:"show_tree_root"`
}

// --- GetSubCategories ---

type GetSubCategories struct {
	glpi *glpi.Client
}

func NewGetSubCategories(g *glpi.Client) *GetSubCategories {
	return &GetSubCategories{glpi: g}
}

func (t *GetSubCategories) Name() string { return "get_subcategories" }
func (t *GetSubCategories) Description() string {
	return "Lista as sub-categorias de uma categoria ITIL. Use quando uma categoria tem filhas e você precisa aprofundar."
}
func (t *GetSubCategories) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"category_id": {Type: "integer", Description: "ID da categoria pai"},
		},
		Required: []string{"category_id"},
	}
}

func (t *GetSubCategories) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	parentID, err := intArg(args, "category_id")
	if err != nil {
		return nil, err
	}

	adminSession, err := t.glpi.AdminSession()
	if err != nil {
		return nil, fmt.Errorf("erro ao criar sessão admin: %w", err)
	}
	defer t.glpi.KillSession(adminSession)

	categories, err := t.glpi.GetCategories(adminSession, parentID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar sub-categorias: %w", err)
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
var _ ai.Tool = (*GetSubCategories)(nil)
