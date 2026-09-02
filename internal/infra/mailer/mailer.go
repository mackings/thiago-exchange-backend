// Package mailer sends transactional emails via Resend's HTTP API. It's a
// no-op — Send just returns nil without making a request — whenever
// RESEND_API_KEY isn't configured, so the app runs fine without it.
//
// This replaced a raw Gmail SMTP sender: SMTP errors from that
// implementation were being silently discarded by every caller (they all
// fire Send in a goroutine and ignore the error, since a slow/failed email
// should never block the actual request), so a misconfigured or
// Gmail-blocked send failed completely invisibly — the request looked
// successful, log line and Bybit-outreach flavor text and all, and nothing
// arrived. Send here logs a failure instead of just returning it, so that
// class of bug is at least visible in the server log even though callers
// still don't block on it.
package mailer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type Mailer struct {
	apiKey string
	from   string
	client *http.Client
}

func New(apiKey, from string) *Mailer {
	return &Mailer{apiKey: apiKey, from: from, client: &http.Client{Timeout: 10 * time.Second}}
}

func (m *Mailer) Configured() bool {
	return m.apiKey != ""
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

// Send fires a plain-text email through Resend. Callers that don't want a
// failed send to block a request (e.g. registration) should run this in a
// goroutine — which is every caller today, so a failure here is logged
// (see package doc) rather than silently lost.
func (m *Mailer) Send(to, subject, body string) error {
	if !m.Configured() {
		return nil
	}
	payload, err := json.Marshal(resendRequest{From: m.from, To: []string{to}, Subject: subject, Text: body})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		log.Printf("resend: send to %s failed: %v", to, err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("resend: %d %s", resp.StatusCode, string(respBody))
		log.Printf("resend: send to %s failed: %v", to, err)
		return err
	}
	return nil
}
