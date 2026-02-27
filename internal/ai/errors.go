package ai

import "strings"

// ErrorType categorizes tool errors for structured handling by the agent loop.
type ErrorType string

const (
	ErrAuth       ErrorType = "auth_error"    // 401, invalid/expired token
	ErrNotFound   ErrorType = "not_found"     // 404, resource doesn't exist
	ErrRateLimit  ErrorType = "rate_limit"    // 429, too many requests
	ErrServer     ErrorType = "server_error"  // 5xx, GLPI/external service down
	ErrValidation ErrorType = "validation"    // Bad arguments, missing params
	ErrSession    ErrorType = "session_error" // GLPI session expired mid-request
	ErrTimeout    ErrorType = "timeout"       // Context deadline exceeded
)

// ToolError wraps a tool execution error with type classification.
type ToolError struct {
	Type      ErrorType
	Message   string // User-facing message in PT-BR
	RawError  string // Original error for logging
	Retryable bool
}

func (e *ToolError) Error() string { return e.RawError }

// ClassifyError inspects an error string and returns a typed ToolError
// with user-friendly messages in PT-BR.
func ClassifyError(err error) *ToolError {
	raw := err.Error()

	switch {
	case containsAny(raw, "context deadline exceeded", "timeout"):
		return &ToolError{
			Type: ErrTimeout, Retryable: true,
			Message:  "O Nexus demorou para responder. Tentando novamente...",
			RawError: raw,
		}
	case containsAny(raw, "401", "unauthorized", "invalid.*token", "ERROR_SESSION_TOKEN_INVALID"):
		return &ToolError{
			Type: ErrAuth, Retryable: false,
			Message:  "Sua sessão expirou. Reconectando...",
			RawError: raw,
		}
	case containsAny(raw, "404", "not found", "item not found"):
		return &ToolError{
			Type: ErrNotFound, Retryable: false,
			Message:  "Recurso não encontrado no Nexus. Verifique o ID informado.",
			RawError: raw,
		}
	case containsAny(raw, "429", "rate limit", "too many requests"):
		return &ToolError{
			Type: ErrRateLimit, Retryable: true,
			Message:  "Servidor ocupado, tentando novamente...",
			RawError: raw,
		}
	case containsAny(raw, "500", "502", "503", "504", "server error", "internal error"):
		return &ToolError{
			Type: ErrServer, Retryable: true,
			Message:  "O Nexus está temporariamente indisponível. Tente novamente em alguns minutos.",
			RawError: raw,
		}
	case containsAny(raw, "session", "sessão", "initSession"):
		return &ToolError{
			Type: ErrSession, Retryable: false,
			Message:  "Erro na sessão do Nexus. Pode ser necessário vincular novamente.",
			RawError: raw,
		}
	case containsAny(raw, "argumento", "obrigatório", "inválido", "deve ser"):
		return &ToolError{
			Type: ErrValidation, Retryable: false,
			Message:  raw, // Validation messages are already user-friendly
			RawError: raw,
		}
	default:
		return &ToolError{
			Type: ErrServer, Retryable: false,
			Message:  "Erro inesperado ao acessar o Nexus.",
			RawError: raw,
		}
	}
}

func containsAny(s string, patterns ...string) bool {
	lower := strings.ToLower(s)
	for _, p := range patterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}
