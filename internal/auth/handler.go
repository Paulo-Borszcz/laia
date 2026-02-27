package auth

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/lojasmm/laia/internal/glpi"
	"github.com/lojasmm/laia/internal/store"
)

//go:embed page.html
var pageFS embed.FS

var pageTmpl = template.Must(template.ParseFS(pageFS, "page.html"))

type pageData struct {
	Phone   string
	Message string
	Success bool
}

type Handler struct {
	glpi  *glpi.Client
	store store.Store
}

func NewHandler(g *glpi.Client, s store.Store) *Handler {
	return &Handler{glpi: g, store: s}
}

func (h *Handler) HandleVerifyPage(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		http.Error(w, "parametro phone obrigatorio", http.StatusBadRequest)
		return
	}
	pageTmpl.Execute(w, pageData{Phone: phone})
}

func (h *Handler) HandleVerifySubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	phone := r.FormValue("phone")
	userToken := r.FormValue("user_token")

	if phone == "" || userToken == "" {
		pageTmpl.Execute(w, pageData{
			Phone:   phone,
			Message: "Telefone e token são obrigatórios.",
		})
		return
	}

	sessionToken, err := h.glpi.InitSession(userToken)
	if err != nil {
		log.Printf("auth: initSession failed for phone %s: %v", phone, err)
		pageTmpl.Execute(w, pageData{
			Phone:   phone,
			Message: "Token inválido ou erro ao conectar ao Nexus. Verifique e tente novamente.",
		})
		return
	}

	fullSession, err := h.glpi.GetFullSession(sessionToken)
	if err != nil {
		log.Printf("auth: getFullSession failed: %v", err)
		h.glpi.KillSession(sessionToken)
		pageTmpl.Execute(w, pageData{
			Phone:   phone,
			Message: "Erro ao obter dados da sessão. Tente novamente.",
		})
		return
	}

	h.glpi.KillSession(sessionToken)

	u := store.User{
		Phone:           phone,
		UserToken:       userToken,
		GLPIUserID:      fullSession.Session.GlpiID,
		Name:            fullSession.Session.GlpiFriendlyName,
		AuthenticatedAt: time.Now(),
	}
	if err := h.store.SaveUser(u); err != nil {
		log.Printf("auth: saveUser failed: %v", err)
		pageTmpl.Execute(w, pageData{
			Phone:   phone,
			Message: "Erro interno ao salvar dados. Tente novamente.",
		})
		return
	}

	log.Printf("auth: user %s (%d) linked to phone %s", u.Name, u.GLPIUserID, phone)

	pageTmpl.Execute(w, pageData{
		Phone:   phone,
		Message: "Verificação concluída! Seu WhatsApp está vinculado ao Nexus. Pode voltar ao chat.",
		Success: true,
	})
}
