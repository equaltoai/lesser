package models

import (
	"fmt"
	"time"
)

// RouteMetricsWindow represents aggregated route metrics for a time window
type RouteMetricsWindow struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys
	PK string `theorydb:"pk,attr:PK" json:"pk"` // METRICS#ROUTE#{routeID}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // WINDOW#{windowStartUnix}

	// Route and time info
	RouteID     string    `theorydb:"attr:routeID" json:"route_id"`
	WindowStart time.Time `theorydb:"attr:windowStart" json:"window_start"`
	WindowSize  int64     `theorydb:"attr:windowSizeMinutes" json:"window_size_minutes"`

	// Aggregated metrics
	MessageCount   int64   `theorydb:"attr:messageCount" json:"message_count"`
	SuccessCount   int64   `theorydb:"attr:successCount" json:"success_count"`
	FailureCount   int64   `theorydb:"attr:failureCount" json:"failure_count"`
	TotalBytes     int64   `theorydb:"attr:totalBytes" json:"total_bytes"`
	TotalCost      float64 `theorydb:"attr:totalCost" json:"total_cost"`
	AvgLatency     int64   `theorydb:"attr:avgLatencyMs" json:"avg_latency_ms"`
	CircuitChanges int64   `theorydb:"attr:circuitChanges" json:"circuit_changes"`

	// Latency histogram (JSON-encoded map[int]int64)
	LatencyHistogram string `theorydb:"attr:latencyHistogram" json:"latency_histogram,omitempty"`

	// Error types (JSON-encoded map[string]int64)
	ErrorTypes string `theorydb:"attr:errorTypes" json:"error_types,omitempty"`

	// TTL for cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing RouteMetricsWindow.
func (RouteMetricsWindow) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys based on the current data
func (r *RouteMetricsWindow) UpdateKeys() error {
	r.PK = fmt.Sprintf("METRICS#ROUTE#%s", r.RouteID)
	r.SK = fmt.Sprintf("WINDOW#%d", r.WindowStart.Unix())

	// Set TTL for 30 days from now
	r.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key
func (r *RouteMetricsWindow) GetPK() string {
	return r.PK
}

// GetSK returns the sort key
func (r *RouteMetricsWindow) GetSK() string {
	return r.SK
}

// GlobalMetricsWindow represents aggregated global metrics for a time window
type GlobalMetricsWindow struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys
	PK string `theorydb:"pk,attr:PK" json:"pk"` // METRICS#GLOBAL#SUMMARY
	SK string `theorydb:"sk,attr:SK" json:"sk"` // WINDOW#{windowStartUnix}

	// GSI keys for time-based queries
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1pk"` // METRICS#GLOBAL
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1sk"` // {windowStartUnix}

	// Time info
	WindowStart time.Time `theorydb:"attr:windowStart" json:"window_start"`
	WindowSize  int64     `theorydb:"attr:windowSizeMinutes" json:"window_size_minutes"`

	// Aggregated metrics
	TotalMessages   int64   `theorydb:"attr:totalMessages" json:"total_messages"`
	TotalBytes      int64   `theorydb:"attr:totalBytes" json:"total_bytes"`
	TotalCost       float64 `theorydb:"attr:totalCost" json:"total_cost"`
	UniqueInstances int64   `theorydb:"attr:uniqueInstances" json:"unique_instances"`
	ActiveRoutes    int64   `theorydb:"attr:activeRoutes" json:"active_routes"`

	// Hourly volume (JSON-encoded [24]int64)
	HourlyVolume string `theorydb:"attr:hourlyVolume" json:"hourly_volume,omitempty"`

	// Top/bottom performers (JSON-encoded)
	TopRoutes    string `theorydb:"attr:topRoutes" json:"top_routes,omitempty"`
	BottomRoutes string `theorydb:"attr:bottomRoutes" json:"bottom_routes,omitempty"`

	// TTL for cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing GlobalMetricsWindow.
func (GlobalMetricsWindow) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys based on the current data
func (g *GlobalMetricsWindow) UpdateKeys() error {
	windowUnix := g.WindowStart.Unix()
	g.PK = "METRICS#GLOBAL#SUMMARY"
	g.SK = fmt.Sprintf("WINDOW#%d", windowUnix)
	g.GSI1PK = "METRICS#GLOBAL"
	g.GSI1SK = fmt.Sprintf("%d", windowUnix)

	// Set TTL for 30 days from now
	g.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key
func (g *GlobalMetricsWindow) GetPK() string {
	return g.PK
}

// GetSK returns the sort key
func (g *GlobalMetricsWindow) GetSK() string {
	return g.SK
}

// InstanceMetricsWindow represents aggregated instance metrics for a time window
type InstanceMetricsWindow struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys
	PK string `theorydb:"pk,attr:PK" json:"pk"` // METRICS#INSTANCE#{instanceID}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // WINDOW#{windowStartUnix}

	// Instance and time info
	InstanceID  string    `theorydb:"attr:instanceID" json:"instance_id"`
	WindowStart time.Time `theorydb:"attr:windowStart" json:"window_start"`
	WindowSize  int64     `theorydb:"attr:windowSizeMinutes" json:"window_size_minutes"`

	// Aggregated metrics
	TotalMessages int64   `theorydb:"attr:totalMessages" json:"total_messages"`
	TotalBytes    int64   `theorydb:"attr:totalBytes" json:"total_bytes"`
	TotalCost     float64 `theorydb:"attr:totalCost" json:"total_cost"`
	HealthChecks  int64   `theorydb:"attr:healthChecks" json:"health_checks"`
	Availability  float64 `theorydb:"attr:availability" json:"availability"`

	// Message type distribution (JSON-encoded map[string]int64)
	MessageTypes string `theorydb:"attr:messageTypes" json:"message_types,omitempty"`

	// TTL for cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing InstanceMetricsWindow.
func (InstanceMetricsWindow) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys based on the current data
func (i *InstanceMetricsWindow) UpdateKeys() error {
	i.PK = fmt.Sprintf("METRICS#INSTANCE#%s", i.InstanceID)
	i.SK = fmt.Sprintf("WINDOW#%d", i.WindowStart.Unix())

	// Set TTL for 30 days from now
	i.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key
func (i *InstanceMetricsWindow) GetPK() string {
	return i.PK
}

// GetSK returns the sort key
func (i *InstanceMetricsWindow) GetSK() string {
	return i.SK
}
