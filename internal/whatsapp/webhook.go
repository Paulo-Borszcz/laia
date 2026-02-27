package whatsapp

import (
	"encoding/json"
	"log"
	"net/http"
)

// MessageHandler is called for each incoming message with (senderPhone, messageID, messageBody).
type MessageHandler func(phone, messageID, text string)

type WebhookHandler struct {
	verifyToken string
	onMessage   MessageHandler
}

func NewWebhookHandler(verifyToken string, onMessage MessageHandler) *WebhookHandler {
	return &WebhookHandler{
		verifyToken: verifyToken,
		onMessage:   onMessage,
	}
}

// HandleVerify handles the GET webhook verification from Meta.
// Reference: https://developers.facebook.com/docs/whatsapp/cloud-api/get-started#webhook-verification
func (h *WebhookHandler) HandleVerify(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == h.verifyToken {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(challenge))
		return
	}

	http.Error(w, "Forbidden", http.StatusForbidden)
}

// HandleIncoming processes incoming webhook POST notifications.
// Reference: https://developers.facebook.com/docs/whatsapp/cloud-api/webhooks/components
func (h *WebhookHandler) HandleIncoming(w http.ResponseWriter, r *http.Request) {
	var payload WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("webhook: failed to decode payload: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Meta requires 200 OK quickly; processing happens here synchronously for simplicity.
	// TODO: move to async processing if latency becomes an issue
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			for _, msg := range change.Value.Messages {
				switch msg.Type {
				case "text":
					if msg.Text != nil {
						h.onMessage(msg.From, msg.ID, msg.Text.Body)
					}
				case "interactive":
					if msg.Interactive != nil {
						switch msg.Interactive.Type {
						case "button_reply":
							if msg.Interactive.ButtonReply != nil {
								h.onMessage(msg.From, msg.ID, msg.Interactive.ButtonReply.Title)
							}
						case "list_reply":
							if msg.Interactive.ListReply != nil {
								h.onMessage(msg.From, msg.ID, msg.Interactive.ListReply.Title)
							}
						}
					}
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}
