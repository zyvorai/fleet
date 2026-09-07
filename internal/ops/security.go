// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package ops

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

var validScopes = map[string]bool{
	"read":           true,
	"sites:write":    true,
	"rollouts:write": true,
	"admin":          true,
}

func ValidScope(scope string) bool { return validScopes[scope] }

// NormalizeScopes trims, validates and de-duplicates a list of API scopes.
func NormalizeScopes(in []string) ([]string, bool) {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, scope := range in {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if !ValidScope(scope) {
			return nil, false
		}
		if !seen[scope] {
			seen[scope] = true
			out = append(out, scope)
		}
	}
	return out, true
}

func HasScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want || scope == "admin" {
			return true
		}
	}
	return false
}

// ScopeAllows maps the existing REST API into a deliberately small set of
// automation scopes. It is intentionally conservative: new mutation paths
// default to admin until explicitly categorized here.
func ScopeAllows(scopes []string, method, path string) bool {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return HasScope(scopes, "read")
	}
	if strings.HasPrefix(path, "/api/v1/sites/") {
		return HasScope(scopes, "sites:write")
	}
	if strings.HasPrefix(path, "/api/v1/revisions") ||
		strings.HasPrefix(path, "/api/v1/rollouts") ||
		strings.HasPrefix(path, "/api/v1/site-groups") {
		return HasScope(scopes, "rollouts:write")
	}
	return HasScope(scopes, "admin")
}

// SignWebhook returns the value used in X-Zyvor-Fleet-Signature.
func SignWebhook(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func MatchEventKind(filters []string, kind string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter == "*" || filter == kind {
			return true
		}
		if strings.HasSuffix(filter, ".*") && strings.HasPrefix(kind, strings.TrimSuffix(filter, "*")) {
			return true
		}
	}
	return false
}
