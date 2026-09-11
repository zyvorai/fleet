// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/zyvorai/fleet/internal/model"
)

// deviceAgentProjection mirrors the subset of Zyvor Device Agent's
// FleetInventoryProjection (GET /api/v1/integrations/fleet/inventory) that is
// safe to merge in: hostname/os/arch/kernel/cpu/capabilities use a different
// vocabulary than Fleet's own detection and risk silent divergence if
// overlaid, so only the already-namespaced (zyvor.*) metadata map is used.
type deviceAgentProjection struct {
	Metadata map[string]string `json:"metadata"`
}

// enrichWithDeviceAgent merges a local Zyvor Device Agent's hardware metadata
// into inv when DeviceAgentURL is configured. Never fails the caller: an
// unreachable or disabled Device Agent just means inv is returned unchanged,
// so a Register/Heartbeat call is never blocked by this being opt-in and
// possibly absent (most non-Minewing Fleet deployments have no Device Agent
// at all).
func (c *Client) enrichWithDeviceAgent(ctx context.Context, inv model.Inventory) model.Inventory {
	if c.DeviceAgentURL == "" {
		return inv
	}
	proj, err := c.fetchDeviceAgentProjection(ctx)
	if err != nil {
		if c.Logger != nil {
			c.Logger.Debug("device-agent inventory fetch skipped", "url", c.DeviceAgentURL, "error", err)
		}
		return inv
	}
	return mergeDeviceAgentProjection(inv, proj)
}

func (c *Client) fetchDeviceAgentProjection(ctx context.Context) (deviceAgentProjection, error) {
	var out deviceAgentProjection
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.DeviceAgentURL+"/api/v1/integrations/fleet/inventory", nil)
	if err != nil {
		return out, err
	}
	if c.DeviceAgentToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.DeviceAgentToken)
	}
	client := c.DeviceAgentHTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return out, fmt.Errorf("device agent returned %d: %s", resp.StatusCode, string(b))
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

func mergeDeviceAgentProjection(inv model.Inventory, proj deviceAgentProjection) model.Inventory {
	if inv.Metadata == nil {
		inv.Metadata = map[string]string{}
	}
	for k, v := range proj.Metadata {
		inv.Metadata[k] = v
	}
	inv.Metadata["zyvor.device_agent.reachable"] = "true"
	return inv
}
