package ai

import (
	"context"
	"fmt"
)

// ParamSchema describes tool parameters using JSON Schema conventions.
type ParamSchema struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description,omitempty"`
	Properties  map[string]*ParamSchema `json:"properties,omitempty"`
	Required    []string               `json:"required,omitempty"`
}

// Tool is a single function the AI agent can call.
type Tool interface {
	Name() string
	Description() string
	Parameters() *ParamSchema
	Execute(ctx context.Context, args map[string]any) (map[string]any, error)
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
	return m
}
