package models

// NodeInfoWellKnown represents the .well-known/nodeinfo response.
type NodeInfoWellKnown struct {
	Links []NodeInfoLink `json:"links"`
}

// NodeInfoLink represents a link in the .well-known/nodeinfo response.
type NodeInfoLink struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}

// NodeInfo represents a NodeInfo 2.0 response.
type NodeInfo struct {
	Version           string           `json:"version"`
	Software          NodeInfoSoftware `json:"software"`
	Protocols         []string         `json:"protocols"`
	Services          NodeInfoServices `json:"services"`
	OpenRegistrations bool             `json:"openRegistrations"`
	Usage             NodeInfoUsage    `json:"usage"`
	Metadata          map[string]any   `json:"metadata"`
}

// NodeInfoSoftware represents software information.
type NodeInfoSoftware struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Repository string `json:"repository,omitempty"`
	Homepage   string `json:"homepage,omitempty"`
}

// NodeInfoServices represents services information.
type NodeInfoServices struct {
	Inbound  []string `json:"inbound"`
	Outbound []string `json:"outbound"`
}

// NodeInfoUsage represents usage statistics.
type NodeInfoUsage struct {
	Users         NodeInfoUsers `json:"users"`
	LocalPosts    int           `json:"localPosts"`
	LocalComments int           `json:"localComments"`
}

// NodeInfoUsers represents user statistics.
type NodeInfoUsers struct {
	Total          int `json:"total"`
	ActiveHalfyear int `json:"activeHalfyear"`
	ActiveMonth    int `json:"activeMonth"`
}
