package models

import "github.com/equaltoai/lesser/pkg/storage"

// InstanceV1Response represents the response for GET /api/v1/instance.
type InstanceV1Response struct {
	URI                 string                 `json:"uri"`
	Title               string                 `json:"title"`
	ShortDescription    string                 `json:"short_description"`
	Description         string                 `json:"description"`
	Email               string                 `json:"email"`
	Version             string                 `json:"version"`
	URLs                map[string]any         `json:"urls"`
	Stats               map[string]any         `json:"stats"`
	Thumbnail           string                 `json:"thumbnail"`
	Languages           []string               `json:"languages"`
	Registrations       bool                   `json:"registrations"`
	ApprovalRequired    bool                   `json:"approval_required"`
	InvitesEnabled      bool                   `json:"invites_enabled"`
	ContactAccount      map[string]any         `json:"contact_account,omitempty"`
	Configuration       map[string]any         `json:"configuration"`
	ExtendedDescription string                 `json:"extended_description,omitempty"`
	VAPIDKey            string                 `json:"vapid_key,omitempty"`
	Rules               []storage.InstanceRule `json:"rules"`
}

// InstanceActivityEntry represents a single entry in GET /api/v1/instance/activity.
type InstanceActivityEntry struct {
	Week          string `json:"week"`
	Statuses      string `json:"statuses"`
	Logins        string `json:"logins"`
	Registrations string `json:"registrations"`
}

// InstanceDomainBlock represents a public instance domain block in GET /api/v1/instance/domain_blocks.
type InstanceDomainBlock struct {
	Domain   string `json:"domain"`
	Digest   string `json:"digest"`
	Severity string `json:"severity"`
	Comment  string `json:"comment"`
}

// InstanceV2Response represents the response for GET /api/v2/instance.
type InstanceV2Response struct {
	Domain        string         `json:"domain"`
	Title         string         `json:"title"`
	Version       string         `json:"version"`
	SourceURL     string         `json:"source_url"`
	Description   string         `json:"description"`
	Usage         map[string]any `json:"usage"`
	Thumbnail     map[string]any `json:"thumbnail"`
	Icon          []any          `json:"icon"`
	Languages     []string       `json:"languages"`
	Configuration map[string]any `json:"configuration"`
	Registrations map[string]any `json:"registrations"`
	APIVersions   map[string]any `json:"api_versions"`
	Contact       map[string]any `json:"contact"`
	Rules         []Rule         `json:"rules"`
}
