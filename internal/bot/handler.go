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
	msg := fmt.Sprintf(
		"Olá! Para usar este serviço, primeiro vincule seu WhatsApp ao Nexus.\n\nAcesse: %s",
		link,
	)
	if err := h.wa.SendText(phone, msg); err != nil {
		log.Printf("bot: failed to send verification link to %s: %v", phone, err)
	}
}

func (h *Handler) handleCommand(user *store.User, phone, text string) {
	ctx := context.Background()
	reply, err := h.agent.Handle(ctx, user, phone, text)
	if err != nil {
		log.Printf("bot: agent error for %s: %v", phone, err)
		h.wa.SendText(phone, "Desculpe, ocorreu um erro ao processar sua mensagem. Tente novamente mais tarde.")
		return
	}

	if err := h.wa.SendText(phone, reply); err != nil {
		log.Printf("bot: failed to send reply to %s: %v", phone, err)
	}
}
