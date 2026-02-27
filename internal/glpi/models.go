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
	UsersIDRequester int    `json:"_users_id_requester,omitempty"`
	UsersIDAssign    []int  `json:"_users_id_assign,omitempty"`
	GroupsIDAssign   []int  `json:"_groups_id_assign,omitempty"`
	UsersIDObserver  []int  `json:"_users_id_observer,omitempty"`
	GroupsIDObserver []int  `json:"_groups_id_observer,omitempty"`
}

// TargetTicket is a FormCreator target that defines how a ticket is created from a form.
type TargetTicket struct {
	ID                       int `json:"id"`
	PluginFormcreatorFormsID int `json:"plugin_formcreator_forms_id"`
}

// TargetActor defines an actor (requester/assigned/observer) for a target ticket.
type TargetActor struct {
	ID         int `json:"id"`
	ActorRole  int `json:"actor_role"`  // 1=Requester, 2=Assigned, 3=Observer
	ActorType  int `json:"actor_type"`  // 1=Creator, 3=Specific user, 5=Specific group
	ActorValue int `json:"actor_value"` // user/group ID (0 for Creator type)
}

type UpdateTicketInput struct {
	Name             string `json:"name,omitempty"`
	Content          string `json:"content,omitempty"`
	Status           int    `json:"status,omitempty"`
	Urgency          int    `json:"urgency,omitempty"`
	Priority         int    `json:"priority,omitempty"`
	ITILCategoriesID int    `json:"itilcategories_id,omitempty"`
	Type             int    `json:"type,omitempty"`
}

type TicketTask struct {
	ID          int    `json:"id"`
	Content     string `json:"content"`
	State       int    `json:"state"`
	UsersID     int    `json:"users_id"`
	DateCreated string `json:"date"`
	Actiontime  int    `json:"actiontime"`
	PercentDone int    `json:"percent_done"`
}

type TicketValidation struct {
	ID                int    `json:"id"`
	UsersIDValidate   int    `json:"users_id_validate"`
	Status            int    `json:"status"`
	CommentSubmission string `json:"comment_submission"`
	CommentValidation string `json:"comment_validation"`
	DateCreated       string `json:"submission_date"`
}

type TicketSatisfaction struct {
	ID           int    `json:"id"`
	TicketsID    int    `json:"tickets_id"`
	Satisfaction int    `json:"satisfaction"`
	Comment      string `json:"comment"`
	DateAnswered string `json:"date_answered"`
	DateCreated  string `json:"date_begin"`
}

type LogEntry struct {
	ID          int    `json:"id"`
	DateMod     string `json:"date_mod"`
	UsersName   string `json:"user_name"`
	LinkedField string `json:"id_search_option"`
	OldValue    string `json:"old_value"`
	NewValue    string `json:"new_value"`
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
