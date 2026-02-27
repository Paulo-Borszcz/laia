package ai

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// Tool is a single function the AI agent can call.
type Tool interface {
	Name() string
	Description() string
	Parameters() *genai.Schema
	Execute(ctx context.Context, args map[string]any) (map[string]any, error)
}

// Registry holds all registered tools and builds the genai declarations.
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

// GenaiTools returns the tools slice for Gemini GenerateContent config.
func (r *Registry) GenaiTools() []*genai.Tool {
	var decls []*genai.FunctionDeclaration
	for _, t := range r.tools {
		decls = append(decls, &genai.FunctionDeclaration{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}
