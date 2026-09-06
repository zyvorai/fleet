package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zyvorai/fleet/internal/auth"
	"github.com/zyvorai/fleet/internal/model"
	"github.com/zyvorai/fleet/internal/store"
	"github.com/zyvorai/fleet/webui"
)

type Config struct {
	SecureCookies bool
	Logger        *slog.Logger
	SiteTimeout   time.Duration
}

type Server struct {
	store    *store.Store
	sessions *auth.SessionManager
	cfg      Config
	limiter  *loginLimiter
}

type contextKey string

const claimsKey contextKey = "claims"

func New(st *store.Store, sessions *auth.SessionManager, cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.SiteTimeout <= 0 {
		cfg.SiteTimeout = 45 * time.Second
	}
	return &Server{store: st, sessions: sessions, cfg: cfg, limiter: newLoginLimiter()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("GET /metrics", s.metrics)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.Handle("POST /api/v1/auth/logout", s.requireAuth(http.HandlerFunc(s.logout)))
	mux.Handle("GET /api/v1/auth/me", s.requireAuth(http.HandlerFunc(s.me)))
	mux.Handle("GET /api/v1/dashboard", s.requireAuth(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /api/v1/users", s.requireRoles(http.HandlerFunc(s.listUsers), model.RoleAdmin))
	mux.Handle("POST /api/v1/users", s.requireRoles(http.HandlerFunc(s.createUser), model.RoleAdmin))
	mux.Handle("PATCH /api/v1/users/{id}", s.requireRoles(http.HandlerFunc(s.updateUser), model.RoleAdmin))
	mux.Handle("DELETE /api/v1/users/{id}", s.requireRoles(http.HandlerFunc(s.deleteUser), model.RoleAdmin))
	mux.Handle("GET /api/v1/sites", s.requireAuth(http.HandlerFunc(s.listSites)))
	mux.Handle("GET /api/v1/sites/{id}", s.requireAuth(http.HandlerFunc(s.getSite)))
	mux.Handle("PATCH /api/v1/sites/{id}", s.requireRoles(http.HandlerFunc(s.updateSite), model.RoleAdmin, model.RoleOperator))
	mux.Handle("DELETE /api/v1/sites/{id}", s.requireRoles(http.HandlerFunc(s.deleteSite), model.RoleAdmin))
	mux.Handle("GET /api/v1/site-groups", s.requireAuth(http.HandlerFunc(s.listSiteGroups)))
	mux.Handle("POST /api/v1/site-groups", s.requireRoles(http.HandlerFunc(s.createSiteGroup), model.RoleAdmin, model.RoleOperator))
	mux.Handle("DELETE /api/v1/site-groups/{id}", s.requireRoles(http.HandlerFunc(s.deleteSiteGroup), model.RoleAdmin, model.RoleOperator))
	mux.Handle("GET /api/v1/enrollment-tokens", s.requireRoles(http.HandlerFunc(s.listEnrollmentTokens), model.RoleAdmin))
	mux.Handle("POST /api/v1/enrollment-tokens", s.requireRoles(http.HandlerFunc(s.createEnrollmentToken), model.RoleAdmin))
	mux.Handle("DELETE /api/v1/enrollment-tokens/{id}", s.requireRoles(http.HandlerFunc(s.deleteEnrollmentToken), model.RoleAdmin))
	mux.Handle("GET /api/v1/revisions", s.requireAuth(http.HandlerFunc(s.listRevisions)))
	mux.Handle("POST /api/v1/revisions", s.requireRoles(http.HandlerFunc(s.createRevision), model.RoleAdmin, model.RoleOperator))
	mux.Handle("GET /api/v1/rollouts", s.requireAuth(http.HandlerFunc(s.listRollouts)))
	mux.Handle("POST /api/v1/rollouts/plan", s.requireRoles(http.HandlerFunc(s.planRollout), model.RoleAdmin, model.RoleOperator))
	mux.Handle("POST /api/v1/rollouts", s.requireRoles(http.HandlerFunc(s.createRollout), model.RoleAdmin, model.RoleOperator))
	mux.Handle("POST /api/v1/rollouts/{id}/pause", s.requireRoles(http.HandlerFunc(s.pauseRollout), model.RoleAdmin, model.RoleOperator))
	mux.Handle("POST /api/v1/rollouts/{id}/resume", s.requireRoles(http.HandlerFunc(s.resumeRollout), model.RoleAdmin, model.RoleOperator))
	mux.Handle("POST /api/v1/rollouts/{id}/abort", s.requireRoles(http.HandlerFunc(s.abortRollout), model.RoleAdmin, model.RoleOperator))
	mux.Handle("POST /api/v1/rollouts/{id}/approve", s.requireRoles(http.HandlerFunc(s.approveRollout), model.RoleAdmin))
	mux.Handle("POST /api/v1/rollouts/{id}/rollback", s.requireRoles(http.HandlerFunc(s.rollbackRollout), model.RoleAdmin, model.RoleOperator))
	mux.Handle("POST /api/v1/rollouts/{id}/retry", s.requireRoles(http.HandlerFunc(s.retryRollout), model.RoleAdmin, model.RoleOperator))
	mux.Handle("GET /api/v1/events", s.requireAuth(http.HandlerFunc(s.listEvents)))
	mux.HandleFunc("POST /api/v1/agent/register", s.agentRegister)
	mux.HandleFunc("POST /api/v1/agent/heartbeat", s.agentHeartbeat)
	mux.HandleFunc("GET /api/v1/agent/sync", s.agentSync)
	mux.HandleFunc("POST /api/v1/agent/ack", s.agentAck)
	mux.Handle("GET /api/v1/integrations", s.requireAuth(http.HandlerFunc(s.integrations)))
	mux.Handle("PATCH /api/v1/integrations/{id}", s.requireRoles(http.HandlerFunc(s.updateIntegration), model.RoleAdmin))
	mux.Handle("/", webui.Handler())
	return s.securityHeaders(s.accessLog(mux))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "schema": s.store.Snapshot().SchemaVersion})
}

func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	s.markOfflineAndReconcile()
	st := s.store.Snapshot()
	statuses := map[string]int{"online": 0, "degraded": 0, "offline": 0}
	for _, site := range st.Sites {
		if _, ok := statuses[site.Status]; ok {
			statuses[site.Status]++
		} else {
			statuses["offline"]++
		}
	}
	running, failed, paused := 0, 0, 0
	for _, ro := range st.Rollouts {
		switch ro.Status {
		case "running", "scheduled", "pending_approval":
			running++
		case "failed":
			failed++
		case "paused":
			paused++
		}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP zyvor_fleet_sites Fleet sites by status.\n# TYPE zyvor_fleet_sites gauge\n")
	for _, status := range []string{"online", "degraded", "offline"} {
		fmt.Fprintf(w, "zyvor_fleet_sites{status=%q} %d\n", status, statuses[status])
	}
	fmt.Fprintf(w, "# HELP zyvor_fleet_rollouts Rollouts by operational state.\n# TYPE zyvor_fleet_rollouts gauge\n")
	fmt.Fprintf(w, "zyvor_fleet_rollouts{state=\"active\"} %d\n", running)
	fmt.Fprintf(w, "zyvor_fleet_rollouts{state=\"paused\"} %d\n", paused)
	fmt.Fprintf(w, "zyvor_fleet_rollouts{state=\"failed\"} %d\n", failed)
	fmt.Fprintf(w, "zyvor_fleet_events_total %d\n", len(st.Events))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.limiter.Allow(ip) {
		writeError(w, http.StatusTooManyRequests, "too many sign-in attempts")
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	state := s.store.Snapshot()
	var found *model.User
	for i := range state.Users {
		if strings.EqualFold(state.Users[i].Email, in.Email) {
			found = &state.Users[i]
			break
		}
	}
	if found == nil || !auth.VerifyPassword(found.PasswordHash, in.Password) {
		s.limiter.Fail(ip)
		time.Sleep(80 * time.Millisecond)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	s.limiter.Success(ip)
	token, err := s.sessions.Sign(*found)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "zyvor_fleet_session", Value: token, Path: "/", HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: int((12 * time.Hour).Seconds())})
	writeJSON(w, http.StatusOK, publicUser(*found))
}

func (s *Server) logout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "zyvor_fleet_session", Value: "", Path: "/", HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"id": c.Subject, "email": c.Email, "name": c.Name, "role": c.Role})
}

func (s *Server) listUsers(w http.ResponseWriter, _ *http.Request) {
	st := s.store.Snapshot()
	out := make([]map[string]any, 0, len(st.Users))
	for _, u := range st.Users {
		out = append(out, publicUser(u))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email, Name, Password string
		Role                  model.Role
	}
	if err := decodeJSON(r, &in, 32<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	if in.Email == "" || !strings.Contains(in.Email, "@") {
		writeError(w, 400, "valid email is required")
		return
	}
	if in.Name == "" {
		in.Name = in.Email
	}
	if in.Role != model.RoleAdmin && in.Role != model.RoleOperator && in.Role != model.RoleViewer {
		writeError(w, 400, "role must be admin, operator or viewer")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	u := model.User{ID: newID("usr"), Email: in.Email, Name: in.Name, Role: in.Role, PasswordHash: hash, CreatedAt: time.Now().UTC()}
	err = s.store.Update(func(st *model.State) error {
		for _, existing := range st.Users {
			if strings.EqualFold(existing.Email, u.Email) {
				return errors.New("email already exists")
			}
		}
		st.Users = append(st.Users, u)
		return nil
	})
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, publicUser(u))
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Name     string
		Role     model.Role
		Password string
	}
	if err := decodeJSON(r, &in, 32<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if in.Role != "" && in.Role != model.RoleAdmin && in.Role != model.RoleOperator && in.Role != model.RoleViewer {
		writeError(w, 400, "invalid role")
		return
	}
	if c := claimsFrom(r.Context()); id == c.Subject && in.Role != "" && in.Role != model.RoleAdmin {
		writeError(w, 400, "cannot remove your own admin role")
		return
	}
	var newHash string
	var err error
	if in.Password != "" {
		newHash, err = auth.HashPassword(in.Password)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
	}
	found := false
	var updated model.User
	err = s.store.Update(func(st *model.State) error {
		for i := range st.Users {
			if st.Users[i].ID == id {
				found = true
				if in.Name != "" {
					st.Users[i].Name = strings.TrimSpace(in.Name)
				}
				if in.Role != "" {
					st.Users[i].Role = in.Role
				}
				if newHash != "" {
					st.Users[i].PasswordHash = newHash
					st.Users[i].AuthVersion++
				}
				updated = st.Users[i]
				break
			}
		}
		if !found {
			return errors.New("user not found")
		}
		admins := 0
		for _, u := range st.Users {
			if u.Role == model.RoleAdmin {
				admins++
			}
		}
		if admins == 0 {
			return errors.New("at least one admin is required")
		}
		return nil
	})
	if err != nil {
		status := 400
		if !found {
			status = 404
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, 200, publicUser(updated))
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c := claimsFrom(r.Context())
	if id == c.Subject {
		writeError(w, 400, "cannot delete your current account")
		return
	}
	found := false
	err := s.store.Update(func(st *model.State) error {
		out := st.Users[:0]
		for _, u := range st.Users {
			if u.ID == id {
				found = true
				continue
			}
			out = append(out, u)
		}
		if !found {
			return errors.New("user not found")
		}
		admins := 0
		for _, u := range out {
			if u.Role == model.RoleAdmin {
				admins++
			}
		}
		if admins == 0 {
			return errors.New("at least one admin is required")
		}
		st.Users = out
		return nil
	})
	if err != nil {
		status := 400
		if !found {
			status = 404
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	s.markOfflineAndReconcile()
	st := s.store.Snapshot()
	online, degraded, offline, autonomy := 0, 0, 0, 0
	regions := map[string]int{}
	for _, site := range st.Sites {
		switch site.Status {
		case "online":
			online++
		case "degraded":
			degraded++
		default:
			offline++
		}
		if site.AutonomyMode {
			autonomy++
		}
		if site.Region != "" {
			regions[site.Region]++
		}
	}
	running := 0
	for _, ro := range st.Rollouts {
		if ro.Status == "running" {
			running++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sites":           map[string]int{"total": len(st.Sites), "online": online, "degraded": degraded, "offline": offline, "autonomy": autonomy},
		"regions":         regions,
		"runningRollouts": running,
		"recentEvents":    tailEvents(st.Events, 8),
		"generatedAt":     time.Now().UTC(),
	})
}

func (s *Server) listSites(w http.ResponseWriter, _ *http.Request) {
	s.markOfflineAndReconcile()
	st := s.store.Snapshot()
	views := make([]model.SiteView, 0, len(st.Sites))
	for _, site := range st.Sites {
		views = append(views, site.View())
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) getSite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	st := s.store.Snapshot()
	for _, site := range st.Sites {
		if site.ID == id {
			writeJSON(w, http.StatusOK, site.View())
			return
		}
	}
	writeError(w, http.StatusNotFound, "site not found")
}

func (s *Server) updateSite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Name   *string           `json:"name"`
		Region *string           `json:"region"`
		Labels map[string]string `json:"labels"`
	}
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	labels, err := normalizeLabels(in.Labels)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	var updated model.SiteView
	found := false
	err = s.store.Update(func(st *model.State) error {
		for i := range st.Sites {
			if st.Sites[i].ID != id {
				continue
			}
			found = true
			if in.Name != nil {
				name := strings.TrimSpace(*in.Name)
				if name == "" {
					return errors.New("site name cannot be empty")
				}
				for _, other := range st.Sites {
					if other.ID != id && strings.EqualFold(other.Name, name) {
						return errors.New("site name already exists")
					}
				}
				st.Sites[i].Name = name
			}
			if in.Region != nil {
				st.Sites[i].Region = strings.TrimSpace(*in.Region)
			}
			if in.Labels != nil {
				st.Sites[i].Labels = labels
			}
			updated = st.Sites[i].View()
			st.Events = appendEvent(st.Events, model.Event{ID: newID("evt"), SiteID: id, Kind: "site.updated", Severity: "info", Message: st.Sites[i].Name + " metadata updated", CreatedAt: time.Now().UTC()})
			break
		}
		if !found {
			return errors.New("site not found")
		}
		return nil
	})
	if err != nil {
		status := 400
		if !found {
			status = 404
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, 200, updated)
}

func (s *Server) listSiteGroups(w http.ResponseWriter, _ *http.Request) {
	st := s.store.Snapshot()
	type groupView struct {
		model.SiteGroup
		ResolvedSiteIDs []string `json:"resolvedSiteIds"`
	}
	out := make([]groupView, 0, len(st.SiteGroups))
	for _, g := range st.SiteGroups {
		out = append(out, groupView{SiteGroup: g, ResolvedSiteIDs: resolveGroupSites(st, g)})
	}
	writeJSON(w, 200, out)
}

func (s *Server) createSiteGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Selector    map[string]string `json:"selector"`
		SiteIDs     []string          `json:"siteIds"`
	}
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, 400, "name is required")
		return
	}
	selector, err := normalizeLabels(in.Selector)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	st := s.store.Snapshot()
	valid := map[string]bool{}
	for _, site := range st.Sites {
		valid[site.ID] = true
	}
	for _, id := range unique(in.SiteIDs) {
		if !valid[id] {
			writeError(w, 400, "unknown site: "+id)
			return
		}
	}
	if len(selector) == 0 && len(in.SiteIDs) == 0 {
		writeError(w, 400, "selector or siteIds is required")
		return
	}
	c := claimsFrom(r.Context())
	g := model.SiteGroup{ID: newID("grp"), Name: in.Name, Description: strings.TrimSpace(in.Description), Selector: selector, SiteIDs: unique(in.SiteIDs), CreatedAt: time.Now().UTC(), CreatedBy: c.Email}
	if err := s.store.Update(func(state *model.State) error {
		for _, existing := range state.SiteGroups {
			if strings.EqualFold(existing.Name, g.Name) {
				return errors.New("group name already exists")
			}
		}
		state.SiteGroups = append(state.SiteGroups, g)
		state.Events = appendEvent(state.Events, model.Event{ID: newID("evt"), Kind: "group.created", Severity: "info", Message: "Site group " + g.Name + " created", CreatedAt: time.Now().UTC()})
		return nil
	}); err != nil {
		writeError(w, 409, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"group": g, "resolvedSiteIds": resolveGroupSites(s.store.Snapshot(), g)})
}

func (s *Server) deleteSiteGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	found := false
	err := s.store.Update(func(st *model.State) error {
		out := st.SiteGroups[:0]
		for _, g := range st.SiteGroups {
			if g.ID == id {
				found = true
				continue
			}
			out = append(out, g)
		}
		st.SiteGroups = out
		return nil
	})
	if err != nil {
		writeError(w, 500, "could not delete group")
		return
	}
	if !found {
		writeError(w, 404, "group not found")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) deleteSite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	found := false
	conflict := false
	err := s.store.Update(func(st *model.State) error {
		for _, site := range st.Sites {
			if site.ID == id {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		for _, ro := range st.Rollouts {
			if !terminalRollout(ro.Status) && contains(ro.SiteIDs, id) {
				conflict = true
				return errors.New("site belongs to an active rollout; abort or complete the rollout first")
			}
		}
		out := st.Sites[:0]
		for _, site := range st.Sites {
			if site.ID != id {
				out = append(out, site)
			}
		}
		st.Sites = out
		st.Events = appendEvent(st.Events, model.Event{ID: newID("evt"), SiteID: id, Kind: "site.deleted", Severity: "info", Message: "Site removed from fleet", CreatedAt: time.Now().UTC()})
		return nil
	})
	if err != nil {
		if conflict {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, 500, "could not remove site")
		return
	}
	if !found {
		writeError(w, 404, "site not found")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) listEnrollmentTokens(w http.ResponseWriter, _ *http.Request) {
	st := s.store.Snapshot()
	type view struct {
		ID        string     `json:"id"`
		Name      string     `json:"name"`
		CreatedAt time.Time  `json:"createdAt"`
		ExpiresAt *time.Time `json:"expiresAt,omitempty"`
		MaxUses   int        `json:"maxUses"`
		Uses      int        `json:"uses"`
	}
	out := make([]view, 0, len(st.EnrollmentTokens))
	for _, t := range st.EnrollmentTokens {
		out = append(out, view{t.ID, t.Name, t.CreatedAt, t.ExpiresAt, t.MaxUses, t.Uses})
	}
	writeJSON(w, 200, out)
}

func (s *Server) createEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name         string `json:"name"`
		ExpiresHours int    `json:"expiresHours"`
		MaxUses      int    `json:"maxUses"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = "Fleet enrollment"
	}
	if in.MaxUses <= 0 {
		in.MaxUses = 1
	}
	if in.MaxUses > 10000 {
		writeError(w, 400, "maxUses too large")
		return
	}
	plain, err := auth.RandomToken("zf_enroll_")
	if err != nil {
		writeError(w, 500, "could not create token")
		return
	}
	now := time.Now().UTC()
	var expires *time.Time
	if in.ExpiresHours > 0 {
		v := now.Add(time.Duration(in.ExpiresHours) * time.Hour)
		expires = &v
	}
	t := model.EnrollmentToken{ID: newID("enr"), Name: strings.TrimSpace(in.Name), TokenHash: auth.SHA256Token(plain), CreatedAt: now, ExpiresAt: expires, MaxUses: in.MaxUses}
	if err := s.store.Update(func(st *model.State) error { st.EnrollmentTokens = append(st.EnrollmentTokens, t); return nil }); err != nil {
		writeError(w, 500, "could not save token")
		return
	}
	writeJSON(w, 201, map[string]any{"id": t.ID, "name": t.Name, "token": plain, "expiresAt": t.ExpiresAt, "maxUses": t.MaxUses})
}

func (s *Server) deleteEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	found := false
	err := s.store.Update(func(st *model.State) error {
		out := st.EnrollmentTokens[:0]
		for _, t := range st.EnrollmentTokens {
			if t.ID == id {
				found = true
				continue
			}
			out = append(out, t)
		}
		st.EnrollmentTokens = out
		return nil
	})
	if err != nil {
		writeError(w, 500, "could not revoke token")
		return
	}
	if !found {
		writeError(w, 404, "token not found")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) listRevisions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, s.store.Snapshot().Revisions)
}

func (s *Server) createRevision(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name, Notes string
		Workloads   []model.RuntimeSpec
	}
	if err := decodeJSON(r, &in, 1<<20); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeError(w, 400, "name is required")
		return
	}
	if len(in.Workloads) > 100 {
		writeError(w, 400, "too many workloads")
		return
	}
	for _, spec := range in.Workloads {
		if err := validateRuntimeSpec(spec); err != nil {
			writeError(w, 400, err.Error())
			return
		}
	}
	c := claimsFrom(r.Context())
	rev := model.Revision{ID: newID("rev"), Name: strings.TrimSpace(in.Name), Notes: strings.TrimSpace(in.Notes), Workloads: in.Workloads, CreatedAt: time.Now().UTC(), CreatedBy: c.Email}
	if err := s.store.Update(func(st *model.State) error { st.Revisions = append(st.Revisions, rev); return nil }); err != nil {
		writeError(w, 500, "could not save revision")
		return
	}
	writeJSON(w, 201, rev)
}

func validateRuntimeSpec(spec model.RuntimeSpec) error {
	allowed := map[string]bool{"systemd": true, "container": true, "k3s": true, "qemu": true}
	if !allowed[spec.Kind] {
		return fmt.Errorf("unsupported runtime kind %q", spec.Kind)
	}
	if strings.TrimSpace(spec.Name) == "" {
		return errors.New("workload name is required")
	}
	if spec.State != "running" && spec.State != "stopped" && spec.State != "present" {
		return fmt.Errorf("invalid desired state for %s", spec.Name)
	}
	if spec.Kind == "container" && spec.State != "stopped" {
		image := strings.TrimSpace(spec.Image)
		if image == "" {
			return fmt.Errorf("container %s requires image", spec.Name)
		}
		if strings.HasPrefix(image, "-") || strings.ContainsAny(image, " \t\r\n\x00") || len(image) > 512 {
			return fmt.Errorf("container %s has an unsafe image reference", spec.Name)
		}
		for key := range spec.Env {
			if key == "" || strings.ContainsAny(key, "=\x00\r\n") {
				return fmt.Errorf("container %s has an invalid environment key", spec.Name)
			}
		}
		for _, port := range spec.Ports {
			if strings.TrimSpace(port) == "" || strings.HasPrefix(strings.TrimSpace(port), "-") || strings.ContainsAny(port, " \t\r\n\x00") {
				return fmt.Errorf("container %s has an invalid port mapping", spec.Name)
			}
		}
	}
	if spec.Kind == "qemu" && spec.State != "stopped" && spec.Disk == "" {
		return fmt.Errorf("qemu workload %s requires disk", spec.Name)
	}
	if spec.Kind == "k3s" && spec.Manifest == "" {
		return fmt.Errorf("k3s workload %s requires manifest", spec.Name)
	}
	if spec.Health != nil {
		if spec.Health.TimeoutSeconds < 0 || spec.Health.TimeoutSeconds > 30 {
			return fmt.Errorf("health timeout for %s must be between 0 and 30 seconds", spec.Name)
		}
		if spec.Health.GraceSeconds < 0 || spec.Health.GraceSeconds > 300 {
			return fmt.Errorf("health grace for %s must be between 0 and 300 seconds", spec.Name)
		}
		switch spec.Health.Type {
		case "", "runtime":
		case "http":
			if !strings.HasPrefix(spec.Health.URL, "http://") && !strings.HasPrefix(spec.Health.URL, "https://") {
				return fmt.Errorf("http health probe for %s requires an http(s) URL", spec.Name)
			}
		case "tcp":
			if strings.TrimSpace(spec.Health.Address) == "" || !strings.Contains(spec.Health.Address, ":") {
				return fmt.Errorf("tcp health probe for %s requires host:port", spec.Name)
			}
		default:
			return fmt.Errorf("unsupported health probe type %q", spec.Health.Type)
		}
	}
	return nil
}

func (s *Server) listRollouts(w http.ResponseWriter, _ *http.Request) {
	s.markOfflineAndReconcile()
	writeJSON(w, 200, s.store.Snapshot().Rollouts)
}

type rolloutRequest struct {
	Name             string            `json:"name"`
	RevisionID       string            `json:"revisionId"`
	Strategy         string            `json:"strategy"`
	WaveSize         int               `json:"waveSize"`
	PauseSeconds     int               `json:"pauseSeconds"`
	MaxFailures      int               `json:"maxFailures"`
	SiteIDs          []string          `json:"siteIds"`
	GroupID          string            `json:"groupId"`
	Selector         map[string]string `json:"selector"`
	AutoRollback     bool              `json:"autoRollback"`
	ApprovalRequired bool              `json:"approvalRequired"`
	StartAt          *time.Time        `json:"startAt"`
	EndAt            *time.Time        `json:"endAt"`
}

func (s *Server) planRollout(w http.ResponseWriter, r *http.Request) {
	var in rolloutRequest
	if err := decodeJSON(r, &in, 128<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	st := s.store.Snapshot()
	if in.RevisionID != "" && !revisionExists(st, in.RevisionID) {
		writeError(w, 400, "revision not found")
		return
	}
	selector, err := normalizeLabels(in.Selector)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	targets, err := resolveRolloutSites(st, in.SiteIDs, in.GroupID, selector)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	type target struct {
		ID, Name, Region, Status string
		Labels                   map[string]string
	}
	out := make([]target, 0, len(targets))
	offline := 0
	for _, id := range targets {
		for _, site := range st.Sites {
			if site.ID == id {
				out = append(out, target{ID: site.ID, Name: site.Name, Region: site.Region, Status: site.Status, Labels: site.Labels})
				if site.Status != "online" {
					offline++
				}
				break
			}
		}
	}
	waveSize := in.WaveSize
	if waveSize <= 0 {
		waveSize = 10
	}
	if waveSize > len(targets) && len(targets) > 0 {
		waveSize = len(targets)
	}
	waves := 0
	if waveSize > 0 {
		waves = (len(targets) + waveSize - 1) / waveSize
	}
	writeJSON(w, 200, map[string]any{"siteIds": targets, "targets": out, "total": len(targets), "offline": offline, "waveSize": waveSize, "waves": waves})
}

func (s *Server) createRollout(w http.ResponseWriter, r *http.Request) {
	var in rolloutRequest
	if err := decodeJSON(r, &in, 128<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	st := s.store.Snapshot()
	var rev *model.Revision
	for i := range st.Revisions {
		if st.Revisions[i].ID == in.RevisionID {
			rev = &st.Revisions[i]
			break
		}
	}
	if rev == nil {
		writeError(w, 400, "revision not found")
		return
	}
	selector, err := normalizeLabels(in.Selector)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	targets, err := resolveRolloutSites(st, in.SiteIDs, in.GroupID, selector)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if len(targets) == 0 {
		writeError(w, 400, "no sites selected")
		return
	}
	if in.WaveSize <= 0 {
		in.WaveSize = 10
	}
	if in.WaveSize > len(targets) {
		in.WaveSize = len(targets)
	}
	if in.Strategy == "" {
		in.Strategy = "waves"
	}
	if in.Strategy != "waves" && in.Strategy != "all-at-once" {
		writeError(w, 400, "strategy must be waves or all-at-once")
		return
	}
	if in.Strategy == "all-at-once" {
		in.WaveSize = len(targets)
	}
	if in.PauseSeconds < 0 || in.PauseSeconds > 86400 {
		writeError(w, 400, "pauseSeconds must be between 0 and 86400")
		return
	}
	if in.MaxFailures < 0 || in.MaxFailures >= len(targets) {
		writeError(w, 400, "maxFailures must be between 0 and target count - 1")
		return
	}
	now := time.Now().UTC()
	if in.StartAt != nil && in.EndAt != nil && !in.EndAt.After(*in.StartAt) {
		writeError(w, 400, "endAt must be after startAt")
		return
	}
	if in.EndAt != nil && !in.EndAt.After(now) {
		writeError(w, 400, "endAt must be in the future")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = rev.Name + " rollout"
	}
	status := "running"
	if in.ApprovalRequired {
		status = "pending_approval"
	} else if in.StartAt != nil && in.StartAt.After(now) {
		status = "scheduled"
	}
	previous := map[string]string{}
	for _, site := range st.Sites {
		if contains(targets, site.ID) {
			previous[site.ID] = site.AppliedRevision
			if previous[site.ID] == "" {
				previous[site.ID] = site.DesiredRevision
			}
		}
	}
	if in.AutoRollback {
		for _, id := range targets {
			if previous[id] == "" {
				writeError(w, 400, "autoRollback requires every target site to have a previous applied revision")
				return
			}
		}
	}
	c := claimsFrom(r.Context())
	ro := model.Rollout{ID: newID("rol"), Name: strings.TrimSpace(in.Name), RevisionID: in.RevisionID, Status: status, Strategy: in.Strategy, WaveSize: in.WaveSize, PauseSeconds: in.PauseSeconds, MaxFailures: in.MaxFailures, AutoRollback: in.AutoRollback, ApprovalRequired: in.ApprovalRequired, StartAt: in.StartAt, EndAt: in.EndAt, SiteIDs: targets, PreviousRevisions: previous, CreatedAt: now, UpdatedAt: now, CreatedBy: c.Email}
	if err := s.store.Update(func(state *model.State) error {
		state.Rollouts = append(state.Rollouts, ro)
		idx := len(state.Rollouts) - 1
		if state.Rollouts[idx].Status == "running" {
			activateWave(state, &state.Rollouts[idx], now)
		}
		state.Events = appendEvent(state.Events, model.Event{ID: newID("evt"), Kind: "rollout.started", Severity: "info", Message: fmt.Sprintf("%s created for %d sites (%s)", ro.Name, len(ro.SiteIDs), ro.Status), CreatedAt: now})
		return nil
	}); err != nil {
		writeError(w, 500, "could not start rollout")
		return
	}
	for _, saved := range s.store.Snapshot().Rollouts {
		if saved.ID == ro.ID {
			writeJSON(w, 201, saved)
			return
		}
	}
	writeJSON(w, 201, ro)
}

func (s *Server) pauseRollout(w http.ResponseWriter, r *http.Request) {
	s.transitionRollout(w, r, "pause")
}
func (s *Server) resumeRollout(w http.ResponseWriter, r *http.Request) {
	s.transitionRollout(w, r, "resume")
}
func (s *Server) abortRollout(w http.ResponseWriter, r *http.Request) {
	s.transitionRollout(w, r, "abort")
}
func (s *Server) approveRollout(w http.ResponseWriter, r *http.Request) {
	s.transitionRollout(w, r, "approve")
}
func (s *Server) rollbackRollout(w http.ResponseWriter, r *http.Request) {
	s.transitionRollout(w, r, "rollback")
}
func (s *Server) retryRollout(w http.ResponseWriter, r *http.Request) {
	s.transitionRollout(w, r, "retry")
}

func (s *Server) transitionRollout(w http.ResponseWriter, r *http.Request, action string) {
	id := r.PathValue("id")
	now := time.Now().UTC()
	var out model.Rollout
	found := false
	err := s.store.Update(func(st *model.State) error {
		for i := range st.Rollouts {
			ro := &st.Rollouts[i]
			if ro.ID != id {
				continue
			}
			found = true
			switch action {
			case "pause":
				if ro.Status != "running" && ro.Status != "scheduled" {
					return fmt.Errorf("rollout cannot be paused from %s", ro.Status)
				}
				ro.Status = "paused"
				ro.UpdatedAt = now
			case "resume":
				if ro.Status != "paused" {
					return fmt.Errorf("rollout cannot be resumed from %s", ro.Status)
				}
				if ro.EndAt != nil && !ro.EndAt.After(now) {
					return errors.New("rollout maintenance window has ended")
				}
				if ro.StartAt != nil && ro.StartAt.After(now) {
					ro.Status = "scheduled"
				} else {
					ro.Status = "running"
					activateWave(st, ro, now)
				}
				ro.UpdatedAt = now
			case "abort":
				if terminalRollout(ro.Status) {
					return fmt.Errorf("rollout is already %s", ro.Status)
				}
				ro.Status = "aborted"
				ro.UpdatedAt = now
			case "approve":
				if ro.Status != "pending_approval" {
					return fmt.Errorf("rollout cannot be approved from %s", ro.Status)
				}
				if ro.EndAt != nil && !ro.EndAt.After(now) {
					return errors.New("rollout maintenance window has ended")
				}
				ro.ApprovedAt = &now
				if ro.StartAt != nil && ro.StartAt.After(now) {
					ro.Status = "scheduled"
				} else {
					ro.Status = "running"
					activateWave(st, ro, now)
				}
				ro.UpdatedAt = now
			case "rollback":
				for _, id := range ro.SiteIDs {
					if ro.PreviousRevisions[id] == "" {
						return errors.New("rollback requires a previous revision for every target site")
					}
				}
				rollbackRolloutState(st, ro)
				ro.RolledBack = true
				ro.Status = "rolled_back"
				ro.UpdatedAt = now
			case "retry":
				if ro.Status != "failed" {
					return fmt.Errorf("rollout cannot be retried from %s", ro.Status)
				}
				if ro.EndAt != nil && !ro.EndAt.After(now) {
					return errors.New("rollout maintenance window has ended")
				}
				ro.Status = "running"
				ro.RolledBack = false
				ro.FailedSites = nil
				ro.NextWaveAt = nil
				ro.UpdatedAt = now
				for si := range st.Sites {
					site := &st.Sites[si]
					if !contains(ro.ActivatedSites, site.ID) {
						continue
					}
					site.DesiredRevision = ro.RevisionID
					site.FailedRevision = ""
					site.RevisionError = ""
				}
			default:
				return errors.New("unknown rollout action")
			}
			verbs := map[string]string{"pause": "paused", "resume": "resumed", "abort": "aborted", "approve": "approved", "rollback": "rolled back", "retry": "retried"}
			st.Events = appendEvent(st.Events, model.Event{ID: newID("evt"), Kind: "rollout." + action, Severity: "info", Message: ro.Name + " " + verbs[action], CreatedAt: now})
			out = *ro
			return nil
		}
		return errors.New("rollout not found")
	})
	if err != nil {
		status := 400
		if !found {
			status = 404
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v, _ := strconv.Atoi(r.URL.Query().Get("limit")); v > 0 && v <= 500 {
		limit = v
	}
	events := tailEvents(s.store.Snapshot().Events, limit)
	// newest first for operators
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	writeJSON(w, 200, events)
}

func (s *Server) agentRegister(w http.ResponseWriter, r *http.Request) {
	enrollment := bearer(r)
	if enrollment == "" {
		writeError(w, 401, "enrollment token required")
		return
	}
	var in struct {
		Name, Region, AgentVersion string
		Labels                     map[string]string
		Inventory                  model.Inventory
	}
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, 400, "site name is required")
		return
	}
	labels, err := normalizeLabels(in.Labels)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	in.Labels = labels
	agentToken, err := auth.RandomToken("zf_agent_")
	if err != nil {
		writeError(w, 500, "could not create site identity")
		return
	}
	now := time.Now().UTC()
	site := model.Site{ID: newID("site"), Name: in.Name, Region: strings.TrimSpace(in.Region), Labels: in.Labels, Status: "online", LastSeen: now, CreatedAt: now, AgentVersion: in.AgentVersion, AgentTokenHash: auth.SHA256Token(agentToken), Inventory: in.Inventory, IntegrationState: map[string]string{"nodra": "available", "packetwolf": "available"}}
	validToken := false
	err = s.store.Update(func(st *model.State) error {
		h := auth.SHA256Token(enrollment)
		for i := range st.EnrollmentTokens {
			t := &st.EnrollmentTokens[i]
			if subtle.ConstantTimeCompare([]byte(t.TokenHash), []byte(h)) != 1 {
				continue
			}
			if t.ExpiresAt != nil && now.After(*t.ExpiresAt) {
				continue
			}
			if t.MaxUses > 0 && t.Uses >= t.MaxUses {
				continue
			}
			t.Uses++
			validToken = true
			break
		}
		if !validToken {
			return errors.New("invalid enrollment token")
		}
		for _, existing := range st.Sites {
			if strings.EqualFold(existing.Name, in.Name) {
				return errors.New("site name already enrolled")
			}
		}
		st.Sites = append(st.Sites, site)
		st.Events = appendEvent(st.Events, model.Event{ID: newID("evt"), SiteID: site.ID, Kind: "site.enrolled", Severity: "success", Message: "Site " + site.Name + " joined the fleet", CreatedAt: now})
		return nil
	})
	if err != nil {
		writeError(w, 401, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"siteId": site.ID, "agentToken": agentToken, "heartbeatSeconds": 15})
}

func (s *Server) agentHeartbeat(w http.ResponseWriter, r *http.Request) {
	siteID, ok := s.authenticateAgent(r)
	if !ok {
		writeError(w, 401, "invalid site identity")
		return
	}
	var in struct {
		Inventory                     model.Inventory `json:"inventory"`
		AgentVersion, AppliedRevision string
		FailedRevision, RevisionError string
		QueuedEvents                  int
		AutonomyMode                  bool
		WorkloadHealth                []model.WorkloadHealth
		Events                        []model.Event
	}
	if err := decodeJSON(r, &in, 512<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	now := time.Now().UTC()
	found := false
	err := s.store.Update(func(st *model.State) error {
		for i := range st.Sites {
			if st.Sites[i].ID == siteID {
				site := &st.Sites[i]
				found = true
				wasOffline := site.Status == "offline"
				site.Status = "online"
				site.LastSeen = now
				site.OfflineSince = nil
				site.Inventory = in.Inventory
				site.AgentVersion = in.AgentVersion
				site.QueuedEvents = in.QueuedEvents
				site.AutonomyMode = in.AutonomyMode
				site.FailedRevision = in.FailedRevision
				site.RevisionError = strings.TrimSpace(in.RevisionError)
				site.WorkloadHealth = append([]model.WorkloadHealth(nil), in.WorkloadHealth...)
				if in.FailedRevision != "" {
					site.Status = "degraded"
				}
				if in.AppliedRevision != "" {
					site.AppliedRevision = in.AppliedRevision
				}
				if wasOffline {
					st.Events = appendEvent(st.Events, model.Event{ID: newID("evt"), SiteID: site.ID, Kind: "site.reconnected", Severity: "success", Message: site.Name + " reconnected", CreatedAt: now})
				}
				break
			}
		}
		if !found {
			return errors.New("site not found")
		}
		for _, evt := range in.Events {
			evt.ID = newID("evt")
			evt.SiteID = siteID
			if evt.CreatedAt.IsZero() {
				evt.CreatedAt = now
			}
			st.Events = appendEvent(st.Events, evt)
		}
		reconcileRollouts(st)
		return nil
	})
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "serverTime": now})
}

func (s *Server) agentSync(w http.ResponseWriter, r *http.Request) {
	siteID, ok := s.authenticateAgent(r)
	if !ok {
		writeError(w, 401, "invalid site identity")
		return
	}
	s.markOfflineAndReconcile()
	st := s.store.Snapshot()
	var site *model.Site
	for i := range st.Sites {
		if st.Sites[i].ID == siteID {
			site = &st.Sites[i]
			break
		}
	}
	if site == nil {
		writeError(w, 404, "site not found")
		return
	}
	var rev *model.Revision
	if site.DesiredRevision != "" {
		for i := range st.Revisions {
			if st.Revisions[i].ID == site.DesiredRevision {
				rev = &st.Revisions[i]
				break
			}
		}
	}
	commands := []model.Command{}
	for _, c := range st.Commands {
		if c.SiteID == siteID && c.Status == "pending" {
			commands = append(commands, c)
		}
	}
	writeJSON(w, 200, map[string]any{"siteId": siteID, "desiredRevision": site.DesiredRevision, "revision": rev, "commands": commands, "syncAfterSeconds": 15})
}

func (s *Server) agentAck(w http.ResponseWriter, r *http.Request) {
	siteID, ok := s.authenticateAgent(r)
	if !ok {
		writeError(w, 401, "invalid site identity")
		return
	}
	var in struct{ CommandID, Status, Error, AppliedRevision string }
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	now := time.Now().UTC()
	err := s.store.Update(func(st *model.State) error {
		for i := range st.Commands {
			if st.Commands[i].ID == in.CommandID && st.Commands[i].SiteID == siteID {
				st.Commands[i].Status = in.Status
				st.Commands[i].Error = in.Error
				st.Commands[i].UpdatedAt = now
			}
		}
		if in.AppliedRevision != "" {
			for i := range st.Sites {
				if st.Sites[i].ID == siteID {
					st.Sites[i].AppliedRevision = in.AppliedRevision
				}
			}
		}
		reconcileRollouts(st)
		return nil
	})
	if err != nil {
		writeError(w, 500, "could not acknowledge command")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func defaultIntegrations() []model.Integration {
	return []model.Integration{
		{ID: "nodra", Name: "Nodra", Purpose: "Edge data, MQTT/HTTP ingress, twins and local routes"},
		{ID: "packetwolf", Name: "PacketWolf", Purpose: "eBPF network intelligence and policy verification"},
		{ID: "relay", Name: "Zyvor Relay", Purpose: "Durable operational actions and verification"},
		{ID: "argus", Name: "Argus", Purpose: "Post-rollout application assurance"},
		{ID: "forge", Name: "Forge", Purpose: "GPU and edge inference operations"},
	}
}

func publicIntegration(it model.Integration) map[string]any {
	keys := make([]string, 0, len(it.Config))
	for k := range it.Config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return map[string]any{
		"id": it.ID, "name": it.Name, "purpose": it.Purpose,
		"enabled": it.Enabled, "configuredFields": keys,
	}
}

func (s *Server) integrations(w http.ResponseWriter, _ *http.Request) {
	st := s.store.Snapshot()
	if len(st.Integrations) == 0 {
		if err := s.store.Update(func(st *model.State) error {
			if len(st.Integrations) == 0 {
				st.Integrations = defaultIntegrations()
			}
			return nil
		}); err != nil {
			writeError(w, 500, "could not initialize integrations")
			return
		}
		st = s.store.Snapshot()
	}
	out := make([]map[string]any, 0, len(st.Integrations))
	for _, it := range st.Integrations {
		out = append(out, publicIntegration(it))
	}
	writeJSON(w, 200, out)
}

func (s *Server) updateIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Enabled *bool             `json:"enabled"`
		Config  map[string]string `json:"config"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	found := false
	var updated model.Integration
	err := s.store.Update(func(st *model.State) error {
		if len(st.Integrations) == 0 {
			st.Integrations = defaultIntegrations()
		}
		for i := range st.Integrations {
			if st.Integrations[i].ID != id {
				continue
			}
			found = true
			if in.Enabled != nil {
				st.Integrations[i].Enabled = *in.Enabled
			}
			for k, v := range in.Config {
				k = strings.TrimSpace(k)
				if k == "" {
					continue
				}
				if st.Integrations[i].Config == nil {
					st.Integrations[i].Config = map[string]string{}
				}
				if v == "" {
					delete(st.Integrations[i].Config, k)
					continue
				}
				st.Integrations[i].Config[k] = v
			}
			updated = st.Integrations[i]
			break
		}
		if !found {
			return errors.New("integration not found")
		}
		return nil
	})
	if err != nil {
		status := 400
		if !found {
			status = 404
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, 200, publicIntegration(updated))
}

func (s *Server) authenticateAgent(r *http.Request) (string, bool) {
	siteID, token := strings.TrimSpace(r.Header.Get("X-Zyvor-Site-ID")), bearer(r)
	if siteID == "" || token == "" {
		return "", false
	}
	h := auth.SHA256Token(token)
	st := s.store.Snapshot()
	for _, site := range st.Sites {
		if site.ID == siteID && subtle.ConstantTimeCompare([]byte(site.AgentTokenHash), []byte(h)) == 1 {
			return siteID, true
		}
	}
	return "", false
}

func (s *Server) markOfflineAndReconcile() {
	now := time.Now().UTC()
	_ = s.store.Update(func(st *model.State) error {
		for i := range st.Sites {
			site := &st.Sites[i]
			if (site.Status == "online" || site.Status == "degraded") && !site.LastSeen.IsZero() && now.Sub(site.LastSeen) > s.cfg.SiteTimeout {
				site.Status = "offline"
				site.AutonomyMode = true
				t := site.LastSeen.Add(s.cfg.SiteTimeout)
				site.OfflineSince = &t
				st.Events = appendEvent(st.Events, model.Event{ID: newID("evt"), SiteID: site.ID, Kind: "site.offline", Severity: "warning", Message: site.Name + " is operating without the control plane", CreatedAt: now})
			}
		}
		reconcileRollouts(st)
		return nil
	})
}

func reconcileRollouts(st *model.State) {
	reconcileRolloutsAt(st, time.Now().UTC())
}

func reconcileRolloutsAt(st *model.State, now time.Time) {
	for ri := range st.Rollouts {
		ro := &st.Rollouts[ri]
		if ro.Status == "scheduled" {
			if ro.EndAt != nil && !ro.EndAt.After(now) {
				ro.Status = "expired"
				ro.UpdatedAt = now
				continue
			}
			if ro.StartAt == nil || !ro.StartAt.After(now) {
				ro.Status = "running"
				ro.UpdatedAt = now
			}
		}
		if ro.Status != "running" {
			continue
		}
		if ro.EndAt != nil && !ro.EndAt.After(now) {
			ro.Status = "expired"
			ro.UpdatedAt = now
			continue
		}

		completed := map[string]bool{}
		failed := map[string]bool{}
		for _, id := range ro.FailedSites {
			failed[id] = true
		}
		for _, site := range st.Sites {
			if !contains(ro.SiteIDs, site.ID) {
				continue
			}
			if site.AppliedRevision == ro.RevisionID {
				completed[site.ID] = true
				delete(failed, site.ID)
				continue
			}
			if site.FailedRevision == ro.RevisionID {
				failed[site.ID] = true
			}
		}
		ro.CompletedSites = keysInOrder(completed, ro.SiteIDs)
		ro.FailedSites = keysInOrder(failed, ro.SiteIDs)

		if len(ro.FailedSites) > ro.MaxFailures {
			ro.Status = "failed"
			ro.UpdatedAt = now
			if ro.AutoRollback {
				rollbackRolloutState(st, ro)
				ro.RolledBack = true
			}
			st.Events = appendEvent(st.Events, model.Event{ID: newID("evt"), Kind: "rollout.failed", Severity: "error", Message: fmt.Sprintf("%s stopped after %d failed site(s)", ro.Name, len(ro.FailedSites)), CreatedAt: now})
			continue
		}

		if len(ro.CompletedSites)+len(ro.FailedSites) >= len(ro.SiteIDs) {
			if len(ro.FailedSites) > 0 {
				ro.Status = "completed_with_failures"
			} else {
				ro.Status = "completed"
			}
			ro.UpdatedAt = now
			ro.CompletedAt = &now
			continue
		}

		if activeRolloutSites(ro) > 0 {
			continue
		}
		if len(ro.ActivatedSites) > 0 && ro.PauseSeconds > 0 {
			if ro.NextWaveAt == nil {
				next := now.Add(time.Duration(ro.PauseSeconds) * time.Second)
				ro.NextWaveAt = &next
				ro.UpdatedAt = now
				continue
			}
			if ro.NextWaveAt.After(now) {
				continue
			}
			ro.NextWaveAt = nil
		}
		activateWave(st, ro, now)
	}
}

func activeRolloutSites(ro *model.Rollout) int {
	completed := map[string]bool{}
	failed := map[string]bool{}
	for _, id := range ro.CompletedSites {
		completed[id] = true
	}
	for _, id := range ro.FailedSites {
		failed[id] = true
	}
	active := 0
	for _, id := range ro.ActivatedSites {
		if !completed[id] && !failed[id] {
			active++
		}
	}
	return active
}

func activateWave(st *model.State, ro *model.Rollout, now time.Time) {
	if ro.Status != "running" {
		return
	}
	activated := map[string]bool{}
	completed := map[string]bool{}
	failed := map[string]bool{}
	for _, id := range ro.ActivatedSites {
		activated[id] = true
	}
	for _, id := range ro.CompletedSites {
		completed[id] = true
	}
	for _, id := range ro.FailedSites {
		failed[id] = true
	}
	capacity := ro.WaveSize
	newSites := []string{}
	for _, id := range ro.SiteIDs {
		if capacity <= 0 {
			break
		}
		if activated[id] || completed[id] || failed[id] {
			continue
		}
		for i := range st.Sites {
			site := &st.Sites[i]
			if site.ID != id {
				continue
			}
			site.DesiredRevision = ro.RevisionID
			site.FailedRevision = ""
			site.RevisionError = ""
			activated[id] = true
			newSites = append(newSites, id)
			capacity--
			break
		}
	}
	if len(newSites) == 0 {
		return
	}
	ro.ActivatedSites = keysInOrder(activated, ro.SiteIDs)
	ro.CurrentWave++
	ro.UpdatedAt = now
	st.Events = appendEvent(st.Events, model.Event{ID: newID("evt"), Kind: "rollout.wave", Severity: "info", Message: fmt.Sprintf("%s activated wave %d for %d site(s)", ro.Name, ro.CurrentWave, len(newSites)), CreatedAt: now})
}

func rollbackRolloutState(st *model.State, ro *model.Rollout) {
	for i := range st.Sites {
		site := &st.Sites[i]
		if !contains(ro.SiteIDs, site.ID) || site.DesiredRevision != ro.RevisionID {
			continue
		}
		site.DesiredRevision = ro.PreviousRevisions[site.ID]
		site.FailedRevision = ""
		site.RevisionError = ""
	}
}

func terminalRollout(status string) bool {
	switch status {
	case "completed", "completed_with_failures", "failed", "aborted", "rolled_back", "expired":
		return true
	default:
		return false
	}
}

func revisionExists(st model.State, id string) bool {
	for _, rev := range st.Revisions {
		if rev.ID == id {
			return true
		}
	}
	return false
}

func resolveRolloutSites(st model.State, explicit []string, groupID string, selector map[string]string) ([]string, error) {
	valid := map[string]bool{}
	for _, site := range st.Sites {
		valid[site.ID] = true
	}
	selected := []string{}
	for _, id := range explicit {
		if !valid[id] {
			return nil, fmt.Errorf("unknown site: %s", id)
		}
		selected = append(selected, id)
	}
	if groupID != "" {
		found := false
		for _, g := range st.SiteGroups {
			if g.ID == groupID {
				selected = append(selected, resolveGroupSites(st, g)...)
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("site group not found")
		}
	}
	if len(selector) > 0 {
		for _, site := range st.Sites {
			if labelsMatch(site.Labels, selector) {
				selected = append(selected, site.ID)
			}
		}
	}
	if len(selected) == 0 && len(explicit) == 0 && groupID == "" && len(selector) == 0 {
		for _, site := range st.Sites {
			selected = append(selected, site.ID)
		}
	}
	selected = unique(selected)
	ordered := make([]string, 0, len(selected))
	set := map[string]bool{}
	for _, id := range selected {
		set[id] = true
	}
	for _, site := range st.Sites {
		if set[site.ID] {
			ordered = append(ordered, site.ID)
		}
	}
	return ordered, nil
}

func resolveGroupSites(st model.State, g model.SiteGroup) []string {
	selected := append([]string(nil), g.SiteIDs...)
	if len(g.Selector) > 0 {
		for _, site := range st.Sites {
			if labelsMatch(site.Labels, g.Selector) {
				selected = append(selected, site.ID)
			}
		}
	}
	selected = unique(selected)
	set := map[string]bool{}
	for _, id := range selected {
		set[id] = true
	}
	ordered := []string{}
	for _, site := range st.Sites {
		if set[site.ID] {
			ordered = append(ordered, site.ID)
		}
	}
	return ordered
}

func labelsMatch(labels, selector map[string]string) bool {
	for key, want := range selector {
		if labels[key] != want {
			return false
		}
	}
	return true
}

func normalizeLabels(in map[string]string) (map[string]string, error) {
	if in == nil {
		return nil, nil
	}
	if len(in) > 32 {
		return nil, errors.New("too many labels")
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || len(key) > 63 || len(value) > 128 {
			return nil, errors.New("label keys must be 1-63 chars and values at most 128 chars")
		}
		for _, r := range key {
			if !(r == '-' || r == '_' || r == '.' || r == '/' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
				return nil, fmt.Errorf("invalid label key %q", key)
			}
		}
		out[key] = value
	}
	return out, nil
}

func (s *Server) requireAuth(next http.Handler) http.Handler { return s.requireRoles(next) }
func (s *Server) requireRoles(next http.Handler, roles ...model.Role) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("zyvor_fleet_session")
		if err != nil {
			writeError(w, 401, "sign in required")
			return
		}
		claims, err := s.sessions.Verify(cookie.Value)
		if err != nil {
			writeError(w, 401, "session expired")
			return
		}
		var current *model.User
		st := s.store.Snapshot()
		for i := range st.Users {
			if st.Users[i].ID == claims.Subject {
				current = &st.Users[i]
				break
			}
		}
		if current == nil || current.AuthVersion != claims.Version {
			writeError(w, 401, "session revoked")
			return
		}
		claims.Email = current.Email
		claims.Name = current.Name
		claims.Role = current.Role
		if len(roles) > 0 {
			allowed := false
			for _, role := range roles {
				if claims.Role == role {
					allowed = true
				}
			}
			if !allowed {
				writeError(w, 403, "insufficient role")
				return
			}
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && r.Header.Get("X-Zyvor-Request") != "1" {
			writeError(w, 403, "missing same-origin request marker")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey, claims)))
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.cfg.Logger.Debug("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start).String())
		}
	})
}

func publicUser(u model.User) map[string]any {
	return map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "role": u.Role}
}
func claimsFrom(ctx context.Context) auth.Claims {
	c, _ := ctx.Value(claimsKey).(auth.Claims)
	return c
}
func bearer(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(v) > 7 && strings.EqualFold(v[:7], "Bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return ""
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func decodeJSON(r *http.Request, dst any, max int64) error {
	r.Body = http.MaxBytesReader(nil, r.Body, max)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request must contain one JSON object")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg, "status": status})
}
func appendEvent(events []model.Event, e model.Event) []model.Event {
	events = append(events, e)
	if len(events) > 5000 {
		events = append([]model.Event(nil), events[len(events)-5000:]...)
	}
	return events
}
func tailEvents(events []model.Event, n int) []model.Event {
	if n > len(events) {
		n = len(events)
	}
	return append([]model.Event(nil), events[len(events)-n:]...)
}
func unique(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range in {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func keysInOrder(set map[string]bool, order []string) []string {
	out := []string{}
	for _, x := range order {
		if set[x] {
			out = append(out, x)
		}
	}
	return out
}

func newID(prefix string) string {
	t, _ := auth.RandomToken("")
	t = strings.TrimRight(t, "=")
	if len(t) > 14 {
		t = t[:14]
	}
	return prefix + "_" + t
}

type loginBucket struct {
	failures     int
	window       time.Time
	blockedUntil time.Time
}
type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]loginBucket
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{buckets: map[string]loginBucket{}} }
func (l *loginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.buckets[key]
	return time.Now().After(b.blockedUntil)
}
func (l *loginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b := l.buckets[key]
	if b.window.IsZero() || now.Sub(b.window) > 5*time.Minute {
		b = loginBucket{window: now}
	}
	b.failures++
	if b.failures >= 5 {
		b.blockedUntil = now.Add(5 * time.Minute)
	}
	l.buckets[key] = b
}
func (l *loginLimiter) Success(key string) { l.mu.Lock(); defer l.mu.Unlock(); delete(l.buckets, key) }
