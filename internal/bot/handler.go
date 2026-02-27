package bot

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/lojasmm/laia/internal/ai"
	"github.com/lojasmm/laia/internal/session"
	"github.com/lojasmm/laia/internal/store"
	"github.com/lojasmm/laia/internal/whatsapp"
)

type Handler struct {
	wa         *whatsapp.Client
	store      store.Store
	authURL    string
	agent      *ai.Agent
	sessionMgr *session.Manager
}

func NewHandler(wa *whatsapp.Client, s store.Store, authURL string, agent *ai.Agent, sm *session.Manager) *Handler {
	return &Handler{wa: wa, store: s, authURL: authURL, agent: agent, sessionMgr: sm}
}

func (h *Handler) HandleMessage(phone, messageID, text string) {
	// Per-user lock prevents race conditions from concurrent messages
	err := h.sessionMgr.WithLock(phone, func() error {
		user, err := h.store.GetUser(phone)
		if err != nil {
			log.Printf("bot: store error for %s: %v", phone, err)
			return nil
		}

		if user == nil {
			h.sendVerificationLink(phone)
			return nil
		}

		h.handleCommand(user, phone, messageID, text)
		return nil
	})
	if err != nil {
		log.Printf("bot: session lock error for %s: %v", phone, err)
	}
}

func (h *Handler) sendVerificationLink(phone string) {
	link := fmt.Sprintf("%s/auth/verify?phone=%s", h.authURL, phone)
	body := "Olá! Eu sou a *Laia*, sua assistente virtual do *Nexus* aqui nas Lojas MM.\n\n" +
		"Comigo você pode:\n" +
		"• Abrir e acompanhar chamados\n" +
		"• Adicionar comentários e atualizações\n" +
		"• Consultar a base de conhecimento\n\n" +
		"Para começarmos, preciso vincular seu WhatsApp à sua conta do Nexus. " +
		"É rápido — basta clicar no botão abaixo!"

	if err := h.wa.SendCTAButton(phone, body, "Vincular conta", link); err != nil {
		log.Printf("bot: failed to send verification link to %s: %v", phone, err)
	}
}

func (h *Handler) handleCommand(user *store.User, phone, messageID, text string) {
	// Hourglass reaction: signal to user that we're processing
	if messageID != "" {
		if err := h.wa.ReactMessage(phone, messageID, "⏳"); err != nil {
			log.Printf("bot: failed to send hourglass reaction: %v", err)
		}
	}

	ctx := context.Background()
	resp, err := h.agent.Handle(ctx, user, phone, text)

	// Remove hourglass reaction after processing
	if messageID != "" {
		// Empty emoji removes the reaction
		h.wa.ReactMessage(phone, messageID, "")
	}

	if err != nil {
		log.Printf("bot: agent error for %s: %v", phone, err)
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "auth_error"):
			h.wa.SendText(phone, "Sua sessão com o Nexus expirou. Vou enviar um novo link para reconectar sua conta.")
			h.store.DeleteUser(phone)
			h.sendVerificationLink(phone)
		case strings.Contains(errMsg, "initSession"):
			h.wa.SendText(phone, "O Nexus pode estar em manutenção no momento. Tente novamente em alguns minutos.")
		case strings.Contains(errMsg, "context"):
			h.store.ClearHistory(phone)
			h.wa.SendText(phone, "Nossa conversa ficou muito longa. Comece uma nova pergunta, por favor.")
		default:
			h.wa.SendText(phone, "Desculpe, ocorreu um erro ao processar sua mensagem. Tente novamente mais tarde.")
		}
		return
	}

	var sendErr error
	switch {
	case len(resp.Buttons) > 0:
		sendErr = h.wa.SendInteractiveButtons(phone, resp.Text, toWAButtons(resp.Buttons))
	case resp.List != nil:
		sendErr = h.wa.SendList(phone, resp.Text, truncate(resp.List.ButtonText, 20), toWASections(resp.List.Sections))
	default:
		sendErr = h.wa.SendText(phone, resp.Text)
	}

	if sendErr != nil {
		log.Printf("bot: failed to send reply to %s: %v", phone, sendErr)
	}
}

func toWAButtons(buttons []ai.ButtonOption) []whatsapp.Button {
	// WhatsApp allows max 3 buttons
	if len(buttons) > 3 {
		buttons = buttons[:3]
	}
	wa := make([]whatsapp.Button, len(buttons))
	for i, b := range buttons {
		wa[i] = whatsapp.Button{
			Type:  "reply",
			Reply: whatsapp.ButtonReply{ID: truncate(b.ID, 256), Title: truncate(b.Title, 20)},
		}
	}
	return wa
}

func toWASections(sections []ai.ListSection) []whatsapp.Section {
	wa := make([]whatsapp.Section, len(sections))
	for i, s := range sections {
		rows := make([]whatsapp.SectionRow, len(s.Rows))
		for j, r := range s.Rows {
			rows[j] = whatsapp.SectionRow{
				ID:          truncate(r.ID, 200),
				Title:       truncate(r.Title, 24),
				Description: truncate(r.Description, 72),
			}
		}
		wa[i] = whatsapp.Section{Title: truncate(s.Title, 24), Rows: rows}
	}
	return wa
}

// truncate cuts a string to maxLen, respecting WhatsApp field limits.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
