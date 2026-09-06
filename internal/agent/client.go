// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zyvorai/fleet/internal/model"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
	Version string
}

var Version = "dev"

func NewClient(base string) *Client {
	return &Client{BaseURL: strings.TrimRight(base, "/"), HTTP: &http.Client{Timeout: 12 * time.Second}, Version: Version}
}

type Registration struct {
	SiteID           string `json:"siteId"`
	AgentToken       string `json:"agentToken"`
	HeartbeatSeconds int    `json:"heartbeatSeconds"`
}

func (c *Client) Register(ctx context.Context, enroll, name, region string, labels map[string]string) (Registration, error) {
	var out Registration
	body := map[string]any{"name": name, "region": region, "labels": labels, "agentVersion": c.Version, "inventory": Inventory()}
	err := c.do(ctx, http.MethodPost, "/api/v1/agent/register", enroll, "", body, &out)
	return out, err
}
func (c *Client) Heartbeat(ctx context.Context, siteID, token, applied, failedRevision, revisionError string, queued int, autonomy bool, health []model.WorkloadHealth, events []model.Event) error {
	body := map[string]any{"inventory": Inventory(), "agentVersion": c.Version, "appliedRevision": applied, "failedRevision": failedRevision, "revisionError": revisionError, "queuedEvents": queued, "autonomyMode": autonomy, "workloadHealth": health, "events": events}
	return c.do(ctx, http.MethodPost, "/api/v1/agent/heartbeat", token, siteID, body, &struct{}{})
}

type SyncResponse struct {
	SiteID, DesiredRevision string
	Revision                *model.Revision `json:"revision"`
	Commands                []model.Command `json:"commands"`
	SyncAfterSeconds        int             `json:"syncAfterSeconds"`
}

func (c *Client) Sync(ctx context.Context, siteID, token string) (SyncResponse, error) {
	var out SyncResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/agent/sync", token, siteID, nil, &out)
	return out, err
}
func (c *Client) Ack(ctx context.Context, siteID, token, commandID, status, errText, applied string) error {
	body := map[string]any{"commandId": commandID, "status": status, "error": errText, "appliedRevision": applied}
	return c.do(ctx, http.MethodPost, "/api/v1/agent/ack", token, siteID, body, &struct{}{})
}
func (c *Client) do(ctx context.Context, method, path, token, siteID string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if siteID != "" {
		req.Header.Set("X-Zyvor-Site-ID", siteID)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(b))
		}
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, e.Error)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
