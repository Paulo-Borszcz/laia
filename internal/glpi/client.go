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
	baseURL  string
	appToken string
	http     *http.Client
}

func NewClient(baseURL, appToken string) *Client {
	return &Client{
		baseURL:  baseURL,
		appToken: appToken,
		http:     &http.Client{Timeout: 15 * time.Second},
	}
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

// GetCategories returns ITIL ticket categories filtered by parent.
// parentID=0 returns root categories (departments), parentID>0 returns sub-categories.
// Reference: nexus_apirest.md — GET /apirest.php/search/ITILCategory/
func (c *Client) GetCategories(sessionToken string, parentID int) ([]ITILCategory, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/apirest.php/search/ITILCategory/", nil)
	if err != nil {
		return nil, err
	}
	c.setSessionHeaders(req, sessionToken)

	q := req.URL.Query()
	// Field 3 = itilcategories_id (parent category)
	q.Set("criteria[0][field]", "3")
	q.Set("criteria[0][searchtype]", "equals")
	q.Set("criteria[0][value]", fmt.Sprintf("%d", parentID))
	// Display: id (2), name (1), completename (14)
	q.Set("forcedisplay[0]", "2")
	q.Set("forcedisplay[1]", "1")
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

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding categories search: %w", err)
	}

	categories := make([]ITILCategory, 0, len(result.Data))
	for _, row := range result.Data {
		cat := ITILCategory{}
		if v, ok := row["2"]; ok {
			if f, ok := v.(float64); ok {
				cat.ID = int(f)
			}
		}
		if v, ok := row["1"]; ok {
			if s, ok := v.(string); ok {
				cat.Name = s
			}
		}
		categories = append(categories, cat)
	}
	return categories, nil
}
