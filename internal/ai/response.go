package ai

// Response represents the AI agent's structured reply.
// It may contain interactive elements (buttons or lists) for WhatsApp.
type Response struct {
	Text    string
	Buttons []ButtonOption
	List    *ListOption
}

type ButtonOption struct {
	ID    string // Unique identifier for callback
	Title string // Max 20 chars (WhatsApp limit)
}

type ListOption struct {
	ButtonText string // Text on the button that opens the list (max 20 chars)
	Sections   []ListSection
}

type ListSection struct {
	Title string // Max 24 chars
	Rows  []ListRow
}

type ListRow struct {
	ID          string
	Title       string // Max 24 chars
	Description string // Max 72 chars
}
