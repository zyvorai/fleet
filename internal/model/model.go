// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Role         Role      `json:"role"`
	PasswordHash string    `json:"passwordHash,omitempty"`
	AuthVersion  int       `json:"authVersion,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Inventory struct {
	Hostname     string            `json:"hostname"`
	OS           string            `json:"os"`
	Arch         string            `json:"arch"`
	Kernel       string            `json:"kernel,omitempty"`
	CPUCount     int               `json:"cpuCount"`
	MemoryBytes  uint64            `json:"memoryBytes,omitempty"`
	Runtimes     []string          `json:"runtimes,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Addresses    []string          `json:"addresses,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type Site struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Region            string            `json:"region,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Status            string            `json:"status"`
	LastSeen          time.Time         `json:"lastSeen"`
	CreatedAt         time.Time         `json:"createdAt"`
	AgentVersion      string            `json:"agentVersion,omitempty"`
	AgentTokenHash    string            `json:"agentTokenHash,omitempty"`
	Inventory         Inventory         `json:"inventory"`
	DesiredRevision   string            `json:"desiredRevision,omitempty"`
	AppliedRevision   string            `json:"appliedRevision,omitempty"`
	QueuedEvents      int               `json:"queuedEvents"`
	OfflineSince      *time.Time        `json:"offlineSince,omitempty"`
	AutonomyMode      bool              `json:"autonomyMode"`
	Maintenance       bool              `json:"maintenance,omitempty"`
	MaintenanceReason string            `json:"maintenanceReason,omitempty"`
	FailedRevision    string            `json:"failedRevision,omitempty"`
	RevisionError     string            `json:"revisionError,omitempty"`
	WorkloadHealth    []WorkloadHealth  `json:"workloadHealth,omitempty"`
	IntegrationState  map[string]string `json:"integrationState,omitempty"`
}

type SiteView struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Region            string            `json:"region,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Status            string            `json:"status"`
	LastSeen          time.Time         `json:"lastSeen"`
	CreatedAt         time.Time         `json:"createdAt"`
	AgentVersion      string            `json:"agentVersion,omitempty"`
	Inventory         Inventory         `json:"inventory"`
	DesiredRevision   string            `json:"desiredRevision,omitempty"`
	AppliedRevision   string            `json:"appliedRevision,omitempty"`
	QueuedEvents      int               `json:"queuedEvents"`
	OfflineSince      *time.Time        `json:"offlineSince,omitempty"`
	AutonomyMode      bool              `json:"autonomyMode"`
	Maintenance       bool              `json:"maintenance,omitempty"`
	MaintenanceReason string            `json:"maintenanceReason,omitempty"`
	FailedRevision    string            `json:"failedRevision,omitempty"`
	RevisionError     string            `json:"revisionError,omitempty"`
	WorkloadHealth    []WorkloadHealth  `json:"workloadHealth,omitempty"`
	IntegrationState  map[string]string `json:"integrationState,omitempty"`
}

func (s Site) View() SiteView {
	return SiteView{ID: s.ID, Name: s.Name, Region: s.Region, Labels: s.Labels, Status: s.Status, LastSeen: s.LastSeen, CreatedAt: s.CreatedAt, AgentVersion: s.AgentVersion, Inventory: s.Inventory, DesiredRevision: s.DesiredRevision, AppliedRevision: s.AppliedRevision, QueuedEvents: s.QueuedEvents, OfflineSince: s.OfflineSince, AutonomyMode: s.AutonomyMode, Maintenance: s.Maintenance, MaintenanceReason: s.MaintenanceReason, FailedRevision: s.FailedRevision, RevisionError: s.RevisionError, WorkloadHealth: s.WorkloadHealth, IntegrationState: s.IntegrationState}
}

type HealthProbe struct {
	Type           string `json:"type"` // runtime, http, tcp
	URL            string `json:"url,omitempty"`
	Address        string `json:"address,omitempty"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`
	GraceSeconds   int    `json:"graceSeconds,omitempty"`
	ExpectedStatus int    `json:"expectedStatus,omitempty"`
}

type WorkloadHealth struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Healthy   bool      `json:"healthy"`
	Message   string    `json:"message,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

type EnrollmentToken struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	TokenHash string     `json:"tokenHash,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	MaxUses   int        `json:"maxUses"`
	Uses      int        `json:"uses"`
}

type APIToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	TokenHash  string     `json:"tokenHash,omitempty"`
	Role       Role       `json:"role"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

type Webhook struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	URL                 string     `json:"url"`
	SigningSecret       string     `json:"signingSecret,omitempty"`
	EventKinds          []string   `json:"eventKinds,omitempty"`
	Enabled             bool       `json:"enabled"`
	CreatedAt           time.Time  `json:"createdAt"`
	LastEventID         string     `json:"lastEventId,omitempty"`
	LastDeliveryAt      *time.Time `json:"lastDeliveryAt,omitempty"`
	LastError           string     `json:"lastError,omitempty"`
	ConsecutiveFailures int        `json:"consecutiveFailures,omitempty"`
}

type AuditRecord struct {
	ID        string    `json:"id"`
	Actor     string    `json:"actor"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	RemoteIP  string    `json:"remoteIp,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Integration struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Purpose string            `json:"purpose"`
	Enabled bool              `json:"enabled"`
	Config  map[string]string `json:"config,omitempty"`
}

type Event struct {
	ID        string            `json:"id"`
	SiteID    string            `json:"siteId,omitempty"`
	Kind      string            `json:"kind"`
	Severity  string            `json:"severity"`
	Message   string            `json:"message"`
	Details   map[string]string `json:"details,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
}

type RuntimeSpec struct {
	Kind      string            `json:"kind"` // systemd, container, k3s, qemu
	Name      string            `json:"name"`
	State     string            `json:"state"` // running, stopped, present
	Image     string            `json:"image,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Ports     []string          `json:"ports,omitempty"`
	CPUs      int               `json:"cpus,omitempty"`
	MemoryMiB int               `json:"memoryMiB,omitempty"`
	Disk      string            `json:"disk,omitempty"`
	Manifest  string            `json:"manifest,omitempty"`
	Health    *HealthProbe      `json:"health,omitempty"`
}

type Revision struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Notes     string        `json:"notes,omitempty"`
	Workloads []RuntimeSpec `json:"workloads"`
	CreatedAt time.Time     `json:"createdAt"`
	CreatedBy string        `json:"createdBy"`
}

type Rollout struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	RevisionID        string            `json:"revisionId"`
	Status            string            `json:"status"`
	Strategy          string            `json:"strategy"`
	WaveSize          int               `json:"waveSize"`
	PauseSeconds      int               `json:"pauseSeconds"`
	MaxFailures       int               `json:"maxFailures"`
	AutoRollback      bool              `json:"autoRollback"`
	ApprovalRequired  bool              `json:"approvalRequired"`
	ApprovedAt        *time.Time        `json:"approvedAt,omitempty"`
	StartAt           *time.Time        `json:"startAt,omitempty"`
	EndAt             *time.Time        `json:"endAt,omitempty"`
	NextWaveAt        *time.Time        `json:"nextWaveAt,omitempty"`
	CurrentWave       int               `json:"currentWave"`
	SiteIDs           []string          `json:"siteIds"`
	ActivatedSites    []string          `json:"activatedSites,omitempty"`
	CompletedSites    []string          `json:"completedSites,omitempty"`
	FailedSites       []string          `json:"failedSites,omitempty"`
	PreviousRevisions map[string]string `json:"previousRevisions,omitempty"`
	RolledBack        bool              `json:"rolledBack,omitempty"`
	CreatedAt         time.Time         `json:"createdAt"`
	CreatedBy         string            `json:"createdBy"`
	UpdatedAt         time.Time         `json:"updatedAt"`
	CompletedAt       *time.Time        `json:"completedAt,omitempty"`
}

type SiteGroup struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Selector    map[string]string `json:"selector,omitempty"`
	SiteIDs     []string          `json:"siteIds,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	CreatedBy   string            `json:"createdBy"`
}

type Command struct {
	ID        string            `json:"id"`
	SiteID    string            `json:"siteId"`
	Type      string            `json:"type"`
	Payload   map[string]string `json:"payload,omitempty"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Error     string            `json:"error,omitempty"`
}

type State struct {
	SchemaVersion    int               `json:"schemaVersion"`
	Users            []User            `json:"users"`
	Sites            []Site            `json:"sites"`
	EnrollmentTokens []EnrollmentToken `json:"enrollmentTokens"`
	APITokens        []APIToken        `json:"apiTokens,omitempty"`
	Webhooks         []Webhook         `json:"webhooks,omitempty"`
	AuditLog         []AuditRecord     `json:"auditLog,omitempty"`
	Events           []Event           `json:"events"`
	Revisions        []Revision        `json:"revisions"`
	Rollouts         []Rollout         `json:"rollouts"`
	SiteGroups       []SiteGroup       `json:"siteGroups"`
	Commands         []Command         `json:"commands"`
	Integrations     []Integration     `json:"integrations"`
}
