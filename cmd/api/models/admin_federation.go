package models

import "time"

// AdminDomainBlockRequest represents a request to block a domain at the instance level.
type AdminDomainBlockRequest struct {
	Domain         string `json:"domain"`
	Severity       string `json:"severity"`
	RejectMedia    bool   `json:"reject_media"`
	RejectReports  bool   `json:"reject_reports"`
	PrivateComment string `json:"private_comment"`
	PublicComment  string `json:"public_comment"`
	Obfuscate      bool   `json:"obfuscate"`
}

// AdminDomainBlockResponse represents an admin domain block in API responses.
type AdminDomainBlockResponse struct {
	ID             string    `json:"id"`
	Domain         string    `json:"domain"`
	Severity       string    `json:"severity"`
	RejectMedia    bool      `json:"reject_media"`
	RejectReports  bool      `json:"reject_reports"`
	PrivateComment string    `json:"private_comment,omitempty"`
	PublicComment  string    `json:"public_comment,omitempty"`
	Obfuscate      bool      `json:"obfuscate"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AdminDomainAllowRequest represents a request to allow a domain.
type AdminDomainAllowRequest struct {
	Domain string `json:"domain"`
}

// AdminDomainAllowResponse represents a domain allow in API responses.
type AdminDomainAllowResponse struct {
	ID        string    `json:"id"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

// InstanceInfoResponse represents instance information in API responses.
type InstanceInfoResponse struct {
	Domain        string    `json:"domain"`
	Software      string    `json:"software,omitempty"`
	Version       string    `json:"version,omitempty"`
	ActiveUsers   int       `json:"active_users"`
	TotalMessages int64     `json:"total_messages"`
	TrustScore    float64   `json:"trust_score"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	IsSilenced    bool      `json:"is_silenced"`
	IsSuspended   bool      `json:"is_suspended"`
}

// FederationInstancesResponse represents GET /api/v1/admin/federation/instances.
type FederationInstancesResponse struct {
	Instances  []InstanceInfoResponse `json:"instances"`
	NextCursor *string                `json:"next_cursor,omitempty"`
}

// FederationInstanceResponse represents GET /api/v1/admin/federation/instance/{domain}.
type FederationInstanceResponse struct {
	Instance InstanceInfoResponse `json:"instance"`
	Details  map[string]any       `json:"details"`
}

// FederationStatisticsTimeRange represents the time range returned by federation statistics.
type FederationStatisticsTimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// FederationStatisticsResponse represents GET /api/v1/admin/federation/statistics.
type FederationStatisticsResponse struct {
	ActiveInstances int64                         `json:"active_instances"`
	TotalMessages   int64                         `json:"total_messages"`
	TotalUsers      int64                         `json:"total_users"`
	TimeRange       FederationStatisticsTimeRange `json:"time_range"`
}

// EmailDomainBlockResponse represents an email domain block in API responses.
type EmailDomainBlockResponse struct {
	ID        string    `json:"id"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

// EmailDomainBlockRequest represents a request to block an email domain.
type EmailDomainBlockRequest struct {
	Domain string `json:"domain"`
}

// EmailDomainBlocksResponse represents GET /api/v1/admin/email_domain_blocks.
type EmailDomainBlocksResponse struct {
	Blocks     []EmailDomainBlockResponse `json:"blocks"`
	NextCursor *string                    `json:"next_cursor,omitempty"`
}
