package whatsapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiURL = "https://graph.facebook.com/v21.0"

type Client struct {
	phoneNumberID string
	accessToken   string
	http          *http.Client
}

func NewClient(phoneNumberID, accessToken string) *Client {
	return &Client{
		phoneNumberID: phoneNumberID,
		accessToken:   accessToken,
		http:          &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) SendText(to, body string) error {
	msg := SendMessageRequest{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               to,
		Type:             "text",
		Text:             &SendText{Body: body},
	}
	return c.send(msg)
}

func (c *Client) SendInteractiveButtons(to, body string, buttons []Button) error {
	msg := SendMessageRequest{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               to,
		Type:             "interactive",
		Interactive: &Interactive{
			Type: "button",
			Body: InteractiveBody{Text: body},
			Action: InteractiveAction{Buttons: buttons},
		},
	}
	return c.send(msg)
}

func (c *Client) SendList(to, body, buttonText string, sections []Section) error {
	msg := SendMessageRequest{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               to,
		Type:             "interactive",
		Interactive: &Interactive{
			Type: "list",
			Body: InteractiveBody{Text: body},
			Action: InteractiveAction{
				Button:   buttonText,
				Sections: sections,
			},
		},
	}
	return c.send(msg)
}

func (c *Client) send(msg SendMessageRequest) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling message: %w", err)
	}

	url := fmt.Sprintf("%s/%s/messages", apiURL, c.phoneNumberID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sending message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("whatsapp API status %d: %s", resp.StatusCode, respBody)
	}
	return nil
}
