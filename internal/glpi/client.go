package glpi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL      string
	appToken     string
	adminToken   string
	adminProfile int
	http         *http.Client
}

func NewClient(baseURL, appToken, adminToken string, adminProfile int) *Client {
	return &Client{
		baseURL:      baseURL,
		appToken:     appToken,
		adminToken:   adminToken,
		adminProfile: adminProfile,
		http:         &http.Client{Timeout: 15 * time.Second},
	}
}

// AdminSession creates a session with elevated profile for reading reference data
// (e.g. ITILCategory) that regular self-service users can't access.
func (c *Client) AdminSession() (string, error) {
	if c.adminToken == "" {
		return "", fmt.Errorf("admin token not configured")
	}
	session, err := c.InitSession(c.adminToken)
	if err != nil {
		return "", err
	}
	if c.adminProfile > 0 {
		if err := c.ChangeActiveProfile(session, c.adminProfile); err != nil {
			c.KillSession(session)
			return "", fmt.Errorf("changing to admin profile: %w", err)
		}
	}
	return session, nil
}

// ChangeActiveProfile switches the active profile for a session.
// Reference: POST /apirest.php/changeActiveProfile
func (c *Client) ChangeActiveProfile(sessionToken string, profileID int) error {
	body, err := json.Marshal(map[string]int{"profiles_id": profileID})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/apirest.php/changeActiveProfile", bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("changeActiveProfile request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("changeActiveProfile status %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// InitSession validates a user_token and returns a session_token.
// Reference: nexus_apirest.md — GET /apirest.php/initSession
func (c *Client) InitSession(userToken string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/apirest.php/initSession", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "user_token "+userToken)
	req.Header.Set("App-Token", c.appToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("initSession request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("initSession status %d: %s", resp.StatusCode, body)
	}

	var result InitSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding initSession response: %w", err)
	}
	return result.SessionToken, nil
}

// GetFullSession returns the current session details including user info.
// Reference: nexus_apirest.md — GET /apirest.php/getFullSession
func (c *Client) GetFullSession(sessionToken string) (*FullSession, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/apirest.php/getFullSession", nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getFullSession request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getFullSession status %d: %s", resp.StatusCode, body)
	}

	var result FullSession
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding getFullSession response: %w", err)
	}
	return &result, nil
}

// KillSession ends the current GLPI session.
// Reference: nexus_apirest.md — GET /apirest.php/killSession
func (c *Client) KillSession(sessionToken string) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/apirest.php/killSession", nil)
	if err != nil {
		return err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("killSession request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("killSession status %d: %s", resp.StatusCode, body)
	}
	return nil
}

// GetMyTickets returns tickets assigned to or requested by the current user.
// Reference: nexus_apirest.md — GET /apirest.php/Ticket (with search criteria)
func (c *Client) GetMyTickets(sessionToken string) ([]Ticket, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/apirest.php/Ticket", nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	// mygroups=true returns tickets visible to the current user
	q := req.URL.Query()
	q.Set("is_deleted", "0")
	q.Set("as_map", "0")
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getMyTickets request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getMyTickets status %d: %s", resp.StatusCode, body)
	}

	var tickets []Ticket
	if err := json.NewDecoder(resp.Body).Decode(&tickets); err != nil {
		return nil, fmt.Errorf("decoding tickets: %w", err)
	}
	return tickets, nil
}

func (c *Client) setSessionHeaders(req *http.Request, sessionToken string) {
	req.Header.Set("Session-Token", sessionToken)
	req.Header.Set("App-Token", c.appToken)
	req.Header.Set("Content-Type", "application/json")
}

// setWriteSessionHeaders adds session headers + session_write=true for POST/PUT.
func (c *Client) setWriteSessionHeaders(req *http.Request, sessionToken string) {
	c.setSessionHeaders(req, sessionToken)
	q := req.URL.Query()
	q.Set("session_write", "true")
	req.URL.RawQuery = q.Encode()
}

// GetTicket returns detailed ticket info.
// Reference: nexus_apirest.md — GET /apirest.php/Ticket/:id
func (c *Client) GetTicket(sessionToken string, ticketID int) (*TicketDetail, error) {
	url := fmt.Sprintf("%s/apirest.php/Ticket/%d?expand_dropdowns=true", c.baseURL, ticketID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getTicket request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getTicket status %d: %s", resp.StatusCode, body)
	}

	var ticket TicketDetail
	if err := json.NewDecoder(resp.Body).Decode(&ticket); err != nil {
		return nil, fmt.Errorf("decoding ticket: %w", err)
	}
	return &ticket, nil
}

// SearchTickets uses the GLPI search engine to find tickets.
// Reference: nexus_apirest.md — GET /apirest.php/search/Ticket/
func (c *Client) SearchTickets(sessionToken, query string, userID int) (*SearchResponse, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/apirest.php/search/Ticket/", nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	// Search by name (field 1) containing query, filtered to user's tickets (field 4 = requester, field 22 = recipient)
	q := req.URL.Query()
	q.Set("criteria[0][field]", "1")
	q.Set("criteria[0][searchtype]", "contains")
	q.Set("criteria[0][value]", query)
	q.Set("forcedisplay[0]", "1")
	q.Set("forcedisplay[1]", "2")
	q.Set("forcedisplay[2]", "12")
	q.Set("forcedisplay[3]", "15")
	q.Set("range", "0-19")
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searchTickets request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("searchTickets status %d: %s", resp.StatusCode, body)
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding search results: %w", err)
	}
	return &result, nil
}

// CreateTicket creates a new ticket.
// Reference: nexus_apirest.md — POST /apirest.php/Ticket/
func (c *Client) CreateTicket(sessionToken string, input CreateTicketInput) (int, error) {
	body, err := json.Marshal(glpiInput[CreateTicketInput]{Input: input})
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/apirest.php/Ticket/", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	c.setWriteSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("createTicket request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("createTicket status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding createTicket response: %w", err)
	}
	return result.ID, nil
}

// UpdateTicket updates a ticket (e.g. change status).
// Reference: nexus_apirest.md — PUT /apirest.php/Ticket/:id
func (c *Client) UpdateTicket(sessionToken string, ticketID int, input UpdateTicketInput) error {
	body, err := json.Marshal(glpiInput[UpdateTicketInput]{Input: input})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/apirest.php/Ticket/%d", c.baseURL, ticketID)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setWriteSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("updateTicket request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("updateTicket status %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// AddFollowup adds a followup comment to a ticket.
// Reference: nexus_apirest.md — POST /apirest.php/Ticket/:id/ITILFollowup
func (c *Client) AddFollowup(sessionToken string, ticketID int, content string) (int, error) {
	input := map[string]any{
		"itemtype": "Ticket",
		"items_id": ticketID,
		"content":  content,
	}
	body, err := json.Marshal(glpiInput[map[string]any]{Input: input})
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/apirest.php/ITILFollowup/", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	c.setWriteSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("addFollowup request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("addFollowup status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding addFollowup response: %w", err)
	}
	return result.ID, nil
}

// GetFollowups returns followup comments for a ticket.
// Reference: nexus_apirest.md — GET /apirest.php/Ticket/:id/ITILFollowup
func (c *Client) GetFollowups(sessionToken string, ticketID int) ([]Followup, error) {
	url := fmt.Sprintf("%s/apirest.php/Ticket/%d/ITILFollowup", c.baseURL, ticketID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getFollowups request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getFollowups status %d: %s", resp.StatusCode, body)
	}

	var followups []Followup
	if err := json.NewDecoder(resp.Body).Decode(&followups); err != nil {
		return nil, fmt.Errorf("decoding followups: %w", err)
	}
	return followups, nil
}

// SearchKnowledgeBase searches the GLPI knowledge base.
// Reference: nexus_apirest.md — GET /apirest.php/search/KnowbaseItem/
func (c *Client) SearchKnowledgeBase(sessionToken, query string) (*SearchResponse, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/apirest.php/search/KnowbaseItem/", nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	q := req.URL.Query()
	q.Set("criteria[0][field]", "1")
	q.Set("criteria[0][searchtype]", "contains")
	q.Set("criteria[0][value]", query)
	q.Set("forcedisplay[0]", "1")
	q.Set("forcedisplay[1]", "2")
	q.Set("forcedisplay[2]", "6")
	q.Set("range", "0-9")
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searchKnowledgeBase request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("searchKnowledgeBase status %d: %s", resp.StatusCode, body)
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding KB search results: %w", err)
	}
	return &result, nil
}

// GetKBArticle returns a specific knowledge base article.
// Reference: nexus_apirest.md — GET /apirest.php/KnowbaseItem/:id
func (c *Client) GetKBArticle(sessionToken string, articleID int) (*KBArticle, error) {
	url := fmt.Sprintf("%s/apirest.php/KnowbaseItem/%d", c.baseURL, articleID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getKBArticle request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getKBArticle status %d: %s", resp.StatusCode, body)
	}

	var article KBArticle
	if err := json.NewDecoder(resp.Body).Decode(&article); err != nil {
		return nil, fmt.Errorf("decoding KB article: %w", err)
	}
	return &article, nil
}

// SearchAssets searches for assets of a given type (Computer, Monitor, Printer, etc.).
// Reference: nexus_apirest.md — GET /apirest.php/search/:itemtype/
func (c *Client) SearchAssets(sessionToken, itemtype, query string) (*SearchResponse, error) {
	url := fmt.Sprintf("%s/apirest.php/search/%s/", c.baseURL, itemtype)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	q := req.URL.Query()
	q.Set("criteria[0][field]", "1")
	q.Set("criteria[0][searchtype]", "contains")
	q.Set("criteria[0][value]", query)
	q.Set("forcedisplay[0]", "1")
	q.Set("forcedisplay[1]", "2")
	q.Set("forcedisplay[2]", "31")
	q.Set("range", "0-9")
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searchAssets request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("searchAssets status %d: %s", resp.StatusCode, body)
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding asset search results: %w", err)
	}
	return &result, nil
}

// GetForms returns available FormCreator forms (departments/sectors).
// Reference: GET /apirest.php/PluginFormcreatorForm/
func (c *Client) GetForms(sessionToken string) ([]Form, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/apirest.php/PluginFormcreatorForm/", nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	q := req.URL.Query()
	q.Set("range", "0-99")
	q.Set("searchText[is_active]", "1")
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getForms request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getForms status %d: %s", resp.StatusCode, body)
	}

	var forms []Form
	if err := json.NewDecoder(resp.Body).Decode(&forms); err != nil {
		return nil, fmt.Errorf("decoding forms: %w", err)
	}
	return forms, nil
}

// GetFormSections returns the sections of a FormCreator form.
// Reference: GET /apirest.php/PluginFormcreatorForm/:id/PluginFormcreatorSection
func (c *Client) GetFormSections(sessionToken string, formID int) ([]FormSection, error) {
	url := fmt.Sprintf("%s/apirest.php/PluginFormcreatorForm/%d/PluginFormcreatorSection", c.baseURL, formID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getFormSections request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getFormSections status %d: %s", resp.StatusCode, body)
	}

	var sections []FormSection
	if err := json.NewDecoder(resp.Body).Decode(&sections); err != nil {
		return nil, fmt.Errorf("decoding form sections: %w", err)
	}
	return sections, nil
}

// GetSectionQuestions returns the questions of a FormCreator section.
// Reference: GET /apirest.php/PluginFormcreatorSection/:id/PluginFormcreatorQuestion
func (c *Client) GetSectionQuestions(sessionToken string, sectionID int) ([]FormQuestion, error) {
	url := fmt.Sprintf("%s/apirest.php/PluginFormcreatorSection/%d/PluginFormcreatorQuestion", c.baseURL, sectionID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getSectionQuestions request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getSectionQuestions status %d: %s", resp.StatusCode, body)
	}

	var questions []FormQuestion
	if err := json.NewDecoder(resp.Body).Decode(&questions); err != nil {
		return nil, fmt.Errorf("decoding section questions: %w", err)
	}
	return questions, nil
}

// GetTargetTickets returns FormCreator target tickets for a given form.
// Reference: GET /apirest.php/PluginFormcreatorTargetTicket/
func (c *Client) GetTargetTickets(sessionToken string, formID int) ([]TargetTicket, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/apirest.php/PluginFormcreatorTargetTicket/", nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	q := req.URL.Query()
	q.Set("searchText[plugin_formcreator_forms_id]", fmt.Sprintf("%d", formID))
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getTargetTickets request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getTargetTickets status %d: %s", resp.StatusCode, body)
	}

	var targets []TargetTicket
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("decoding target tickets: %w", err)
	}
	return targets, nil
}

// GetTargetActors returns actors configured for a FormCreator target ticket.
// Reference: GET /apirest.php/PluginFormcreatorTargetTicket/:id/PluginFormcreatorTarget_Actor
func (c *Client) GetTargetActors(sessionToken string, targetID int) ([]TargetActor, error) {
	url := fmt.Sprintf("%s/apirest.php/PluginFormcreatorTargetTicket/%d/PluginFormcreatorTarget_Actor", c.baseURL, targetID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getTargetActors request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getTargetActors status %d: %s", resp.StatusCode, body)
	}

	var actors []TargetActor
	if err := json.NewDecoder(resp.Body).Decode(&actors); err != nil {
		return nil, fmt.Errorf("decoding target actors: %w", err)
	}
	return actors, nil
}

// GetTicketTasks returns tasks for a ticket.
// Reference: GET /apirest.php/Ticket/:id/TicketTask
func (c *Client) GetTicketTasks(sessionToken string, ticketID int) ([]TicketTask, error) {
	url := fmt.Sprintf("%s/apirest.php/Ticket/%d/TicketTask", c.baseURL, ticketID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getTicketTasks request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getTicketTasks status %d: %s", resp.StatusCode, body)
	}

	var tasks []TicketTask
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("decoding ticket tasks: %w", err)
	}
	return tasks, nil
}

// AddTicketTask creates a task on a ticket.
// Reference: POST /apirest.php/TicketTask/
func (c *Client) AddTicketTask(sessionToken string, ticketID int, content string, state int) (int, error) {
	input := map[string]any{
		"tickets_id": ticketID,
		"content":    content,
		"state":      state,
	}
	body, err := json.Marshal(glpiInput[map[string]any]{Input: input})
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/apirest.php/TicketTask/", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	c.setWriteSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("addTicketTask request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("addTicketTask status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding addTicketTask response: %w", err)
	}
	return result.ID, nil
}

// GetTicketValidations returns approval requests for a ticket.
// Reference: GET /apirest.php/Ticket/:id/TicketValidation
func (c *Client) GetTicketValidations(sessionToken string, ticketID int) ([]TicketValidation, error) {
	url := fmt.Sprintf("%s/apirest.php/Ticket/%d/TicketValidation", c.baseURL, ticketID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getTicketValidations request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getTicketValidations status %d: %s", resp.StatusCode, body)
	}

	var validations []TicketValidation
	if err := json.NewDecoder(resp.Body).Decode(&validations); err != nil {
		return nil, fmt.Errorf("decoding ticket validations: %w", err)
	}
	return validations, nil
}

// RespondTicketValidation approves or refuses a validation request.
// Reference: PUT /apirest.php/TicketValidation/:id
func (c *Client) RespondTicketValidation(sessionToken string, validationID int, approve bool, comment string) error {
	status := 3 // Refused
	if approve {
		status = 2 // Approved
	}
	input := map[string]any{
		"status":             status,
		"comment_validation": comment,
	}
	body, err := json.Marshal(glpiInput[map[string]any]{Input: input})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/apirest.php/TicketValidation/%d", c.baseURL, validationID)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setWriteSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("respondTicketValidation request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("respondTicketValidation status %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// GetTicketSatisfaction returns the satisfaction survey for a ticket.
// Reference: GET /apirest.php/Ticket/:id/TicketSatisfaction
func (c *Client) GetTicketSatisfaction(sessionToken string, ticketID int) (*TicketSatisfaction, error) {
	url := fmt.Sprintf("%s/apirest.php/Ticket/%d/TicketSatisfaction", c.baseURL, ticketID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getTicketSatisfaction request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getTicketSatisfaction status %d: %s", resp.StatusCode, body)
	}

	var surveys []TicketSatisfaction
	if err := json.NewDecoder(resp.Body).Decode(&surveys); err != nil {
		return nil, fmt.Errorf("decoding ticket satisfaction: %w", err)
	}
	if len(surveys) == 0 {
		return nil, nil
	}
	return &surveys[0], nil
}

// RateTicketSatisfaction submits a satisfaction rating for a ticket.
// Reference: PUT /apirest.php/TicketSatisfaction/:id
func (c *Client) RateTicketSatisfaction(sessionToken string, satisfactionID int, rating int, comment string) error {
	input := map[string]any{
		"satisfaction": rating,
		"comment":      comment,
	}
	body, err := json.Marshal(glpiInput[map[string]any]{Input: input})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/apirest.php/TicketSatisfaction/%d", c.baseURL, satisfactionID)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setWriteSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("rateTicketSatisfaction request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("rateTicketSatisfaction status %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// GetTicketLogs returns the change history for a ticket.
// Reference: GET /apirest.php/Ticket/:id/Log
func (c *Client) GetTicketLogs(sessionToken string, ticketID int) ([]LogEntry, error) {
	url := fmt.Sprintf("%s/apirest.php/Ticket/%d/Log?range=0-24", c.baseURL, ticketID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getTicketLogs request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getTicketLogs status %d: %s", resp.StatusCode, body)
	}

	var logs []LogEntry
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return nil, fmt.Errorf("decoding ticket logs: %w", err)
	}
	return logs, nil
}

// AdvancedSearchTickets searches tickets with multiple criteria.
// Reference: GET /apirest.php/search/Ticket/
func (c *Client) AdvancedSearchTickets(sessionToken string, criteria map[string]string) (*SearchResponse, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/apirest.php/search/Ticket/", nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	q := req.URL.Query()
	for k, v := range criteria {
		q.Set(k, v)
	}
	// Always show useful fields: ID, Name, Status, Date, Urgency, Priority, Category, Assigned
	q.Set("forcedisplay[0]", "2")  // ID
	q.Set("forcedisplay[1]", "1")  // Name
	q.Set("forcedisplay[2]", "12") // Status
	q.Set("forcedisplay[3]", "15") // Date
	q.Set("forcedisplay[4]", "10") // Urgency
	q.Set("forcedisplay[5]", "3")  // Priority
	q.Set("forcedisplay[6]", "7")  // Category
	q.Set("forcedisplay[7]", "5")  // Assigned
	if _, ok := criteria["range"]; !ok {
		q.Set("range", "0-19")
	}
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("advancedSearchTickets request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("advancedSearchTickets status %d: %s", resp.StatusCode, body)
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding advanced search results: %w", err)
	}
	return &result, nil
}

// GetCategories returns ITIL ticket categories filtered by parent.
// parentID=0 returns root categories (departments), parentID>0 returns sub-categories.
// Uses the list endpoint with searchText filter on itilcategories_id.
// Reference: nexus_apirest.md — GET /apirest.php/ITILCategory/
func (c *Client) GetCategories(sessionToken string, parentID int) ([]ITILCategory, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/apirest.php/ITILCategory/", nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	q := req.URL.Query()
	q.Set("searchText[itilcategories_id]", fmt.Sprintf("%d", parentID))
	q.Set("range", "0-49")
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getCategories request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getCategories status %d: %s", resp.StatusCode, body)
	}

	var categories []ITILCategory
	if err := json.NewDecoder(resp.Body).Decode(&categories); err != nil {
		return nil, fmt.Errorf("decoding categories: %w", err)
	}
	return categories, nil
}
