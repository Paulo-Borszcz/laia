package glpi

type InitSessionResponse struct {
	SessionToken string `json:"session_token"`
}

// FullSession is returned by GET /apirest.php/getFullSession
type FullSession struct {
	Session SessionInfo `json:"session"`
}

type SessionInfo struct {
	GlpiID           int    `json:"glpiID"`
	GlpiName         string `json:"glpiname"`
	GlpiFriendlyName string `json:"glpifriendlyname"`
}

type Ticket struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Status      int    `json:"status"`
	DateCreated string `json:"date"`
	DateMod     string `json:"date_mod"`
}

// TicketDetail has extra fields returned with expand_dropdowns=true.
type TicketDetail struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	Content          string `json:"content"`
	Status           int    `json:"status"`
	Urgency          int    `json:"urgency"`
	Priority         int    `json:"priority"`
	Type             int    `json:"type"`
	UsersIDRecipient any    `json:"users_id_recipient"`
	DateCreated      string `json:"date"`
	DateMod          string `json:"date_mod"`
	SolveDate        string `json:"solvedate"`
	CloseDate        string `json:"closedate"`
	ITILCategoriesID any    `json:"itilcategories_id"`
}

type Followup struct {
	ID          int    `json:"id"`
	Content     string `json:"content"`
	DateCreated string `json:"date"`
	UsersID     int    `json:"users_id"`
}

type CreateTicketInput struct {
	Name             string `json:"name"`
	Content          string `json:"content"`
	ITILCategoriesID int    `json:"itilcategories_id,omitempty"`
	Urgency          int    `json:"urgency,omitempty"`
	Type             int    `json:"type,omitempty"`
}

type UpdateTicketInput struct {
	Status int `json:"status,omitempty"`
}

// SearchResponse is the envelope returned by GET /search/:itemtype/
type SearchResponse struct {
	TotalCount int                `json:"totalcount"`
	Data       []SearchResultItem `json:"data"`
}

// SearchResultItem holds searchoption IDs → values (all as any since GLPI mixes types).
type SearchResultItem map[string]any

type KBArticle struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Answer  string `json:"answer"`
	DateMod string `json:"date_mod"`
}

type ITILCategory struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	Completename     string `json:"completename"`
	ITILCategoriesID int    `json:"itilcategories_id"`
}

// FormCreator models — plugin PluginFormcreator
// Reference: https://github.com/pluginsGLPI/formcreator

type Form struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type FormSection struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type FormQuestion struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	FieldType   string `json:"fieldtype"`
	ItemType    string `json:"itemtype"`
	Values      string `json:"values"`
	Description string `json:"description"`
}

// glpiInput wraps a value in the {"input": ...} envelope required by GLPI POST/PUT.
type glpiInput[T any] struct {
	Input T `json:"input"`
}
