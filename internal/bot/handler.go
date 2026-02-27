package bot

import (
	"fmt"
	"log"
	"strings"

	"github.com/lojasmm/laia/internal/glpi"
	"github.com/lojasmm/laia/internal/store"
	"github.com/lojasmm/laia/internal/whatsapp"
)

type Handler struct {
	wa      *whatsapp.Client
	glpi    *glpi.Client
	store   store.Store
	authURL string
}

func NewHandler(wa *whatsapp.Client, g *glpi.Client, s store.Store, authURL string) *Handler {
	return &Handler{wa: wa, glpi: g, store: s, authURL: authURL}
}

func (h *Handler) HandleMessage(phone, text string) {
	user, err := h.store.GetUser(phone)
	if err != nil {
		log.Printf("bot: store error for %s: %v", phone, err)
		return
	}

	if user == nil {
		h.sendVerificationLink(phone)
		return
	}

	h.handleCommand(user, phone, text)
}

func (h *Handler) sendVerificationLink(phone string) {
	link := fmt.Sprintf("%s/auth/verify?phone=%s", h.authURL, phone)
	msg := fmt.Sprintf(
		"Olá! Para usar este serviço, primeiro vincule seu WhatsApp ao Nexus.\n\nAcesse: %s",
		link,
	)
	if err := h.wa.SendText(phone, msg); err != nil {
		log.Printf("bot: failed to send verification link to %s: %v", phone, err)
	}
}

func (h *Handler) handleCommand(user *store.User, phone, text string) {
	cmd := strings.TrimSpace(strings.ToLower(text))

	switch {
	case cmd == "meus chamados" || cmd == "chamados":
		h.listTickets(user, phone)
	default:
		h.sendHelp(user, phone)
	}
}

func (h *Handler) listTickets(user *store.User, phone string) {
	sessionToken, err := h.glpi.InitSession(user.UserToken)
	if err != nil {
		log.Printf("bot: initSession failed for user %s: %v", user.Name, err)
		h.wa.SendText(phone, "Erro ao conectar ao Nexus. Tente novamente mais tarde.")
		return
	}
	defer h.glpi.KillSession(sessionToken)

	tickets, err := h.glpi.GetMyTickets(sessionToken)
	if err != nil {
		log.Printf("bot: getMyTickets failed for user %s: %v", user.Name, err)
		h.wa.SendText(phone, "Erro ao buscar seus chamados.")
		return
	}

	if len(tickets) == 0 {
		h.wa.SendText(phone, "Você não tem chamados abertos no momento.")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Seus chamados (%d):*\n\n", len(tickets)))
	for _, t := range tickets {
		sb.WriteString(fmt.Sprintf("• #%d — %s\n  Status: %s\n\n", t.ID, t.Name, ticketStatus(t.Status)))
	}

	if err := h.wa.SendText(phone, sb.String()); err != nil {
		log.Printf("bot: failed to send ticket list to %s: %v", phone, err)
	}
}

func (h *Handler) sendHelp(user *store.User, phone string) {
	msg := fmt.Sprintf(
		"Olá, %s! Comandos disponíveis:\n\n• *meus chamados* — lista seus chamados no Nexus",
		user.Name,
	)
	if err := h.wa.SendText(phone, msg); err != nil {
		log.Printf("bot: failed to send help to %s: %v", phone, err)
	}
}

// ticketStatus maps GLPI ticket status IDs to readable labels.
// Reference: GLPI source — inc/ticket.class.php
func ticketStatus(status int) string {
	switch status {
	case 1:
		return "Novo"
	case 2:
		return "Em atendimento (atribuído)"
	case 3:
		return "Em atendimento (planejado)"
	case 4:
		return "Pendente"
	case 5:
		return "Solucionado"
	case 6:
		return "Fechado"
	default:
		return fmt.Sprintf("Desconhecido (%d)", status)
	}
}
