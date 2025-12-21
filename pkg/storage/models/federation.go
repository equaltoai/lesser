package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// FederationInstance represents federation instance information in DynamoDB
type FederationInstance struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	PK     string `dynamorm:"pk,attr:PK"`
	SK     string `dynamorm:"sk,attr:SK"`
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK"`

	Domain        string    `dynamorm:"attr:domain" json:"domain"`
	Software      string    `dynamorm:"attr:software" json:"software"`            // mastodon, pleroma, etc.
	Version       string    `dynamorm:"attr:version" json:"version"`              // Software version
	FirstSeen     time.Time `dynamorm:"attr:firstSeen" json:"first_seen"`         // When we first saw this instance
	LastSeen      time.Time `dynamorm:"attr:lastSeen" json:"last_seen"`           // Last activity from this instance
	PublicKey     string    `dynamorm:"attr:publicKey" json:"public_key"`         // Instance actor public key
	SharedInbox   string    `dynamorm:"attr:sharedInbox" json:"shared_inbox"`     // Shared inbox endpoint
	TrustScore    float64   `dynamorm:"attr:trustScore" json:"trust_score"`       // Calculated trust score
	ActiveUsers   int       `dynamorm:"attr:activeUsers" json:"active_users"`     // Number of active users
	TotalMessages int64     `dynamorm:"attr:totalMessages" json:"total_messages"` // Total messages received
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
	_ struct{} `dynamorm:"naming:camelCase"`

	PK     string `dynamorm:"pk,attr:PK"`
	SK     string `dynamorm:"sk,attr:SK"`
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK"`

	ID           string    `dynamorm:"attr:id" json:"id"`
	Domain       string    `dynamorm:"attr:domain" json:"domain"`
	Type         string    `dynamorm:"attr:type" json:"type"`                  // ingress/egress
	ActivityType string    `dynamorm:"attr:activityType" json:"activity_type"` // Create/Update/Delete/Follow/etc
	ByteSize     int64     `dynamorm:"attr:byteSize" json:"byte_size"`
	Success      bool      `dynamorm:"attr:success" json:"success"`
	ResponseTime int64     `dynamorm:"attr:responseTime" json:"response_time"` // milliseconds
	ErrorMessage string    `dynamorm:"attr:errorMessage" json:"error_message,omitempty"`
	Timestamp    time.Time `dynamorm:"attr:timestamp" json:"timestamp"`
	TTL          int64     `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
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
	_ struct{} `dynamorm:"naming:camelCase"`

	PK string `dynamorm:"pk,attr:PK"`
	SK string `dynamorm:"sk,attr:SK"`

	Domain           string    `dynamorm:"attr:domain" json:"domain"`
	Period           string    `dynamorm:"attr:period" json:"period"` // daily/monthly
	IngressBytes     int64     `dynamorm:"attr:ingressBytes" json:"ingress_bytes"`
	EgressBytes      int64     `dynamorm:"attr:egressBytes" json:"egress_bytes"`
	RequestCount     int64     `dynamorm:"attr:requestCount" json:"request_count"`
	ErrorCount       int64     `dynamorm:"attr:errorCount" json:"error_count"`
	ErrorRate        float64   `dynamorm:"attr:errorRate" json:"error_rate"`
	AvgResponseTime  float64   `dynamorm:"attr:avgResponseTime" json:"avg_response_time"`
	EstimatedCostUSD float64   `dynamorm:"attr:estimatedCostUSD" json:"estimated_cost_usd"`
	LastUpdated      time.Time `dynamorm:"attr:lastUpdated" json:"last_updated"`
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
	_ struct{} `dynamorm:"naming:camelCase"`

	PK     string `dynamorm:"pk,attr:PK"`
	SK     string `dynamorm:"sk,attr:SK"`
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK"`
	GSI3PK string `dynamorm:"index:gsi3,pk,attr:gsi3PK"`
	GSI3SK string `dynamorm:"index:gsi3,sk,attr:gsi3SK"`

	Domain            string         `dynamorm:"attr:domain" json:"domain"`
	DisplayName       string         `dynamorm:"attr:displayName" json:"display_name"`
	Description       string         `dynamorm:"attr:description" json:"description,omitempty"`
	Software          string         `dynamorm:"attr:software" json:"software"`
	Version           string         `dynamorm:"attr:version" json:"version"`
	UserCount         int64          `dynamorm:"attr:userCount" json:"user_count"`
	StatusCount       int64          `dynamorm:"attr:statusCount" json:"status_count"`
	ActiveUsers       int64          `dynamorm:"attr:activeUsers" json:"active_users"`
	FirstSeen         time.Time      `dynamorm:"attr:firstSeen" json:"first_seen"`
	LastSeen          time.Time      `dynamorm:"attr:lastSeen" json:"last_seen"`
	Health            string         `dynamorm:"attr:health" json:"health"` // healthy/warning/critical/unknown
	ErrorRate         float64        `dynamorm:"attr:errorRate" json:"error_rate"`
	ResponseTime      float64        `dynamorm:"attr:responseTime" json:"response_time"`
	ConnectionType    string         `dynamorm:"attr:connectionType" json:"connection_type"` // direct/relay/blocked
	TotalConnections  int64          `dynamorm:"attr:totalConnections" json:"total_connections,omitempty"`
	ActiveConnections int64          `dynamorm:"attr:activeConnections" json:"active_connections,omitempty"`
	ActivityVolume    int64          `dynamorm:"attr:activityVolume" json:"activity_volume,omitempty"`
	Metadata          map[string]any `dynamorm:"attr:metadata" json:"metadata,omitempty"`
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
	_ struct{} `dynamorm:"naming:camelCase"`

	PK     string `dynamorm:"pk,attr:PK"`
	SK     string `dynamorm:"sk,attr:SK"`
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsi2PK"`
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsi2SK"`

	SourceDomain   string    `dynamorm:"attr:sourceDomain" json:"source_domain"`
	TargetDomain   string    `dynamorm:"attr:targetDomain" json:"target_domain"`
	ConnectionType string    `dynamorm:"attr:connectionType" json:"connection_type"` // follows/mentions/boosts/replies
	VolumeIn       int64     `dynamorm:"attr:volumeIn" json:"volume_in"`
	VolumeOut      int64     `dynamorm:"attr:volumeOut" json:"volume_out"`
	Strength       float64   `dynamorm:"attr:strength" json:"strength"` // 0.0-1.0 based on activity volume
	LastActivity   time.Time `dynamorm:"attr:lastActivity" json:"last_activity"`
	SharedUsers    int64     `dynamorm:"attr:sharedUsers" json:"shared_users"`
	ErrorCount     int64     `dynamorm:"attr:errorCount" json:"error_count"`
	SuccessRate    float64   `dynamorm:"attr:successRate" json:"success_rate"`
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
	_ struct{} `dynamorm:"naming:camelCase"`

	PK string `dynamorm:"pk,attr:PK"`
	SK string `dynamorm:"sk,attr:SK"`

	Domain          string    `dynamorm:"attr:domain" json:"domain"`
	DisplayName     string    `dynamorm:"attr:displayName" json:"display_name,omitempty"`
	Description     string    `dynamorm:"attr:description" json:"description,omitempty"`
	Software        string    `dynamorm:"attr:software" json:"software,omitempty"`
	Version         string    `dynamorm:"attr:version" json:"version,omitempty"`
	UserCount       int64     `dynamorm:"attr:userCount" json:"user_count,omitempty"`
	StatusCount     int64     `dynamorm:"attr:statusCount" json:"status_count,omitempty"`
	NodeInfo        string    `dynamorm:"attr:nodeInfo" json:"nodeinfo"`          // JSON string of nodeinfo response
	InstanceInfo    string    `dynamorm:"attr:instanceInfo" json:"instance_info"` // JSON string of instance API response
	AdminContact    string    `dynamorm:"attr:adminContact" json:"admin_contact,omitempty"`
	Rules           []string  `dynamorm:"attr:rules" json:"rules,omitempty"`
	Languages       []string  `dynamorm:"attr:languages" json:"languages,omitempty"`
	Categories      []string  `dynamorm:"attr:categories" json:"categories,omitempty"`
	FederationNotes string    `dynamorm:"attr:federationNotes" json:"federation_notes,omitempty"`
	LastUpdated     time.Time `dynamorm:"attr:lastUpdated" json:"last_updated"`
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
	_ struct{} `dynamorm:"naming:camelCase"`

	PK     string `dynamorm:"pk,attr:PK"`
	SK     string `dynamorm:"sk,attr:SK"`
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK"`

	ClusterID   string    `dynamorm:"attr:clusterID" json:"cluster_id"`
	Name        string    `dynamorm:"attr:name" json:"name"`
	Instances   []string  `dynamorm:"attr:instances" json:"instances"`
	CenterNode  string    `dynamorm:"attr:centerNode" json:"center_node"` // Most connected instance
	Cohesion    float64   `dynamorm:"attr:cohesion" json:"cohesion"`      // How tightly connected (0.0-1.0)
	Size        int       `dynamorm:"attr:size" json:"size"`
	Description string    `dynamorm:"attr:description" json:"description,omitempty"`
	UpdatedAt   time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
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
	_ struct{} `dynamorm:"naming:camelCase"`

	PK     string `dynamorm:"pk,attr:PK"`
	SK     string `dynamorm:"sk,attr:SK"`
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsi2PK"`
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsi2SK"`

	Domain         string    `dynamorm:"attr:domain" json:"domain"`
	TargetDomain   string    `dynamorm:"attr:targetDomain" json:"target_domain"`
	Direction      string    `dynamorm:"attr:direction" json:"direction"` // inbound/outbound
	ConnectionType string    `dynamorm:"attr:connectionType" json:"connection_type"`
	VolumeIn       int64     `dynamorm:"attr:volumeIn" json:"volume_in"`
	VolumeOut      int64     `dynamorm:"attr:volumeOut" json:"volume_out"`
	LastActivity   time.Time `dynamorm:"attr:lastActivity" json:"last_activity"`
	Success        bool      `dynamorm:"attr:success" json:"success"`
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
