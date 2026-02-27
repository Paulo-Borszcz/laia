package glpi

import (
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
