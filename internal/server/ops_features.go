// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zyvorai/fleet/internal/auth"
	"github.com/zyvorai/fleet/internal/model"
	"github.com/zyvorai/fleet/internal/ops"
)

var (
	webhookDispatchMu sync.Mutex
	webhookClient     = &http.Client{Timeout: 5 * time.Second}
)

func (s *Server) listAPITokens(w http.ResponseWriter, _ *http.Request) {
	st := s.store.Snapshot()
	type view struct {
		ID         string     `json:"id"`
		Name       string     `json:"name"`
		Role       model.Role `json:"role"`
		Scopes     []string   `json:"scopes"`
		CreatedAt  time.Time  `json:"createdAt"`
		ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
		LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	}
	out := make([]view, 0, len(st.APITokens))
	for _, token := range st.APITokens {
		out = append(out, view{token.ID, token.Name, token.Role, token.Scopes, token.CreatedAt, token.ExpiresAt, token.LastUsedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	writeJSON(w, 200, out)
}

func (s *Server) createAPIToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name         string     `json:"name"`
		Role         model.Role `json:"role"`
		Scopes       []string   `json:"scopes"`
		ExpiresHours int        `json:"expiresHours"`
	}
	if err := decodeJSON(r, &in, 32<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, 400, "name is required")
		return
	}
	if in.Role == "" {
		in.Role = model.RoleOperator
	}
	if in.Role != model.RoleAdmin && in.Role != model.RoleOperator && in.Role != model.RoleViewer {
		writeError(w, 400, "role must be admin, operator or viewer")
		return
	}
	if len(in.Scopes) == 0 {
		switch in.Role {
		case model.RoleViewer:
			in.Scopes = []string{"read"}
		case model.RoleOperator:
			in.Scopes = []string{"read", "sites:write", "rollouts:write"}
		case model.RoleAdmin:
			in.Scopes = []string{"admin"}
		}
	}
	scopes, ok := ops.NormalizeScopes(in.Scopes)
	if !ok || len(scopes) == 0 {
		writeError(w, 400, "invalid or empty scopes")
		return
	}
	if in.Role == model.RoleViewer {
		for _, scope := range scopes {
			if scope != "read" {
				writeError(w, 400, "viewer API tokens may only use read scope")
				return
			}
		}
	}
	if in.Role != model.RoleAdmin && ops.HasScope(scopes, "admin") {
		writeError(w, 400, "admin scope requires admin role")
		return
	}
	if in.ExpiresHours < 0 || in.ExpiresHours > 24*365 {
		writeError(w, 400, "expiresHours must be between 0 and 8760")
		return
	}
	plain, err := auth.RandomToken("zf_api_")
	if err != nil {
		writeError(w, 500, "could not create API token")
		return
	}
	now := time.Now().UTC()
	var expires *time.Time
	if in.ExpiresHours > 0 {
		v := now.Add(time.Duration(in.ExpiresHours) * time.Hour)
		expires = &v
	}
	token := model.APIToken{ID: newID("api"), Name: in.Name, TokenHash: auth.SHA256Token(plain), Role: in.Role, Scopes: scopes, CreatedAt: now, ExpiresAt: expires}
	actor := claimsFrom(r.Context())
	if err := s.store.Update(func(st *model.State) error {
		st.APITokens = append(st.APITokens, token)
		st.Events = appendEvent(st.Events, model.Event{ID: newID("evt"), Kind: "api_token.created", Severity: "info", Message: "API token " + token.Name + " created", Details: map[string]string{"actor": actor.Email, "role": string(token.Role)}, CreatedAt: now})
		return nil
	}); err != nil {
		writeError(w, 500, "could not save API token")
		return
	}
	writeJSON(w, 201, map[string]any{"id": token.ID, "name": token.Name, "role": token.Role, "scopes": token.Scopes, "expiresAt": token.ExpiresAt, "token": plain})
}

func (s *Server) deleteAPIToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	found := false
	name := ""
	actor := claimsFrom(r.Context())
	err := s.store.Update(func(st *model.State) error {
		out := st.APITokens[:0]
		for _, token := range st.APITokens {
			if token.ID == id {
				found = true
				name = token.Name
				continue
			}
			out = append(out, token)
		}
		st.APITokens = out
		if found {
			st.Events = appendEvent(st.Events, model.Event{ID: newID("evt"), Kind: "api_token.revoked", Severity: "info", Message: "API token " + name + " revoked", Details: map[string]string{"actor": actor.Email}, CreatedAt: time.Now().UTC()})
		}
		return nil
	})
	if err != nil {
		writeError(w, 500, "could not revoke API token")
		return
	}
	if !found {
		writeError(w, 404, "API token not found")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) authenticateAPIToken(plain string) (auth.Claims, *model.APIToken, error) {
	var claims auth.Claims
	if !strings.HasPrefix(plain, "zf_api_") {
		return claims, nil, nil
	}
	hash := auth.SHA256Token(plain)
	now := time.Now().UTC()
	st := s.store.Snapshot()
	for _, token := range st.APITokens {
		if len(token.TokenHash) != len(hash) || subtle.ConstantTimeCompare([]byte(token.TokenHash), []byte(hash)) != 1 {
			continue
		}
		if token.ExpiresAt != nil && !token.ExpiresAt.After(now) {
			return claims, nil, errors.New("API token expired")
		}
		copyToken := token
		// Avoid turning every automated GET into a durable state write. A
		// one-minute last-used resolution is enough for operator visibility.
		if token.LastUsedAt == nil || now.Sub(*token.LastUsedAt) >= time.Minute {
			_ = s.store.Update(func(state *model.State) error {
				for i := range state.APITokens {
					if state.APITokens[i].ID == token.ID {
						v := now
						state.APITokens[i].LastUsedAt = &v
						break
					}
				}
				return nil
			})
		}
		claims = auth.Claims{Subject: "api:" + token.ID, Email: "api-token:" + token.Name, Name: token.Name, Role: token.Role, Issued: now.Unix(), Expires: now.Add(time.Hour).Unix()}
		return claims, &copyToken, nil
	}
	return claims, nil, errors.New("invalid API token")
}

func apiTokenAllows(token model.APIToken, r *http.Request) bool {
	return ops.ScopeAllows(token.Scopes, r.Method, r.URL.Path)
}

func (s *Server) listWebhooks(w http.ResponseWriter, _ *http.Request) {
	st := s.store.Snapshot()
	out := make([]map[string]any, 0, len(st.Webhooks))
	for _, webhook := range st.Webhooks {
		out = append(out, publicWebhook(webhook))
	}
	writeJSON(w, 200, out)
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string   `json:"name"`
		URL        string   `json:"url"`
		Secret     string   `json:"secret"`
		EventKinds []string `json:"eventKinds"`
		Enabled    *bool    `json:"enabled"`
	}
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.URL = strings.TrimSpace(in.URL)
	if in.Name == "" || in.URL == "" {
		writeError(w, 400, "name and url are required")
		return
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeError(w, 400, "webhook url must be an absolute http(s) URL")
		return
	}
	if len(in.EventKinds) > 64 {
		writeError(w, 400, "too many eventKinds")
		return
	}
	for i := range in.EventKinds {
		in.EventKinds[i] = strings.TrimSpace(in.EventKinds[i])
		if in.EventKinds[i] == "" || len(in.EventKinds[i]) > 128 {
			writeError(w, 400, "invalid eventKinds value")
			return
		}
	}
	if in.Secret == "" {
		in.Secret, err = auth.RandomToken("zf_whsec_")
		if err != nil {
			writeError(w, 500, "could not create webhook secret")
			return
		}
	}
	if len(in.Secret) < 16 {
		writeError(w, 400, "webhook secret must be at least 16 characters")
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	now := time.Now().UTC()
	st := s.store.Snapshot()
	lastEventID := ""
	if len(st.Events) > 0 {
		lastEventID = st.Events[len(st.Events)-1].ID
	}
	webhook := model.Webhook{ID: newID("wh"), Name: in.Name, URL: in.URL, SigningSecret: in.Secret, EventKinds: unique(in.EventKinds), Enabled: enabled, CreatedAt: now, LastEventID: lastEventID}
	actor := claimsFrom(r.Context())
	if err := s.store.Update(func(state *model.State) error {
		state.Webhooks = append(state.Webhooks, webhook)
		state.Events = appendEvent(state.Events, model.Event{ID: newID("evt"), Kind: "webhook.created", Severity: "info", Message: "Webhook " + webhook.Name + " created", Details: map[string]string{"actor": actor.Email}, CreatedAt: now})
		return nil
	}); err != nil {
		writeError(w, 500, "could not save webhook")
		return
	}
	out := publicWebhook(webhook)
	out["signingSecret"] = in.Secret
	writeJSON(w, 201, out)
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	found := false
	name := ""
	actor := claimsFrom(r.Context())
	err := s.store.Update(func(st *model.State) error {
		out := st.Webhooks[:0]
		for _, webhook := range st.Webhooks {
			if webhook.ID == id {
				found = true
				name = webhook.Name
				continue
			}
			out = append(out, webhook)
		}
		st.Webhooks = out
		if found {
			st.Events = appendEvent(st.Events, model.Event{ID: newID("evt"), Kind: "webhook.deleted", Severity: "info", Message: "Webhook " + name + " deleted", Details: map[string]string{"actor": actor.Email}, CreatedAt: time.Now().UTC()})
		}
		return nil
	})
	if err != nil {
		writeError(w, 500, "could not delete webhook")
		return
	}
	if !found {
		writeError(w, 404, "webhook not found")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) testWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	st := s.store.Snapshot()
	for _, webhook := range st.Webhooks {
		if webhook.ID != id {
			continue
		}
		event := model.Event{ID: newID("evt"), Kind: "webhook.test", Severity: "info", Message: "Zyvor Fleet webhook test", CreatedAt: time.Now().UTC()}
		if err := deliverWebhook(webhook, event); err != nil {
			writeError(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	writeError(w, 404, "webhook not found")
}

func publicWebhook(webhook model.Webhook) map[string]any {
	return map[string]any{
		"id": webhook.ID, "name": webhook.Name, "url": webhook.URL,
		"eventKinds": webhook.EventKinds, "enabled": webhook.Enabled,
		"createdAt": webhook.CreatedAt, "lastDeliveryAt": webhook.LastDeliveryAt,
		"lastError": webhook.LastError, "consecutiveFailures": webhook.ConsecutiveFailures,
	}
}

func (s *Server) dispatchPendingWebhooks() {
	// Dispatchers serialize rather than dropping a trigger. Each pass reloads
	// state after taking the lock, so a mutation that lands during another
	// delivery is picked up by the queued pass instead of waiting for a later
	// unrelated request.
	webhookDispatchMu.Lock()
	defer webhookDispatchMu.Unlock()

	st := s.store.Snapshot()
	for _, webhook := range st.Webhooks {
		if !webhook.Enabled {
			continue
		}
		start := 0
		if webhook.LastEventID != "" {
			for i := range st.Events {
				if st.Events[i].ID == webhook.LastEventID {
					start = i + 1
					break
				}
			}
		}
		cursor := webhook.LastEventID
		lastDelivery := webhook.LastDeliveryAt
		lastError := ""
		failures := webhook.ConsecutiveFailures
		for i := start; i < len(st.Events); i++ {
			event := st.Events[i]
			if !ops.MatchEventKind(webhook.EventKinds, event.Kind) {
				cursor = event.ID
				continue
			}
			if err := deliverWebhook(webhook, event); err != nil {
				lastError = err.Error()
				failures++
				break
			}
			cursor = event.ID
			v := time.Now().UTC()
			lastDelivery = &v
			lastError = ""
			failures = 0
		}
		if cursor == webhook.LastEventID && lastError == webhook.LastError && failures == webhook.ConsecutiveFailures {
			continue
		}
		_ = s.store.Update(func(state *model.State) error {
			for i := range state.Webhooks {
				if state.Webhooks[i].ID == webhook.ID {
					state.Webhooks[i].LastEventID = cursor
					state.Webhooks[i].LastDeliveryAt = lastDelivery
					state.Webhooks[i].LastError = lastError
					state.Webhooks[i].ConsecutiveFailures = failures
					break
				}
			}
			return nil
		})
	}
}

func deliverWebhook(webhook model.Webhook, event model.Event) error {
	envelope := map[string]any{
		"specversion": "1.0",
		"id":          event.ID,
		"type":        event.Kind,
		"source":      "zyvor-fleet",
		"time":        event.CreatedAt,
		"data":        event,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, webhook.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/cloudevents+json")
	req.Header.Set("User-Agent", "zyvor-fleet/webhook")
	req.Header.Set("X-Zyvor-Fleet-Event", event.Kind)
	req.Header.Set("X-Zyvor-Fleet-Signature", ops.SignWebhook(webhook.SigningSecret, body))
	resp, err := webhookClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook delivery failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

type auditResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *auditResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (s *Server) auditMutations(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Resolve the actor before the mutation. An API token is allowed to
		// revoke itself, and the audit trail must still retain its identity.
		actor := s.auditActor(r)
		rw := &auditResponseWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r)
		if rw.status == 0 {
			rw.status = http.StatusOK
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions ||
			!strings.HasPrefix(r.URL.Path, "/api/v1/") || strings.HasPrefix(r.URL.Path, "/api/v1/agent/") ||
			r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/logout" {
			return
		}
		record := model.AuditRecord{ID: newID("aud"), Actor: actor, Method: r.Method, Path: r.URL.Path, Status: rw.status, RemoteIP: clientIP(r), CreatedAt: time.Now().UTC()}
		_ = s.store.Update(func(st *model.State) error {
			st.AuditLog = append(st.AuditLog, record)
			if len(st.AuditLog) > 5000 {
				st.AuditLog = append([]model.AuditRecord(nil), st.AuditLog[len(st.AuditLog)-5000:]...)
			}
			return nil
		})
	})
}

func (s *Server) auditActor(r *http.Request) string {
	if plain := bearer(r); strings.HasPrefix(plain, "zf_api_") {
		hash := auth.SHA256Token(plain)
		for _, token := range s.store.Snapshot().APITokens {
			if len(token.TokenHash) == len(hash) && subtle.ConstantTimeCompare([]byte(token.TokenHash), []byte(hash)) == 1 {
				return "api-token:" + token.Name
			}
		}
	}
	if cookie, err := r.Cookie("zyvor_fleet_session"); err == nil {
		if claims, err := s.sessions.Verify(cookie.Value); err == nil {
			return claims.Email
		}
	}
	return "unknown"
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	limit := 200
	st := s.store.Snapshot()
	if len(st.AuditLog) < limit {
		limit = len(st.AuditLog)
	}
	out := append([]model.AuditRecord(nil), st.AuditLog[len(st.AuditLog)-limit:]...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	writeJSON(w, 200, out)
}
