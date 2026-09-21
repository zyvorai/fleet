// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zyvorai/fleet/internal/auth"
	"github.com/zyvorai/fleet/internal/model"
)

// OTA device contract routes match zyvor-ota docs/FLEET.md:
//   GET  /v1/devices/{device_id}/assignment
//   POST /v1/devices/{device_id}/events
// Admin management lives under /api/v1/ota/...

func (s *Server) registerOTARoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/devices/{device_id}/assignment", s.otaGetAssignment)
	mux.HandleFunc("POST /v1/devices/{device_id}/events", s.otaPostEvents)
	mux.Handle("GET /api/v1/ota/devices", s.requireRoles(http.HandlerFunc(s.listOTADevices), model.RoleAdmin, model.RoleOperator))
	mux.Handle("POST /api/v1/ota/devices", s.requireRoles(http.HandlerFunc(s.createOTADevice), model.RoleAdmin))
	mux.Handle("DELETE /api/v1/ota/devices/{device_id}", s.requireRoles(http.HandlerFunc(s.deleteOTADevice), model.RoleAdmin))
	mux.Handle("PUT /api/v1/ota/devices/{device_id}/assignment", s.requireRoles(http.HandlerFunc(s.putOTAAssignment), model.RoleAdmin, model.RoleOperator))
	mux.Handle("DELETE /api/v1/ota/devices/{device_id}/assignment", s.requireRoles(http.HandlerFunc(s.clearOTAAssignment), model.RoleAdmin, model.RoleOperator))
	mux.Handle("GET /api/v1/ota/devices/{device_id}/events", s.requireRoles(http.HandlerFunc(s.listOTADeviceEvents), model.RoleAdmin, model.RoleOperator, model.RoleViewer))
	s.registerOTARolloutRoutes(mux)
}

func (s *Server) authenticateOTADevice(r *http.Request, deviceID string) bool {
	token := bearer(r)
	if token == "" || deviceID == "" {
		return false
	}
	h := auth.SHA256Token(token)
	st := s.store.Snapshot()
	for _, d := range st.OTADevices {
		if d.DeviceID == deviceID && subtle.ConstantTimeCompare([]byte(d.TokenHash), []byte(h)) == 1 {
			return true
		}
	}
	return false
}

func (s *Server) otaGetAssignment(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if !s.authenticateOTADevice(r, deviceID) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	st := s.store.Snapshot()
	for _, d := range st.OTADevices {
		if d.DeviceID != deviceID {
			continue
		}
		if len(d.Assignment) == 0 || string(d.Assignment) == "null" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(d.Assignment)
		if len(d.Assignment) == 0 || d.Assignment[len(d.Assignment)-1] != '\n' {
			_, _ = w.Write([]byte("\n"))
		}
		return
	}
	writeError(w, http.StatusUnauthorized, "unauthorized")
}

func (s *Server) otaPostEvents(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if !s.authenticateOTADevice(r, deviceID) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 512<<10))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read error")
		return
	}
	var events []model.OTADeviceEvent
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&events); err != nil {
		writeError(w, http.StatusBadRequest, "invalid events")
		return
	}
	if len(events) == 0 || len(events) > 100 {
		writeError(w, http.StatusBadRequest, "event batch size")
		return
	}

	var acked uint64
	err = s.store.Update(func(st *model.State) error {
		idx := -1
		for i := range st.OTADevices {
			if st.OTADevices[i].DeviceID == deviceID {
				idx = i
				break
			}
		}
		if idx < 0 {
			return errors.New("device not found")
		}
		dev := &st.OTADevices[idx]
		bySeq := map[uint64]model.OTADeviceEvent{}
		for _, ev := range dev.Events {
			bySeq[ev.Sequence] = ev
		}
		for _, ev := range events {
			if ev.Sequence == 0 {
				return errors.New("event sequence required")
			}
			if prev, ok := bySeq[ev.Sequence]; ok {
				if prev.JobID != ev.JobID || prev.State != ev.State {
					return errors.New("conflicting event")
				}
				continue
			}
			bySeq[ev.Sequence] = ev
		}
		// Rebuild sorted-ish slice from map (bounded).
		dev.Events = make([]model.OTADeviceEvent, 0, len(bySeq))
		for _, ev := range bySeq {
			dev.Events = append(dev.Events, ev)
		}
		if len(dev.Events) > 10000 {
			return errors.New("event store full")
		}
		acked = dev.AckedSeq
		for {
			next := acked + 1
			if _, ok := bySeq[next]; !ok {
				break
			}
			acked = next
		}
		last := events[len(events)-1].Sequence
		if acked > last {
			acked = last
		}
		dev.AckedSeq = acked
		classifyOTADevice(dev)
		advanceOTARollouts(st, time.Now().UTC())
		return nil
	})
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "conflicting event" {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]uint64{"sequence": acked})
}

func (s *Server) listOTADevices(w http.ResponseWriter, _ *http.Request) {
	st := s.store.Snapshot()
	out := make([]map[string]any, 0, len(st.OTADevices))
	for _, d := range st.OTADevices {
		out = append(out, publicOTADevice(d))
	}
	writeJSON(w, 200, out)
}

func (s *Server) createOTADevice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DeviceID         string            `json:"deviceId"`
		Name             string            `json:"name"`
		SiteID           string            `json:"siteId"`
		Labels           map[string]string `json:"labels"`
		HardwareRevision string            `json:"hardwareRevision"`
		Slot             string            `json:"slot"`
		Version          string            `json:"version"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	if in.DeviceID == "" {
		writeError(w, 400, "deviceId required")
		return
	}
	plain, err := auth.RandomToken("zf_ota_")
	if err != nil {
		writeError(w, 500, "token generation failed")
		return
	}
	now := time.Now().UTC()
	dev := model.OTADevice{
		DeviceID:         in.DeviceID,
		Name:             strings.TrimSpace(in.Name),
		SiteID:           strings.TrimSpace(in.SiteID),
		TokenHash:        auth.SHA256Token(plain),
		CreatedAt:        now,
		Labels:           in.Labels,
		HardwareRevision: strings.TrimSpace(in.HardwareRevision),
		Slot:             strings.TrimSpace(in.Slot),
		Version:          strings.TrimSpace(in.Version),
		Online:           true,
		Report:           "offline",
	}
	err = s.store.Update(func(st *model.State) error {
		for _, d := range st.OTADevices {
			if d.DeviceID == in.DeviceID {
				return errors.New("device already registered")
			}
		}
		st.OTADevices = append(st.OTADevices, dev)
		st.Events = appendEvent(st.Events, model.Event{
			ID: newID("evt"), Kind: "ota.device.created", Severity: "info",
			Message: "OTA device registered: " + in.DeviceID, CreatedAt: now,
		})
		return nil
	})
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{
		"device": publicOTADevice(dev),
		"token":  plain,
	})
}

func (s *Server) deleteOTADevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	err := s.store.Update(func(st *model.State) error {
		for i, d := range st.OTADevices {
			if d.DeviceID == deviceID {
				st.OTADevices = append(st.OTADevices[:i], st.OTADevices[i+1:]...)
				return nil
			}
		}
		return errors.New("device not found")
	})
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) putOTAAssignment(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		writeError(w, 400, "read error")
		return
	}
	var probe map[string]any
	if err := json.Unmarshal(body, &probe); err != nil {
		writeError(w, 400, "assignment must be JSON object")
		return
	}
	if id, _ := probe["device_id"].(string); id != "" && id != deviceID {
		writeError(w, 400, "assignment device_id must match path")
		return
	}
	if _, ok := probe["job_id"]; !ok {
		writeError(w, 400, "assignment requires job_id")
		return
	}
	if _, ok := probe["release"]; !ok {
		writeError(w, 400, "assignment requires release envelope")
		return
	}
	// Normalize device_id in payload.
	probe["device_id"] = deviceID
	normalized, err := json.Marshal(probe)
	if err != nil {
		writeError(w, 500, "encode failed")
		return
	}
	err = s.store.Update(func(st *model.State) error {
		for i := range st.OTADevices {
			if st.OTADevices[i].DeviceID == deviceID {
				st.OTADevices[i].Assignment = normalized
				return nil
			}
		}
		return errors.New("device not found")
	})
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"deviceId": deviceID, "assignment": json.RawMessage(normalized)})
}

func (s *Server) clearOTAAssignment(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	err := s.store.Update(func(st *model.State) error {
		for i := range st.OTADevices {
			if st.OTADevices[i].DeviceID == deviceID {
				st.OTADevices[i].Assignment = nil
				return nil
			}
		}
		return errors.New("device not found")
	})
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listOTADeviceEvents(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	st := s.store.Snapshot()
	for _, d := range st.OTADevices {
		if d.DeviceID == deviceID {
			writeJSON(w, 200, map[string]any{
				"deviceId":      d.DeviceID,
				"ackedSequence": d.AckedSeq,
				"events":        d.Events,
				"hasAssignment": len(d.Assignment) > 0,
			})
			return
		}
	}
	writeError(w, 404, "device not found")
}

func publicOTADevice(d model.OTADevice) map[string]any {
	return map[string]any{
		"deviceId":         d.DeviceID,
		"name":             d.Name,
		"siteId":           d.SiteID,
		"createdAt":        d.CreatedAt,
		"ackedSequence":    d.AckedSeq,
		"hasAssignment":    len(d.Assignment) > 0,
		"eventCount":       len(d.Events),
		"labels":           d.Labels,
		"hardwareRevision": d.HardwareRevision,
		"slot":             d.Slot,
		"version":          d.Version,
		"report":           d.Report,
		"online":           d.Online,
	}
}
