package models

import (
	"fmt"
	"math"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// FederationInstance represents federation instance information in DynamoDB
type FederationInstance struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk,attr:PK"`
	SK     string `theorydb:"sk,attr:SK"`
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"`

	Domain        string    `theorydb:"attr:domain" json:"domain"`
	Software      string    `theorydb:"attr:software" json:"software"`            // mastodon, pleroma, etc.
	Version       string    `theorydb:"attr:version" json:"version"`              // Software version
	FirstSeen     time.Time `theorydb:"attr:firstSeen" json:"first_seen"`         // When we first saw this instance
	LastSeen      time.Time `theorydb:"attr:lastSeen" json:"last_seen"`           // Last activity from this instance
	PublicKey     string    `theorydb:"attr:publicKey" json:"public_key"`         // Instance actor public key
	SharedInbox   string    `theorydb:"attr:sharedInbox" json:"shared_inbox"`     // Shared inbox endpoint
	TrustScore    float64   `theorydb:"attr:trustScore" json:"trust_score"`       // Calculated trust score
	ActiveUsers   int       `theorydb:"attr:activeUsers" json:"active_users"`     // Number of active users
	TotalMessages int64     `theorydb:"attr:totalMessages" json:"total_messages"` // Total messages received
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
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk,attr:PK"`
	SK     string `theorydb:"sk,attr:SK"`
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"`

	ID           string    `theorydb:"attr:id" json:"id"`
	Domain       string    `theorydb:"attr:domain" json:"domain"`
	Type         string    `theorydb:"attr:type" json:"type"`                  // ingress/egress
	ActivityType string    `theorydb:"attr:activityType" json:"activity_type"` // Create/Update/Delete/Follow/etc
	ByteSize     int64     `theorydb:"attr:byteSize" json:"byte_size"`
	Success      bool      `theorydb:"attr:success" json:"success"`
	ResponseTime int64     `theorydb:"attr:responseTime" json:"response_time"` // milliseconds
	ErrorMessage string    `theorydb:"attr:errorMessage" json:"error_message,omitempty"`
	Timestamp    time.Time `theorydb:"attr:timestamp" json:"timestamp"`
	TTL          int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	Domain           string    `theorydb:"attr:domain" json:"domain"`
	Period           string    `theorydb:"attr:period" json:"period"` // daily/monthly
	IngressBytes     int64     `theorydb:"attr:ingressBytes" json:"ingress_bytes"`
	EgressBytes      int64     `theorydb:"attr:egressBytes" json:"egress_bytes"`
	RequestCount     int64     `theorydb:"attr:requestCount" json:"request_count"`
	ErrorCount       int64     `theorydb:"attr:errorCount" json:"error_count"`
	ErrorRate        float64   `theorydb:"attr:errorRate" json:"error_rate"`
	AvgResponseTime  float64   `theorydb:"attr:avgResponseTime" json:"avg_response_time"`
	EstimatedCostUSD float64   `theorydb:"attr:estimatedCostUSD" json:"estimated_cost_usd"`
	LastUpdated      time.Time `theorydb:"attr:lastUpdated" json:"last_updated"`
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

// FederationTimeseriesWindow stores aggregated federation stream metrics for a time window.
type FederationTimeseriesWindow struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	Type                string `theorydb:"attr:type" json:"type"`
	Window              string `theorydb:"attr:window" json:"window"`
	FollowCount         int    `theorydb:"attr:followCount" json:"follow_count"`
	LikeCount           int    `theorydb:"attr:likeCount" json:"like_count"`
	AnnounceCount       int    `theorydb:"attr:announceCount" json:"announce_count"`
	ActivityCount       int    `theorydb:"attr:activityCount" json:"activity_count"`
	UniqueActorCount    int    `theorydb:"attr:uniqueActorCount" json:"unique_actor_count"`
	UniqueInstanceCount int    `theorydb:"attr:uniqueInstanceCount" json:"unique_instance_count"`
	CreatedAt           string `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt           string `theorydb:"attr:updatedAt" json:"updated_at"`
	TTL                 int64  `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing FederationTimeseriesWindow.
func (FederationTimeseriesWindow) TableName() string {
	return MainTableName
}

// NewFederationTimeseriesWindow builds the key envelope for a federation metrics window.
func NewFederationTimeseriesWindow(window, now time.Time) *FederationTimeseriesWindow {
	windowStr := window.Format(time.RFC3339)
	nowStr := now.Format(time.RFC3339)
	return &FederationTimeseriesWindow{
		PK:        "TIMESERIES#FEDERATION",
		SK:        fmt.Sprintf("WINDOW#%s", windowStr),
		Type:      "FederationTimeseries",
		Window:    windowStr,
		CreatedAt: nowStr,
		UpdatedAt: nowStr,
		TTL:       now.Add(90 * 24 * time.Hour).Unix(),
	}
}

// InstanceTimeseriesWindow stores per-instance stream observation metadata for a time window.
type InstanceTimeseriesWindow struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	Type      string `theorydb:"attr:type" json:"type"`
	Instance  string `theorydb:"attr:instance" json:"instance"`
	Window    string `theorydb:"attr:window" json:"window"`
	CreatedAt string `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt string `theorydb:"attr:updatedAt" json:"updated_at"`
	TTL       int64  `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing InstanceTimeseriesWindow.
func (InstanceTimeseriesWindow) TableName() string {
	return MainTableName
}

// NewInstanceTimeseriesWindow builds the key envelope for an instance metrics window.
func NewInstanceTimeseriesWindow(instance string, window, now time.Time) *InstanceTimeseriesWindow {
	windowStr := window.Format(time.RFC3339)
	nowStr := now.Format(time.RFC3339)
	return &InstanceTimeseriesWindow{
		PK:        fmt.Sprintf("TIMESERIES#INSTANCE#%s", instance),
		SK:        fmt.Sprintf("WINDOW#%s", windowStr),
		Type:      "InstanceTimeseries",
		Instance:  instance,
		Window:    windowStr,
		CreatedAt: nowStr,
		UpdatedAt: nowStr,
		TTL:       now.Add(30 * 24 * time.Hour).Unix(),
	}
}

// FederationNode represents a node in the federation graph
type FederationNode struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk,attr:PK"`
	SK     string `theorydb:"sk,attr:SK"`
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"`
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK,omitempty"`
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK,omitempty"`

	Domain            string         `theorydb:"attr:domain" json:"domain"`
	DisplayName       string         `theorydb:"attr:displayName" json:"display_name"`
	Description       string         `theorydb:"attr:description" json:"description,omitempty"`
	Software          string         `theorydb:"attr:software" json:"software"`
	Version           string         `theorydb:"attr:version" json:"version"`
	UserCount         int64          `theorydb:"attr:userCount" json:"user_count"`
	StatusCount       int64          `theorydb:"attr:statusCount" json:"status_count"`
	ActiveUsers       int64          `theorydb:"attr:activeUsers" json:"active_users"`
	FirstSeen         time.Time      `theorydb:"attr:firstSeen" json:"first_seen"`
	LastSeen          time.Time      `theorydb:"attr:lastSeen" json:"last_seen"`
	Health            string         `theorydb:"attr:health" json:"health"` // healthy/warning/critical/unknown
	ErrorRate         float64        `theorydb:"attr:errorRate" json:"error_rate"`
	ResponseTime      float64        `theorydb:"attr:responseTime" json:"response_time"`
	ConnectionType    string         `theorydb:"attr:connectionType" json:"connection_type"` // direct/relay/blocked
	TotalConnections  int64          `theorydb:"attr:totalConnections" json:"total_connections,omitempty"`
	ActiveConnections int64          `theorydb:"attr:activeConnections" json:"active_connections,omitempty"`
	ActivityVolume    int64          `theorydb:"attr:activityVolume" json:"activity_volume,omitempty"`
	Metadata          map[string]any `theorydb:"attr:metadata" json:"metadata,omitempty"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk,attr:PK"`
	SK     string `theorydb:"sk,attr:SK"`
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty"`
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK,omitempty"`
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK,omitempty"`
	GSI8PK string `theorydb:"index:gsi8,pk,attr:gsi8PK,omitempty"`
	GSI8SK string `theorydb:"index:gsi8,sk,attr:gsi8SK,omitempty"`

	SourceDomain   string    `theorydb:"attr:sourceDomain" json:"source_domain"`
	TargetDomain   string    `theorydb:"attr:targetDomain" json:"target_domain"`
	ConnectionType string    `theorydb:"attr:connectionType" json:"connection_type"` // follows/mentions/boosts/replies
	VolumeIn       int64     `theorydb:"attr:volumeIn" json:"volume_in"`
	VolumeOut      int64     `theorydb:"attr:volumeOut" json:"volume_out"`
	Strength       float64   `theorydb:"attr:strength" json:"strength"` // 0.0-1.0 based on activity volume
	LastActivity   time.Time `theorydb:"attr:lastActivity" json:"last_activity"`
	SharedUsers    int64     `theorydb:"attr:sharedUsers" json:"shared_users"`
	ErrorCount     int64     `theorydb:"attr:errorCount" json:"error_count"`
	SuccessRate    float64   `theorydb:"attr:successRate" json:"success_rate"`
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

	// GSI3 for the global all-edges listing (no index scans) — additive
	// partition on the pre-existing gsi3 slot. Maintained on every write via
	// UpdateKeys (single write entry: UpdateFederationEdge). Legacy edge rows
	// written before this key shape existed are not listed by the global view
	// until the edge is next written.
	f.GSI3PK = "FED_EDGES#ALL"
	f.GSI3SK = fmt.Sprintf("SRC#%s#TGT#%s", f.SourceDomain, f.TargetDomain)

	// GSI8 for global strongest-by-type queries (no index scans).
	// Sort order: strength desc, last activity desc (via Query OrderBy DESC).
	strengthScaled := int64(math.Round(f.Strength * 1000000))
	if strengthScaled < 0 {
		strengthScaled = 0
	}
	if strengthScaled > 1000000 {
		strengthScaled = 1000000
	}

	f.GSI8PK = fmt.Sprintf("FED_EDGES#TYPE#%s", f.ConnectionType)
	f.GSI8SK = fmt.Sprintf(
		"STRENGTH#%07d#LAST#%013d#SRC#%s#TGT#%s",
		strengthScaled,
		f.LastActivity.Unix(),
		f.SourceDomain,
		f.TargetDomain,
	)
}

// InstanceMetadata contains detailed metadata about a federated instance
type InstanceMetadata struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	Domain          string    `theorydb:"attr:domain" json:"domain"`
	DisplayName     string    `theorydb:"attr:displayName" json:"display_name,omitempty"`
	Description     string    `theorydb:"attr:description" json:"description,omitempty"`
	Software        string    `theorydb:"attr:software" json:"software,omitempty"`
	Version         string    `theorydb:"attr:version" json:"version,omitempty"`
	UserCount       int64     `theorydb:"attr:userCount" json:"user_count,omitempty"`
	StatusCount     int64     `theorydb:"attr:statusCount" json:"status_count,omitempty"`
	NodeInfo        string    `theorydb:"attr:nodeInfo" json:"nodeinfo"`          // JSON string of nodeinfo response
	InstanceInfo    string    `theorydb:"attr:instanceInfo" json:"instance_info"` // JSON string of instance API response
	AdminContact    string    `theorydb:"attr:adminContact" json:"admin_contact,omitempty"`
	Rules           []string  `theorydb:"attr:rules" json:"rules,omitempty"`
	Languages       []string  `theorydb:"attr:languages" json:"languages,omitempty"`
	Categories      []string  `theorydb:"attr:categories" json:"categories,omitempty"`
	FederationNotes string    `theorydb:"attr:federationNotes" json:"federation_notes,omitempty"`
	LastUpdated     time.Time `theorydb:"attr:lastUpdated" json:"last_updated"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk,attr:PK"`
	SK     string `theorydb:"sk,attr:SK"`
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"`

	ClusterID   string    `theorydb:"attr:clusterID" json:"cluster_id"`
	Name        string    `theorydb:"attr:name" json:"name"`
	Instances   []string  `theorydb:"attr:instances" json:"instances"`
	CenterNode  string    `theorydb:"attr:centerNode" json:"center_node"` // Most connected instance
	Cohesion    float64   `theorydb:"attr:cohesion" json:"cohesion"`      // How tightly connected (0.0-1.0)
	Size        int       `theorydb:"attr:size" json:"size"`
	Description string    `theorydb:"attr:description" json:"description,omitempty"`
	UpdatedAt   time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk,attr:PK"`
	SK     string `theorydb:"sk,attr:SK"`
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty"`

	Domain         string    `theorydb:"attr:domain" json:"domain"`
	TargetDomain   string    `theorydb:"attr:targetDomain" json:"target_domain"`
	Direction      string    `theorydb:"attr:direction" json:"direction"` // inbound/outbound
	ConnectionType string    `theorydb:"attr:connectionType" json:"connection_type"`
	VolumeIn       int64     `theorydb:"attr:volumeIn" json:"volume_in"`
	VolumeOut      int64     `theorydb:"attr:volumeOut" json:"volume_out"`
	LastActivity   time.Time `theorydb:"attr:lastActivity" json:"last_activity"`
	Success        bool      `theorydb:"attr:success" json:"success"`
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
