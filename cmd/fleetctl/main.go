// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"
)

var version = "dev"

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

func main() {
	server := flag.String("server", envOr("ZYVOR_FLEET_SERVER", "http://127.0.0.1:8080"), "control plane URL")
	email := flag.String("email", envOr("ZYVOR_FLEET_ADMIN_EMAIL", "admin@zyvor.local"), "login email")
	password := flag.String("password", os.Getenv("ZYVOR_FLEET_ADMIN_PASSWORD"), "login password (prefer env)")
	apiToken := flag.String("api-token", os.Getenv("ZYVOR_FLEET_API_TOKEN"), "scoped API token (prefer env)")
	flag.Parse()
	if flag.NArg() < 1 {
		usage()
		os.Exit(2)
	}
	cmd := flag.Arg(0)
	if cmd == "version" {
		fmt.Println(version)
		return
	}
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Timeout: 15 * time.Second, Jar: jar}
	if *apiToken != "" {
		c.Jar = nil
		c.Transport = bearerTransport{token: *apiToken, base: http.DefaultTransport}
	} else {
		if *password == "" {
			fmt.Fprintln(os.Stderr, "ZYVOR_FLEET_ADMIN_PASSWORD/--password or ZYVOR_FLEET_API_TOKEN/--api-token is required")
			os.Exit(2)
		}
		if err := login(c, *server, *email, *password); err != nil {
			fail(err)
		}
	}
	switch cmd {
	case "status":
		printReq(c, *server, "GET", "/api/v1/dashboard", nil)
	case "sites":
		printReq(c, *server, "GET", "/api/v1/sites", nil)
	case "events":
		printReq(c, *server, "GET", "/api/v1/events?limit=50", nil)
	case "audit":
		printReq(c, *server, "GET", "/api/v1/audit?limit=100", nil)
	case "revisions":
		printReq(c, *server, "GET", "/api/v1/revisions", nil)
	case "rollouts":
		printReq(c, *server, "GET", "/api/v1/rollouts", nil)
	case "groups":
		printReq(c, *server, "GET", "/api/v1/site-groups", nil)
	case "webhooks":
		printReq(c, *server, "GET", "/api/v1/webhooks", nil)
	case "api-tokens":
		printReq(c, *server, "GET", "/api/v1/api-tokens", nil)
	case "group-create":
		if flag.NArg() < 3 {
			fail(fmt.Errorf("group-create requires NAME and key=value[,key=value] selector"))
		}
		printReq(c, *server, "POST", "/api/v1/site-groups", map[string]any{"name": flag.Arg(1), "selector": parsePairs(flag.Arg(2))})
	case "rollout-plan":
		if flag.NArg() < 2 {
			fail(fmt.Errorf("rollout-plan requires GROUP_ID"))
		}
		printReq(c, *server, "POST", "/api/v1/rollouts/plan", map[string]any{"groupId": flag.Arg(1), "waveSize": 10})
	case "rollout-pause", "rollout-resume", "rollout-abort", "rollout-approve", "rollout-rollback", "rollout-retry":
		if flag.NArg() < 2 {
			fail(fmt.Errorf("%s requires ROLLOUT_ID", cmd))
		}
		action := strings.TrimPrefix(cmd, "rollout-")
		printReq(c, *server, "POST", "/api/v1/rollouts/"+flag.Arg(1)+"/"+action, nil)
	case "site-maintenance":
		if flag.NArg() < 3 || (flag.Arg(2) != "on" && flag.Arg(2) != "off") {
			fail(fmt.Errorf("site-maintenance requires SITE_ID on|off [reason]"))
		}
		on := flag.Arg(2) == "on"
		reason := ""
		if flag.NArg() > 3 {
			reason = strings.Join(flag.Args()[3:], " ")
		}
		printReq(c, *server, "PATCH", "/api/v1/sites/"+flag.Arg(1), map[string]any{"maintenance": on, "maintenanceReason": reason})
	case "api-token-create":
		if flag.NArg() < 4 {
			fail(fmt.Errorf("api-token-create requires NAME ROLE scope[,scope]"))
		}
		scopes := []string{}
		for _, scope := range strings.Split(flag.Arg(3), ",") {
			if scope = strings.TrimSpace(scope); scope != "" {
				scopes = append(scopes, scope)
			}
		}
		printReq(c, *server, "POST", "/api/v1/api-tokens", map[string]any{"name": flag.Arg(1), "role": flag.Arg(2), "scopes": scopes})
	case "enroll-token":
		name := "CLI enrollment"
		if flag.NArg() > 1 {
			name = flag.Arg(1)
		}
		printReq(c, *server, "POST", "/api/v1/enrollment-tokens", map[string]any{"name": name, "expiresHours": 24, "maxUses": 1})
	default:
		usage()
		os.Exit(2)
	}
}
func login(c *http.Client, server, email, password string) error {
	b, _ := json.Marshal(map[string]string{"email": email, "password": password})
	r, err := http.NewRequest("POST", strings.TrimRight(server, "/")+"/api/v1/auth/login", bytes.NewReader(b))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		x, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed: %s", strings.TrimSpace(string(x)))
	}
	return nil
}
func printReq(c *http.Client, server, method, path string, body any) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, strings.TrimRight(server, "/")+path, r)
	if err != nil {
		fail(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set("X-Zyvor-Request", "1")
	}
	resp, err := c.Do(req)
	if err != nil {
		fail(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fail(fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(b))))
	}
	var v any
	if json.Unmarshal(b, &v) == nil {
		pretty, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(pretty))
	} else {
		fmt.Print(string(b))
	}
}
func usage() {
	fmt.Fprintln(os.Stderr, "fleetctl [flags] <version|status|sites|events|audit|revisions|rollouts|groups|webhooks|api-tokens|group-create NAME selector|rollout-plan GROUP_ID|rollout-pause ID|rollout-resume ID|rollout-abort ID|rollout-approve ID|rollout-rollback ID|rollout-retry ID|site-maintenance SITE_ID on|off [reason]|api-token-create NAME ROLE scope[,scope]|enroll-token [name]>")
}
func fail(err error) { fmt.Fprintln(os.Stderr, "fleetctl:", err); os.Exit(1) }
func envOr(k, v string) string {
	if x := os.Getenv(k); x != "" {
		return x
	}
	return v
}

func parsePairs(s string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) == 2 && strings.TrimSpace(kv[0]) != "" {
			out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return out
}
