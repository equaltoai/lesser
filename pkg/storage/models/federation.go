package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// FederationInstance represents federation instance information in DynamoDB
type FederationInstance struct {
	PK            string    `dynamorm:"pk"`
	SK            string    `dynamorm:"sk"`
	GSI1PK        string    `dynamorm:"index:gsi1,pk"`
	GSI1SK        string    `dynamorm:"index:gsi1,sk"`
	Domain        string    `json:"domain"`
	Software      string    `json:"software"`       // mastodon, pleroma, etc.
	Version       string    `json:"version"`        // Software version
	FirstSeen     time.Time `json:"first_seen"`     // When we first saw this instance
	LastSeen      time.Time `json:"last_seen"`      // Last activity from this instance
	PublicKey     string    `json:"public_key"`     // Instance actor public key
	SharedInbox   string    `json:"shared_inbox"`   // Shared inbox endpoint
	TrustScore    float64   `json:"trust_score"`    // Calculated trust score
	ActiveUsers   int       `json:"active_users"`   // Number of active users
	TotalMessages int64     `json:"total_messages"` // Total messages received
}

// TableName returns the DynamoDB table backing FederationInstance.
func (FederationInstance) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for federation instance
func (f *FederationInstance) UpdateKeys() {
	// Primary key pattern: INSTANCE#domain
	f.PK = fmt.Sprintf("INSTANCE#%s", f.Domain)
	f.SK = fmt.Sprintf("INSTANCE#%s", f.Domain)

	// GSI1 for active federation tracking
	f.GSI1PK = "FEDERATION_ACTIVE"
	f.GSI1SK = f.LastSeen.Format(time.RFC3339)
}

// FederationCostActivity represents a federation activity for cost tracking
type FederationCostActivity struct {
	PK           string    `dynamorm:"pk"`
	SK           string    `dynamorm:"sk"`
	GSI1PK       string    `dynamorm:"index:gsi1,pk"`
	GSI1SK       string    `dynamorm:"index:gsi1,sk"`
	ID           string    `json:"id"`
	Domain       string    `json:"domain"`
	Type         string    `json:"type"`          // ingress/egress
	ActivityType string    `json:"activity_type"` // Create/Update/Delete/Follow/etc
	ByteSize     int64     `json:"byte_size"`
	Success      bool      `json:"success"`
	ResponseTime int64     `json:"response_time"` // milliseconds
	ErrorMessage string    `json:"error_message,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	TTL          int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table backing FederationCostActivity.
func (FederationCostActivity) TableName() string {
	return MainTableName
}

// GetPK returns the partition key for BaseRepository compatibility
func (f *FederationCostActivity) GetPK() string {
	return f.PK
}

// GetSK returns the sort key for BaseRepository compatibility
func (f *FederationCostActivity) GetSK() string {
	return f.SK
}

// UpdateKeys updates the GSI keys for federation cost activity
func (f *FederationCostActivity) UpdateKeys() error {
	now := f.Timestamp
	if now.IsZero() {
		now = time.Now()
	}

	// Monthly partition for time-series queries
	f.PK = fmt.Sprintf("FEDERATION#%s#%s", f.Domain, now.Format(common.MonthFormat))
	f.SK = fmt.Sprintf("ACTIVITY#%s#%s", now.Format(common.CompactTimeFormat), f.ID)

	// GSI1 for daily queries
	f.GSI1PK = fmt.Sprintf("FEDERATION_DAILY#%s", now.Format(common.DateFormat))
	f.GSI1SK = fmt.Sprintf("DOMAIN#%s#%s", f.Domain, f.ID)

	// Set TTL to 90 days
	f.TTL = now.Add(90 * 24 * time.Hour).Unix()

	return nil
}

// FederationCost represents aggregated federation cost data
type FederationCost struct {
	PK               string    `dynamorm:"pk"`
	SK               string    `dynamorm:"sk"`
	Domain           string    `json:"domain"`
	Period           string    `json:"period"` // daily/monthly
	IngressBytes     int64     `json:"ingress_bytes"`
	EgressBytes      int64     `json:"egress_bytes"`
	RequestCount     int64     `json:"request_count"`
	ErrorCount       int64     `json:"error_count"`
	ErrorRate        float64   `json:"error_rate"`
	AvgResponseTime  float64   `json:"avg_response_time"`
	EstimatedCostUSD float64   `json:"estimated_cost_usd"`
	LastUpdated      time.Time `json:"last_updated"`
}

// TableName returns the DynamoDB table backing FederationCost.
func (FederationCost) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for federation cost
func (f *FederationCost) UpdateKeys() {
	// Cost aggregation keys
	f.PK = fmt.Sprintf("FEDERATION_COSTS#%s", time.Now().Format(common.MonthFormat))
	f.SK = fmt.Sprintf("DOMAIN#%s", f.Domain)
}

// FederationHealthReport represents instance health metrics (computed, not stored)
type FederationHealthReport struct {
	Domain          string    `json:"domain"`
	Status          string    `json:"status"` // healthy/warning/critical
	ResponseTime    float64   `json:"response_time"`
	ErrorRate       float64   `json:"error_rate"`
	FederationDelay float64   `json:"federation_delay"`
	QueueDepth      int       `json:"queue_depth"`
	Issues          []string  `json:"issues"`
	Recommendations []string  `json:"recommendations"`
	LastChecked     time.Time `json:"last_checked"`
}

// TableName returns the DynamoDB table backing FederationHealthReport.
func (FederationHealthReport) TableName() string {
	return MainTableName
}

// FederationNode represents a node in the federation graph
type FederationNode struct {
	PK                string         `dynamorm:"pk"`
	SK                string         `dynamorm:"sk"`
	GSI1PK            string         `dynamorm:"index:gsi1,pk"`
	GSI1SK            string         `dynamorm:"index:gsi1,sk"`
	GSI3PK            string         `dynamorm:"index:gsi3,pk"`
	GSI3SK            string         `dynamorm:"index:gsi3,sk"`
	Domain            string         `json:"domain"`
	DisplayName       string         `json:"display_name"`
	Description       string         `json:"description,omitempty"`
	Software          string         `json:"software"`
	Version           string         `json:"version"`
	UserCount         int64          `json:"user_count"`
	StatusCount       int64          `json:"status_count"`
	ActiveUsers       int64          `json:"active_users"`
	FirstSeen         time.Time      `json:"first_seen"`
	LastSeen          time.Time      `json:"last_seen"`
	Health            string         `json:"health"` // healthy/warning/critical/unknown
	ErrorRate         float64        `json:"error_rate"`
	ResponseTime      float64        `json:"response_time"`
	ConnectionType    string         `json:"connection_type"` // direct/relay/blocked
	TotalConnections  int64          `json:"total_connections,omitempty"`
	ActiveConnections int64          `json:"active_connections,omitempty"`
	ActivityVolume    int64          `json:"activity_volume,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

// TableName returns the DynamoDB table backing FederationNode.
func (FederationNode) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for federation node
func (f *FederationNode) UpdateKeys() {
	f.PK = fmt.Sprintf("FEDERATION_NODE#%s", f.Domain)
	f.SK = "NODE"

	// GSI1 for active federation tracking
	f.GSI1PK = "FEDERATION_ACTIVE"
	f.GSI1SK = fmt.Sprintf("%d#%s", f.LastSeen.Unix(), f.Domain)

	// GSI3 for domain lookups
	f.GSI3PK = fmt.Sprintf("DOMAIN#%s", f.Domain)
	f.GSI3SK = "FEDERATION_NODE"
}

// FederationEdge represents an edge between federation nodes
type FederationEdge struct {
	PK             string    `dynamorm:"pk"`
	SK             string    `dynamorm:"sk"`
	GSI2PK         string    `dynamorm:"index:gsi2,pk"`
	GSI2SK         string    `dynamorm:"index:gsi2,sk"`
	SourceDomain   string    `json:"source_domain"`
	TargetDomain   string    `json:"target_domain"`
	ConnectionType string    `json:"connection_type"` // follows/mentions/boosts/replies
	VolumeIn       int64     `json:"volume_in"`
	VolumeOut      int64     `json:"volume_out"`
	Strength       float64   `json:"strength"` // 0.0-1.0 based on activity volume
	LastActivity   time.Time `json:"last_activity"`
	SharedUsers    int64     `json:"shared_users"`
	ErrorCount     int64     `json:"error_count"`
	SuccessRate    float64   `json:"success_rate"`
}

// TableName returns the DynamoDB table backing FederationEdge.
func (FederationEdge) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for federation edge
func (f *FederationEdge) UpdateKeys() {
	f.PK = fmt.Sprintf("FEDERATION_EDGE#%s", f.SourceDomain)
	f.SK = f.TargetDomain

	// GSI2 for connection queries
	f.GSI2PK = fmt.Sprintf("INSTANCE#%s#CONNECTIONS#%s", f.SourceDomain, f.ConnectionType)
	f.GSI2SK = fmt.Sprintf("%d#%s", f.LastActivity.Unix(), f.TargetDomain)
}

// InstanceMetadata contains detailed metadata about a federated instance
type InstanceMetadata struct {
	PK              string    `dynamorm:"pk"`
	SK              string    `dynamorm:"sk"`
	Domain          string    `json:"domain"`
	DisplayName     string    `json:"display_name,omitempty"`
	Description     string    `json:"description,omitempty"`
	Software        string    `json:"software,omitempty"`
	Version         string    `json:"version,omitempty"`
	UserCount       int64     `json:"user_count,omitempty"`
	StatusCount     int64     `json:"status_count,omitempty"`
	NodeInfo        string    `json:"nodeinfo"`      // JSON string of nodeinfo response
	InstanceInfo    string    `json:"instance_info"` // JSON string of instance API response
	AdminContact    string    `json:"admin_contact,omitempty"`
	Rules           []string  `json:"rules,omitempty"`
	Languages       []string  `json:"languages,omitempty"`
	Categories      []string  `json:"categories,omitempty"`
	FederationNotes string    `json:"federation_notes,omitempty"`
	LastUpdated     time.Time `json:"last_updated"`
}

// TableName returns the DynamoDB table backing InstanceMetadata.
func (InstanceMetadata) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for instance metadata
func (i *InstanceMetadata) UpdateKeys() {
	i.PK = fmt.Sprintf("INSTANCE_META#%s", i.Domain)
	i.SK = SKMetadata
}

// InstanceCluster represents a group of closely connected instances
type InstanceCluster struct {
	PK          string    `dynamorm:"pk"`
	SK          string    `dynamorm:"sk"`
	GSI1PK      string    `dynamorm:"index:gsi1,pk"`
	GSI1SK      string    `dynamorm:"index:gsi1,sk"`
	ClusterID   string    `json:"cluster_id"`
	Name        string    `json:"name"`
	Instances   []string  `json:"instances"`
	CenterNode  string    `json:"center_node"` // Most connected instance
	Cohesion    float64   `json:"cohesion"`    // How tightly connected (0.0-1.0)
	Size        int       `json:"size"`
	Description string    `json:"description,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName returns the DynamoDB table backing InstanceCluster.
func (InstanceCluster) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for instance cluster
func (i *InstanceCluster) UpdateKeys() {
	i.PK = "FEDERATION_CLUSTER#CLUSTERS"
	i.SK = i.ClusterID

	// GSI1 for size-based queries
	i.GSI1PK = "CLUSTERS_BY_SIZE"
	i.GSI1SK = fmt.Sprintf("%05d#%s", i.Size, i.ClusterID)
}

// InstanceConnection represents a specific connection type between instances
type InstanceConnection struct {
	PK             string    `dynamorm:"pk"`
	SK             string    `dynamorm:"sk"`
	GSI2PK         string    `dynamorm:"index:gsi2,pk"`
	GSI2SK         string    `dynamorm:"index:gsi2,sk"`
	Domain         string    `json:"domain"`
	TargetDomain   string    `json:"target_domain"`
	Direction      string    `json:"direction"` // inbound/outbound
	ConnectionType string    `json:"connection_type"`
	VolumeIn       int64     `json:"volume_in"`
	VolumeOut      int64     `json:"volume_out"`
	LastActivity   time.Time `json:"last_activity"`
	Success        bool      `json:"success"`
}

// TableName returns the DynamoDB table backing InstanceConnection.
func (InstanceConnection) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for instance connection
func (i *InstanceConnection) UpdateKeys() {
	i.PK = fmt.Sprintf(KeyPatternConnection, i.Domain)
	i.SK = fmt.Sprintf("%s#%s", i.ConnectionType, i.TargetDomain)

	// GSI2 for connection queries
	i.GSI2PK = fmt.Sprintf("INSTANCE#%s#CONNECTIONS#%s", i.Domain, i.ConnectionType)
	i.GSI2SK = fmt.Sprintf("%d#%s", i.LastActivity.Unix(), i.TargetDomain)
}
