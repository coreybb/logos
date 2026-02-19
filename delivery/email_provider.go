package delivery

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const sendgridMailEndpoint = "https://api.sendgrid.com/v3/mail/send"

// EmailDeliveryProvider sends ebook files as email attachments via SendGrid.
type EmailDeliveryProvider struct {
	apiKey    string
	fromEmail string
	fromName  string
	client    *http.Client
}

func NewEmailDeliveryProvider(apiKey, fromEmail, fromName string) *EmailDeliveryProvider {
	return &EmailDeliveryProvider{
		apiKey:    apiKey,
		fromEmail: fromEmail,
		fromName:  fromName,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *EmailDeliveryProvider) Type() string { return "email" }

func (p *EmailDeliveryProvider) Deliver(ctx context.Context, filePath string, fileName string, recipientAddress string) error {
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read ebook file %s: %w", filePath, err)
	}

	encoded := base64.StdEncoding.EncodeToString(fileBytes)
	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	payload := sgMailPayload{
		Personalizations: []sgPersonalization{{
			To: []sgAddress{{Email: recipientAddress}},
		}},
		From:    sgAddress{Email: p.fromEmail, Name: p.fromName},
		Subject: fileName,
		Content: []sgContent{{Type: "text/plain", Value: "Your ebook is attached."}},
		Attachments: []sgAttachment{{
			Content:  encoded,
			Type:     contentType,
			Filename: fileName,
		}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal SendGrid payload: %w", err)
	}

	// Retry up to 3 times with backoff for transient network errors
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			log.Printf("INFO (EmailDeliveryProvider): Retry attempt %d for SendGrid delivery", attempt)
			time.Sleep(time.Duration(attempt*5) * time.Second)
		}

		lastErr = p.sendRequest(ctx, body)
		if lastErr == nil {
			return nil
		}
		log.Printf("WARN (EmailDeliveryProvider): Attempt %d failed: %v", attempt+1, lastErr)
	}

	return lastErr
}

func (p *EmailDeliveryProvider) sendRequest(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendgridMailEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create SendGrid request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("SendGrid request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SendGrid returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// SendGrid v3 Mail Send API payload types.
type sgMailPayload struct {
	Personalizations []sgPersonalization `json:"personalizations"`
	From             sgAddress           `json:"from"`
	Subject          string              `json:"subject"`
	Content          []sgContent         `json:"content"`
	Attachments      []sgAttachment      `json:"attachments"`
}

type sgPersonalization struct {
	To []sgAddress `json:"to"`
}

type sgAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type sgContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type sgAttachment struct {
	Content  string `json:"content"`
	Type     string `json:"type"`
	Filename string `json:"filename"`
}
