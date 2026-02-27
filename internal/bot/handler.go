package bot

import (
	"context"
	"fmt"
	"log"

	"github.com/lojasmm/laia/internal/ai"
	"github.com/lojasmm/laia/internal/store"
	"github.com/lojasmm/laia/internal/whatsapp"
)

type Handler struct {
	wa      *whatsapp.Client
	store   store.Store
	authURL string
	agent   *ai.Agent
}

func NewHandler(wa *whatsapp.Client, s store.Store, authURL string, agent *ai.Agent) *Handler {
	return &Handler{wa: wa, store: s, authURL: authURL, agent: agent}
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
	body := "Olá! Eu sou a *Laia*, sua assistente virtual do *Nexus* aqui nas Lojas MM.\n\n" +
		"Comigo você pode:\n" +
		"• Abrir e acompanhar chamados\n" +
		"• Consultar a base de conhecimento\n" +
		"• Verificar seus ativos de TI\n\n" +
		"Para começarmos, preciso vincular seu WhatsApp à sua conta do Nexus. " +
		"É rápido — basta clicar no botão abaixo!"

	if err := h.wa.SendCTAButton(phone, body, "Vincular conta", link); err != nil {
		log.Printf("bot: failed to send verification link to %s: %v", phone, err)
	}
}

func (h *Handler) handleCommand(user *store.User, phone, text string) {
	ctx := context.Background()
	resp, err := h.agent.Handle(ctx, user, phone, text)
	if err != nil {
		log.Printf("bot: agent error for %s: %v", phone, err)
		h.wa.SendText(phone, "Desculpe, ocorreu um erro ao processar sua mensagem. Tente novamente mais tarde.")
		return
	}

	var sendErr error
	switch {
	case len(resp.Buttons) > 0:
		sendErr = h.wa.SendInteractiveButtons(phone, resp.Text, toWAButtons(resp.Buttons))
	case resp.List != nil:
		sendErr = h.wa.SendList(phone, resp.Text, resp.List.ButtonText, toWASections(resp.List.Sections))
	default:
		sendErr = h.wa.SendText(phone, resp.Text)
	}

	if sendErr != nil {
		log.Printf("bot: failed to send reply to %s: %v", phone, sendErr)
	}
}

func toWAButtons(buttons []ai.ButtonOption) []whatsapp.Button {
	wa := make([]whatsapp.Button, len(buttons))
	for i, b := range buttons {
		wa[i] = whatsapp.Button{
			Type:  "reply",
			Reply: whatsapp.ButtonReply{ID: b.ID, Title: b.Title},
		}
	}
	return wa
}

func toWASections(sections []ai.ListSection) []whatsapp.Section {
	wa := make([]whatsapp.Section, len(sections))
	for i, s := range sections {
		rows := make([]whatsapp.SectionRow, len(s.Rows))
		for j, r := range s.Rows {
			rows[j] = whatsapp.SectionRow{ID: r.ID, Title: r.Title, Description: r.Description}
		}
		wa[i] = whatsapp.Section{Title: s.Title, Rows: rows}
	}
	return wa
}
