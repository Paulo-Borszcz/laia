package glpi

type InitSessionResponse struct {
	SessionToken string `json:"session_token"`
}

// FullSession is returned by GET /apirest.php/getFullSession
type FullSession struct {
	Session SessionInfo `json:"session"`
}

type SessionInfo struct {
	// GLPI user ID (glpiID)
	GlpiID          int    `json:"glpiID"`
	GlpiName        string `json:"glpiname"`
	GlpiFriendlyName string `json:"glpifriendlyname"`
}

type Ticket struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Status      int    `json:"status"`
	DateCreated string `json:"date"`
	DateMod     string `json:"date_mod"`
}
