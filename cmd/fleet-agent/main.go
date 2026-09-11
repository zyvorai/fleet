// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/zyvorai/fleet/internal/agent"
)

func main() {
	server := flag.String("server", envOr("ZYVOR_FLEET_SERVER", "http://127.0.0.1:8080"), "control plane URL")
	name := flag.String("name", envOr("ZYVOR_FLEET_SITE_NAME", hostname()), "site name")
	region := flag.String("region", os.Getenv("ZYVOR_FLEET_REGION"), "site region")
	enroll := flag.String("enrollment-token", os.Getenv("ZYVOR_FLEET_ENROLLMENT_TOKEN"), "one-time enrollment token")
	state := flag.String("state", envOr("ZYVOR_FLEET_AGENT_STATE", defaultState()), "local agent state file")
	labels := flag.String("labels", os.Getenv("ZYVOR_FLEET_LABELS"), "comma-separated key=value labels")
	interval := flag.Duration("interval", 15*time.Second, "sync interval")
	deviceAgentURL := flag.String("device-agent-url", os.Getenv("ZYVOR_FLEET_DEVICE_AGENT_URL"), "optional local Zyvor Device Agent URL (e.g. http://127.0.0.1:9188) to merge hardware metadata from")
	deviceAgentToken := flag.String("device-agent-token", os.Getenv("ZYVOR_FLEET_DEVICE_AGENT_TOKEN"), "bearer token for the Device Agent, if it has auth.mode = \"bearer\" configured")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(agent.Version)
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	sf, err := agent.OpenState(*state)
	if err != nil {
		logger.Error("open state", "error", err)
		os.Exit(1)
	}
	r := agent.NewRunner(agent.Config{Server: *server, Name: *name, Region: *region, EnrollmentToken: *enroll, Labels: parseLabels(*labels), Interval: *interval, Logger: logger, DeviceAgentURL: *deviceAgentURL, DeviceAgentToken: *deviceAgentToken}, sf)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := r.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}
func parseLabels(s string) map[string]string {
	m := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 && kv[0] != "" {
				m[kv[0]] = kv[1]
			}
		}
	}
	return m
}
func hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return "edge-site"
	}
	return h
}
func defaultState() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./data/agent.json"
	}
	return filepath.Join(home, ".local", "share", "zyvor-fleet", "agent.json")
}
func envOr(k, v string) string {
	if x := os.Getenv(k); x != "" {
		return x
	}
	return v
}
