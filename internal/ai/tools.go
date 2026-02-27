package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

const (
	// Max JSON output length before truncation (~8KB, keeps token usage low)
	maxOutputLen = 8192
	// Per-tool execution timeout
	toolTimeout = 30 * time.Second
	// Max items in a list before truncation
	maxListItems = 10
)

// ParamSchema describes tool parameters using JSON Schema conventions.
type ParamSchema struct {
	Type        string                  `json:"type"`
	Description string                  `json:"description,omitempty"`
	Properties  map[string]*ParamSchema `json:"properties,omitempty"`
	Required    []string                `json:"required,omitempty"`
	Enum        []string                `json:"enum,omitempty"`
	Items       *ParamSchema            `json:"items,omitempty"`
}

// Tool is a single function the AI agent can call.
type Tool interface {
	Name() string
	Description() string
	Parameters() *ParamSchema
	Execute(ctx context.Context, args map[string]any) (map[string]any, error)
	// ReadOnly returns true if the tool only reads data (safe for parallel execution).
	ReadOnly() bool
}

// Registry holds all registered tools.
type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return t, nil
}

// ExecuteTool validates args, applies a timeout, runs the tool, truncates output,
// and logs execution duration.
func (r *Registry) ExecuteTool(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	t, err := r.Get(name)
	if err != nil {
		return nil, err
	}

	// Validate required parameters before execution
	if schema := t.Parameters(); schema != nil {
		if err := validateArgs(schema, args); err != nil {
			return nil, &ToolError{
				Type:    ErrValidation,
				Message: fmt.Sprintf("argumentos inválidos para %s: %v", name, err),
			}
		}
	}

	// Apply per-tool timeout
	toolCtx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()

	start := time.Now()
	result, err := t.Execute(toolCtx, args)
	elapsed := time.Since(start)

	log.Printf("tool: %s completed in %dms", name, elapsed.Milliseconds())

	if err != nil {
		return nil, err
	}

	// Truncate large outputs to save tokens
	return truncateOutput(result), nil
}

// IsReadOnly checks if a tool is safe for parallel execution.
func (r *Registry) IsReadOnly(name string) bool {
	t, err := r.Get(name)
	if err != nil {
		return false
	}
	return t.ReadOnly()
}

// validateArgs checks that all required parameters are present and have correct types.
func validateArgs(schema *ParamSchema, args map[string]any) error {
	if args == nil {
		args = map[string]any{}
	}
	for _, req := range schema.Required {
		v, ok := args[req]
		if !ok || v == nil {
			return fmt.Errorf("parâmetro obrigatório ausente: %s", req)
		}

		prop, hasProp := schema.Properties[req]
		if !hasProp {
			continue
		}

		switch prop.Type {
		case "string":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("parâmetro %s deve ser string", req)
			}
			if s == "" {
				return fmt.Errorf("parâmetro %s não pode ser vazio", req)
			}
			if len(prop.Enum) > 0 {
				valid := false
				for _, e := range prop.Enum {
					if s == e {
						valid = true
						break
					}
				}
				if !valid {
					return fmt.Errorf("parâmetro %s deve ser um de: %v", req, prop.Enum)
				}
			}
		case "integer", "number":
			switch v.(type) {
			case float64, int:
				// ok
			default:
				return fmt.Errorf("parâmetro %s deve ser numérico", req)
			}
		}
	}
	return nil
}

// truncateOutput detects large list fields and truncates them, then checks total size.
func truncateOutput(result map[string]any) map[string]any {
	// First pass: truncate known list fields to maxListItems
	for key, val := range result {
		if items, ok := val.([]map[string]any); ok && len(items) > maxListItems {
			originalCount := len(items)
			truncated := make([]map[string]any, maxListItems)
			for i := range maxListItems {
				// Copy item, strip verbose fields to save tokens
				item := make(map[string]any, len(items[i]))
				for k, v := range items[i] {
					if k == "descricao" || k == "conteudo" || k == "preview" {
						continue
					}
					item[k] = v
				}
				truncated[i] = item
			}
			result[key] = truncated
			result["_truncated"] = true
			result["_truncated_field"] = key
			result["_original_count"] = originalCount
			result["_nota"] = fmt.Sprintf("Mostrando %d de %d resultados. Sugira ao usuário refinar a busca.", maxListItems, originalCount)
			return result
		}
	}

	// Second pass: check total size
	data, err := json.Marshal(result)
	if err != nil || len(data) <= maxOutputLen {
		return result
	}

	log.Printf("tool: output truncated from %d to %d bytes", len(data), maxOutputLen)
	return map[string]any{
		"_truncated": true,
		"_summary":   string(data[:maxOutputLen]),
	}
}

// OpenAITools returns tool definitions for the OpenAI chat completion API.
func (r *Registry) OpenAITools() []map[string]any {
	tools := make([]map[string]any, 0, len(r.tools))
	for _, t := range r.tools {
		fn := map[string]any{
			"name":        t.Name(),
			"description": t.Description(),
		}
		if p := t.Parameters(); p != nil {
			fn["parameters"] = schemaToMap(p)
		}
		tools = append(tools, map[string]any{
			"type":     "function",
			"function": fn,
		})
	}
	return tools
}

func schemaToMap(s *ParamSchema) map[string]any {
	m := map[string]any{"type": s.Type}
	if s.Description != "" {
		m["description"] = s.Description
	}
	if len(s.Properties) > 0 {
		props := make(map[string]any)
		for k, v := range s.Properties {
			props[k] = schemaToMap(v)
		}
		m["properties"] = props
	}
	if len(s.Required) > 0 {
		m["required"] = s.Required
	}
	if len(s.Enum) > 0 {
		m["enum"] = s.Enum
	}
	if s.Items != nil {
		m["items"] = schemaToMap(s.Items)
	}
	return m
}
