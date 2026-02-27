package whatsapp

// --- Incoming webhook payload ---
// Reference: https://developers.facebook.com/docs/whatsapp/cloud-api/webhooks/components

type WebhookPayload struct {
	Object string  `json:"object"`
	Entry  []Entry `json:"entry"`
}

type Entry struct {
	ID      string   `json:"id"`
	Changes []Change `json:"changes"`
}

type Change struct {
	Value ChangeValue `json:"value"`
	Field string      `json:"field"`
}

type ChangeValue struct {
	MessagingProduct string    `json:"messaging_product"`
	Metadata         Metadata  `json:"metadata"`
	Contacts         []Contact `json:"contacts"`
	Messages         []Message `json:"messages"`
	Statuses         []Status  `json:"statuses"`
}

type Metadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type Contact struct {
	Profile Profile `json:"profile"`
	WaID    string  `json:"wa_id"`
}

type Profile struct {
	Name string `json:"name"`
}

type Message struct {
	From        string              `json:"from"`
	ID          string              `json:"id"`
	Timestamp   string              `json:"timestamp"`
	Type        string              `json:"type"`
	Text        *TextContent        `json:"text,omitempty"`
	Interactive *InteractiveContent `json:"interactive,omitempty"`
}

// InteractiveContent represents a user's reply to an interactive message (button or list).
// Reference: https://developers.facebook.com/docs/whatsapp/cloud-api/webhooks/components#messages-object
type InteractiveContent struct {
	Type        string          `json:"type"`
	ButtonReply *ButtonReplyMsg `json:"button_reply,omitempty"`
	ListReply   *ListReplyMsg   `json:"list_reply,omitempty"`
}

type ButtonReplyMsg struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ListReplyMsg struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type TextContent struct {
	Body string `json:"body"`
}

type Status struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Timestamp   string `json:"timestamp"`
	RecipientID string `json:"recipient_id"`
}

// --- Outgoing send message ---
// Reference: https://developers.facebook.com/docs/whatsapp/cloud-api/messages

type SendMessageRequest struct {
	MessagingProduct string      `json:"messaging_product"`
	RecipientType    string      `json:"recipient_type"`
	To               string      `json:"to"`
	Type             string      `json:"type"`
	Text             *SendText   `json:"text,omitempty"`
	Interactive      *Interactive `json:"interactive,omitempty"`
}

type SendText struct {
	PreviewURL bool   `json:"preview_url"`
	Body       string `json:"body"`
}

type Interactive struct {
	Type   string            `json:"type"`
	Body   InteractiveBody   `json:"body"`
	Action InteractiveAction `json:"action"`
}

type InteractiveBody struct {
	Text string `json:"text"`
}

type InteractiveAction struct {
	Buttons  []Button  `json:"buttons,omitempty"`
	Button   string    `json:"button,omitempty"`
	Sections []Section `json:"sections,omitempty"`
}

// Section and SectionRow support list-type interactive messages.
// Reference: https://developers.facebook.com/docs/whatsapp/cloud-api/messages/interactive-list-messages
type Section struct {
	Title string       `json:"title"`
	Rows  []SectionRow `json:"rows"`
}

type SectionRow struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type Button struct {
	Type  string      `json:"type"`
	Reply ButtonReply `json:"reply"`
}

type ButtonReply struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
