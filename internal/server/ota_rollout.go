// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zyvorai/fleet/internal/model"
)

func (s *Server) registerOTARolloutRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/ota/cohorts", s.requireRoles(http.HandlerFunc(s.createOTACohort), model.RoleAdmin, model.RoleOperator))
	mux.Handle("GET /api/v1/ota/cohorts", s.requireRoles(http.HandlerFunc(s.listOTACohorts), model.RoleAdmin, model.RoleOperator, model.RoleViewer))
	mux.Handle("POST /api/v1/ota/rollouts", s.requireRoles(http.HandlerFunc(s.createOTARollout), model.RoleAdmin, model.RoleOperator))
	mux.Handle("GET /api/v1/ota/rollouts", s.requireRoles(http.HandlerFunc(s.listOTARollouts), model.RoleAdmin, model.RoleOperator, model.RoleViewer))
	mux.Handle("GET /api/v1/ota/rollouts/{id}", s.requireRoles(http.HandlerFunc(s.getOTARollout), model.RoleAdmin, model.RoleOperator, model.RoleViewer))
	mux.Handle("POST /api/v1/ota/rollouts/{id}/pause", s.requireRoles(http.HandlerFunc(s.pauseOTARollout), model.RoleAdmin, model.RoleOperator))
	mux.Handle("GET /api/v1/ota/devices/{device_id}/timeline", s.requireRoles(http.HandlerFunc(s.otaTimeline), model.RoleAdmin, model.RoleOperator, model.RoleViewer))
	mux.Handle("GET /api/v1/ota/devices/{device_id}/recovery", s.requireRoles(http.HandlerFunc(s.otaRecovery), model.RoleAdmin, model.RoleOperator, model.RoleViewer))
}

func classifyOTADevice(dev *model.OTADevice) {
	dev.Online = true
	var latest model.OTADeviceEvent
	for _, ev := range dev.Events {
		if ev.Sequence >= latest.Sequence {
			latest = ev
		}
	}
	if latest.Sequence == 0 {
		dev.Report = "offline"
		dev.Online = false
		return
	}
	dev.LastEventAt = latest.Time
	switch latest.State {
	case "failed":
		dev.Report = "failed"
	case "rolled_back":
		dev.Report = "rolled_back"
	case "needs_recovery":
		dev.Report = "needs_recovery"
	default:
		dev.Report = "ok"
	}
	if strings.HasPrefix(latest.Error, "deferred:") {
		dev.Report = "deferred"
	}
}

func advanceOTARollouts(st *model.State, now time.Time) {
	byID := map[string]*model.OTADevice{}
	for i := range st.OTADevices {
		byID[st.OTADevices[i].DeviceID] = &st.OTADevices[i]
	}
	for i := range st.OTARollouts {
		r := &st.OTARollouts[i]
		if r.Status == "paused" || r.Status == "completed" || r.Status == "aborted" {
			continue
		}
		if !inOTAWindow(now, r.WindowStart, r.WindowEnd, r.Timezone) {
			r.Status = "deferred"
			continue
		}
		activatedSet := map[string]bool{}
		for _, id := range r.Activated {
			activatedSet[id] = true
		}
		r.Completed = nil
		r.Failed = nil
		r.RolledBack = nil
		r.Offline = nil
		r.Deferred = nil
		for _, id := range r.DeviceIDs {
			dev := byID[id]
			if dev == nil {
				continue
			}
			if !activatedSet[id] {
				continue
			}
			if !dev.Online || dev.Report == "offline" {
				r.Offline = append(r.Offline, id)
				continue
			}
			switch dev.Report {
			case "failed", "needs_recovery":
				r.Failed = append(r.Failed, id)
			case "rolled_back":
				r.RolledBack = append(r.RolledBack, id)
			case "deferred":
				r.Deferred = append(r.Deferred, id)
			case "ok":
				if latestOTAState(dev) == "committed" {
					r.Completed = append(r.Completed, id)
				}
			}
		}
		if r.MaxFailures > 0 && len(r.Failed) >= r.MaxFailures {
			r.Status = "paused"
			r.PausedReason = "failure threshold"
			continue
		}
		if r.MaxRollbacks > 0 && len(r.RolledBack) >= r.MaxRollbacks {
			r.Status = "paused"
			r.PausedReason = "rollback threshold"
			continue
		}
		if r.MaxOffline > 0 && len(r.Offline) >= r.MaxOffline {
			r.Status = "paused"
			r.PausedReason = "offline threshold"
			continue
		}
		limit := r.CanaryCount
		if r.CurrentWave > 0 {
			limit = r.CanaryCount + r.CurrentWave*r.WaveSize
		}
		if limit < 1 {
			limit = 1
		}
		if limit > len(r.DeviceIDs) {
			limit = len(r.DeviceIDs)
		}
		ready := len(r.Activated) == 0
		if !ready {
			ready = true
			for _, id := range r.Activated {
				dev := byID[id]
				if dev == nil {
					ready = false
					break
				}
				switch latestOTAState(dev) {
				case "committed", "failed", "rolled_back", "needs_recovery":
				default:
					ready = false
				}
			}
		}
		if ready && limit < len(r.DeviceIDs) && len(r.Activated) > 0 {
			r.CurrentWave++
			limit = r.CanaryCount + r.CurrentWave*r.WaveSize
			if limit > len(r.DeviceIDs) {
				limit = len(r.DeviceIDs)
			}
		}
		r.Activated = append([]string(nil), r.DeviceIDs[:limit]...)
		for _, id := range r.Activated {
			dev := byID[id]
			if dev == nil {
				continue
			}
			dev.Assignment = rewriteAssignment(r.Assignment, id, r.ID)
		}
		if len(r.Completed) == len(r.DeviceIDs) {
			r.Status = "completed"
		} else if r.Status == "deferred" {
			r.Status = "running"
		} else if r.Status == "" || r.Status == "pending" {
			r.Status = "running"
		}
		r.UpdatedAt = now
	}
}

func latestOTAState(dev *model.OTADevice) string {
	var latest model.OTADeviceEvent
	for _, ev := range dev.Events {
		if ev.Sequence >= latest.Sequence {
			latest = ev
		}
	}
	return latest.State
}

func rewriteAssignment(raw json.RawMessage, deviceID, rolloutID string) json.RawMessage {
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return raw
	}
	probe["device_id"] = deviceID
	if _, ok := probe["job_id"]; ok {
		probe["job_id"] = rolloutID + "-" + deviceID
	}
	b, err := json.Marshal(probe)
	if err != nil {
		return raw
	}
	return b
}

func inOTAWindow(now time.Time, start, end, tz string) bool {
	if start == "" || end == "" {
		return true
	}
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	local := now.In(loc)
	sh, sm, ok1 := clockParts(start)
	eh, em, ok2 := clockParts(end)
	if !ok1 || !ok2 {
		return true
	}
	cur := local.Hour()*60 + local.Minute()
	a, b := sh*60+sm, eh*60+em
	if a <= b {
		return cur >= a && cur < b
	}
	return cur >= a || cur < b
}

func clockParts(v string) (int, int, bool) {
	var h, m int
	if _, err := fmt.Sscanf(v, "%d:%d", &h, &m); err != nil || len(v) != 5 || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

func (s *Server) createOTACohort(w http.ResponseWriter, r *http.Request) {
	var in model.OTACohort
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeError(w, 400, "name required")
		return
	}
	now := time.Now().UTC()
	in.ID = newID("otac")
	in.CreatedAt = now
	err := s.store.Update(func(st *model.State) error {
		st.OTACohorts = append(st.OTACohorts, in)
		return nil
	})
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, in)
}

func (s *Server) listOTACohorts(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, s.store.Snapshot().OTACohorts)
}

func (s *Server) createOTARollout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name         string          `json:"name"`
		CohortID     string          `json:"cohortId"`
		DeviceIDs    []string        `json:"deviceIds"`
		Assignment   json.RawMessage `json:"assignment"`
		CanaryCount  int             `json:"canaryCount"`
		WaveSize     int             `json:"waveSize"`
		MaxFailures  int             `json:"maxFailures"`
		MaxRollbacks int             `json:"maxRollbacks"`
		MaxOffline   int             `json:"maxOffline"`
		WindowStart  string          `json:"windowStart"`
		WindowEnd    string          `json:"windowEnd"`
		Timezone     string          `json:"timezone"`
	}
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if len(in.Assignment) == 0 {
		writeError(w, 400, "assignment required")
		return
	}
	if in.CanaryCount < 1 {
		in.CanaryCount = 1
	}
	if in.WaveSize < 1 {
		in.WaveSize = 10
	}
	now := time.Now().UTC()
	roll := model.OTARollout{
		ID: newID("otar"), Name: in.Name, CohortID: in.CohortID, Assignment: in.Assignment,
		Status: "pending", CanaryCount: in.CanaryCount, WaveSize: in.WaveSize,
		MaxFailures: in.MaxFailures, MaxRollbacks: in.MaxRollbacks, MaxOffline: in.MaxOffline,
		WindowStart: in.WindowStart, WindowEnd: in.WindowEnd, Timezone: in.Timezone,
		CreatedAt: now, UpdatedAt: now,
	}
	err := s.store.Update(func(st *model.State) error {
		ids := append([]string(nil), in.DeviceIDs...)
		if in.CohortID != "" {
			var cohort *model.OTACohort
			for i := range st.OTACohorts {
				if st.OTACohorts[i].ID == in.CohortID {
					cohort = &st.OTACohorts[i]
					break
				}
			}
			if cohort == nil {
				return errors.New("cohort not found")
			}
			ids = devicesForCohort(st.OTADevices, *cohort)
		}
		if len(ids) == 0 {
			return errors.New("no devices targeted")
		}
		roll.DeviceIDs = ids
		st.OTARollouts = append(st.OTARollouts, roll)
		advanceOTARollouts(st, now)
		return nil
	})
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{
		"rollout": roll,
		"hero":    "Can I safely update 10,000 remote gateways tonight?",
	})
}

func devicesForCohort(devices []model.OTADevice, c model.OTACohort) []string {
	pinned := map[string]bool{}
	for _, id := range c.DeviceIDs {
		pinned[id] = true
	}
	var out []string
	for _, d := range devices {
		if pinned[d.DeviceID] {
			out = append(out, d.DeviceID)
			continue
		}
		if len(c.Selector) == 0 {
			continue
		}
		ok := true
		for k, v := range c.Selector {
			if d.Labels[k] != v {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, d.DeviceID)
		}
	}
	return out
}

func (s *Server) listOTARollouts(w http.ResponseWriter, _ *http.Request) {
	st := s.store.Snapshot()
	writeJSON(w, 200, map[string]any{
		"hero":     "Can I safely update 10,000 remote gateways tonight?",
		"rollouts": st.OTARollouts,
	})
}

func (s *Server) getOTARollout(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for _, roll := range s.store.Snapshot().OTARollouts {
		if roll.ID == id {
			total := len(roll.DeviceIDs)
			done := len(roll.Completed)
			eta := ""
			if roll.Status == "running" && done < total {
				eta = "remaining devices wait for the canary to commit before the next wave"
			}
			writeJSON(w, 200, map[string]any{"rollout": roll, "progress": map[string]any{"done": done, "total": total, "eta": eta}})
			return
		}
	}
	writeError(w, 404, "rollout not found")
}

func (s *Server) pauseOTARollout(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.store.Update(func(st *model.State) error {
		for i := range st.OTARollouts {
			if st.OTARollouts[i].ID == id {
				st.OTARollouts[i].Status = "paused"
				st.OTARollouts[i].PausedReason = "operator"
				return nil
			}
		}
		return errors.New("rollout not found")
	})
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) otaTimeline(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("device_id")
	for _, d := range s.store.Snapshot().OTADevices {
		if d.DeviceID == id {
			writeJSON(w, 200, map[string]any{
				"deviceId": id,
				"slot":     d.Slot,
				"version":  d.Version,
				"report":   d.Report,
				"events":   d.Events,
			})
			return
		}
	}
	writeError(w, 404, "device not found")
}

func (s *Server) otaRecovery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("device_id")
	for _, d := range s.store.Snapshot().OTADevices {
		if d.DeviceID != id {
			continue
		}
		if d.Report != "needs_recovery" {
			writeJSON(w, 200, map[string]any{"needed": false, "guidance": "No ambiguous install is waiting."})
			return
		}
		writeJSON(w, 200, map[string]any{
			"needed":   true,
			"guidance": "NeedsRecovery means the agent will not guess. Inspect RAUC, run recover-abort only while the original slot is booted and the backend is idle, then retry with a newly signed release sequence. There is no API that skips signature or sequence checks.",
		})
		return
	}
	writeError(w, 404, "device not found")
}
