// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/zyvorai/fleet/internal/model"
	"github.com/zyvorai/fleet/internal/runtimeadapter"
)

type Config struct {
	Server, Name, Region, EnrollmentToken string
	Labels                                map[string]string
	Interval                              time.Duration
	Logger                                *slog.Logger

	// DeviceAgentURL, when set, points at a local Zyvor Device Agent
	// (e.g. "http://127.0.0.1:9188") whose hardware metadata is merged
	// into every Register/Heartbeat call. Empty disables this entirely.
	DeviceAgentURL string
	// DeviceAgentToken authenticates to the Device Agent when it has
	// auth.mode = "bearer" configured. Empty is fine against auth.mode = "none".
	DeviceAgentToken string
}

type Runner struct {
	cfg     Config
	state   *StateFile
	client  *Client
	runtime *runtimeadapter.Manager
}

func NewRunner(cfg Config, state *StateFile) *Runner {
	if cfg.Interval <= 0 {
		cfg.Interval = 15 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	client := NewClient(cfg.Server)
	client.DeviceAgentURL = cfg.DeviceAgentURL
	client.DeviceAgentToken = cfg.DeviceAgentToken
	client.Logger = cfg.Logger
	return &Runner{cfg: cfg, state: state, client: client, runtime: runtimeadapter.New()}
}

func (a *Runner) Run(ctx context.Context) error {
	if err := a.ensureEnrollment(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()
	a.cycle(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			a.cycle(ctx)
		}
	}
}

func (a *Runner) ensureEnrollment(ctx context.Context) error {
	st := a.state.Snapshot()
	if st.SiteID != "" && st.AgentToken != "" {
		return nil
	}
	if a.cfg.EnrollmentToken == "" {
		return fmt.Errorf("enrollment token is required for first start")
	}
	reg, err := a.client.Register(ctx, a.cfg.EnrollmentToken, a.cfg.Name, a.cfg.Region, a.cfg.Labels)
	if err != nil {
		return fmt.Errorf("enroll site: %w", err)
	}
	if err := a.state.Update(func(s *LocalState) { s.SiteID = reg.SiteID; s.AgentToken = reg.AgentToken }); err != nil {
		return err
	}
	a.cfg.Logger.Info("site enrolled", "site_id", reg.SiteID, "name", a.cfg.Name)
	return nil
}

func (a *Runner) cycle(ctx context.Context) {
	st := a.state.Snapshot()
	sync, err := a.client.Sync(ctx, st.SiteID, st.AgentToken)
	if err != nil {
		if !st.ConnectivityLost {
			a.queueEvent("controlplane.unreachable", "warning", "Control plane unreachable; local autonomy is active")
			_ = a.state.Update(func(s *LocalState) { s.ConnectivityLost = true })
		}
		// Reconcile the complete cached desired state locally. This is the autonomy path:
		// the edge does not need the WAN to keep declared services/workloads converged.
		cached := a.state.Snapshot().CachedRevision
		if cached != nil {
			a.reconcileRevision(ctx, *cached, false)
		}
		a.cfg.Logger.Warn("sync failed; autonomy mode active", "error", err)
		return
	}

	reconnect := st.ConnectivityLost
	_ = a.state.Update(func(s *LocalState) {
		s.LastSync = time.Now().UTC()
		s.DesiredRevision = sync.DesiredRevision
		s.ConnectivityLost = false
		if sync.Revision != nil {
			cp := *sync.Revision
			s.CachedRevision = &cp
		}
	})
	if reconnect {
		a.queueEvent("controlplane.reconnected", "success", "Control plane connection restored; queued events will replay")
	}

	st = a.state.Snapshot()
	if st.CachedRevision != nil && sync.DesiredRevision != "" {
		// Never apply a cached revision under a different desired revision ID. A healthy
		// control plane always returns the desired revision body alongside its ID, but this
		// guard makes partial/corrupt responses fail closed instead of applying stale state.
		if st.CachedRevision.ID == sync.DesiredRevision {
			// Reconcile every cycle, not only on revision change. Typed adapters are idempotent,
			// making this a lightweight drift controller both online and offline.
			a.reconcileRevision(ctx, *st.CachedRevision, st.AppliedRevision != sync.DesiredRevision)
		} else {
			a.cfg.Logger.Error("desired revision body mismatch", "desired", sync.DesiredRevision, "cached", st.CachedRevision.ID)
		}
	}
	for _, cmd := range sync.Commands {
		status, errText := "completed", ""
		switch cmd.Type {
		case "inventory.refresh", "agent.ping":
		default:
			status = "failed"
			errText = "unsupported command type"
		}
		_ = a.client.Ack(ctx, st.SiteID, st.AgentToken, cmd.ID, status, errText, "")
	}

	st = a.state.Snapshot()
	events := append([]model.Event(nil), st.PendingEvents...)
	if len(events) > 100 {
		events = events[:100]
	}
	if err := a.client.Heartbeat(ctx, st.SiteID, st.AgentToken, st.AppliedRevision, st.FailedRevision, st.RevisionError, len(st.PendingEvents), false, st.WorkloadHealth, events); err != nil {
		a.cfg.Logger.Warn("heartbeat failed", "error", err)
		return
	}
	if len(events) > 0 {
		_ = a.state.Update(func(s *LocalState) {
			if len(s.PendingEvents) >= len(events) {
				s.PendingEvents = s.PendingEvents[len(events):]
			}
		})
	}
}

func (a *Runner) reconcileRevision(ctx context.Context, rev model.Revision, announce bool) bool {
	previous := a.state.Snapshot()
	health := make([]model.WorkloadHealth, 0, len(rev.Workloads))
	for _, spec := range rev.Workloads {
		opctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err := a.runtime.Reconcile(opctx, spec)
		cancel()
		if err != nil {
			msg := fmt.Sprintf("%s failed: %v", spec.Name, err)
			health = append(health, model.WorkloadHealth{Name: spec.Name, Kind: spec.Kind, Healthy: false, Message: err.Error(), CheckedAt: time.Now().UTC()})
			_ = a.state.Update(func(s *LocalState) {
				s.FailedRevision = rev.ID
				s.RevisionError = msg
				s.WorkloadHealth = health
			})
			if announce && (previous.FailedRevision != rev.ID || previous.RevisionError != msg) {
				a.queueEvent("revision.failed", "error", msg)
			}
			a.cfg.Logger.Error("workload reconciliation failed", "workload", spec.Name, "error", err)
			return false
		}

		probe := a.runtime.Check(ctx, spec)
		if spec.Health != nil && spec.Health.GraceSeconds > 0 && !probe.Healthy && announce {
			deadline := time.Now().Add(time.Duration(spec.Health.GraceSeconds) * time.Second)
			for !probe.Healthy && time.Now().Before(deadline) {
				select {
				case <-ctx.Done():
					return false
				case <-time.After(time.Second):
					probe = a.runtime.Check(ctx, spec)
				}
			}
		}
		health = append(health, probe)
		if !probe.Healthy {
			msg := fmt.Sprintf("%s health check failed: %s", spec.Name, probe.Message)
			_ = a.state.Update(func(s *LocalState) {
				s.FailedRevision = rev.ID
				s.RevisionError = msg
				s.WorkloadHealth = health
			})
			if announce && (previous.FailedRevision != rev.ID || previous.RevisionError != msg) {
				a.queueEvent("revision.health_failed", "error", msg)
			}
			a.cfg.Logger.Error("workload health check failed", "workload", spec.Name, "error", probe.Message)
			return false
		}
	}
	_ = a.state.Update(func(s *LocalState) {
		s.AppliedRevision = rev.ID
		s.FailedRevision = ""
		s.RevisionError = ""
		s.WorkloadHealth = health
	})
	if announce {
		a.queueEvent("revision.applied", "success", fmt.Sprintf("Revision %s applied", rev.Name))
	}
	return true
}

func (a *Runner) queueEvent(kind, severity, message string) {
	_ = a.state.Update(func(s *LocalState) {
		s.PendingEvents = append(s.PendingEvents, model.Event{Kind: kind, Severity: severity, Message: message, CreatedAt: time.Now().UTC()})
		if len(s.PendingEvents) > 2000 {
			s.PendingEvents = s.PendingEvents[len(s.PendingEvents)-2000:]
		}
	})
}
