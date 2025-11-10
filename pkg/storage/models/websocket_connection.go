package models

import (
	"fmt"
	"time"
)

// ConnectionState represents the current state of a WebSocket connection
type ConnectionState string

// Connection state constants
const (
	ConnectionStateConnecting ConnectionState = "connecting"
	ConnectionStateConnected  ConnectionState = "connected"
	ConnectionStateIdle       ConnectionState = "idle"
	ConnectionStateClosing    ConnectionState = "closing"
	ConnectionStateClosed     ConnectionState = "closed"
	ConnectionStateError      ConnectionState = "error"
)

// ConnectionMetrics tracks connection performance metrics
type ConnectionMetrics struct {
	MessagesReceived  int64     `json:"messages_received"`
	MessagesSent      int64     `json:"messages_sent"`
	BytesReceived     int64     `json:"bytes_received"`
	BytesSent         int64     `json:"bytes_sent"`
	LastPingTime      time.Time `json:"last_ping_time"`
	LastPongTime      time.Time `json:"last_pong_time"`
	PingLatencyMs     int64     `json:"ping_latency_ms"`
	ErrorCount        int32     `json:"error_count"`
	LastError         string    `json:"last_error,omitempty"`
	ConnectionQuality float64   `json:"connection_quality"` // 0.0-1.0
}

// TableName returns the DynamoDB table backing ConnectionMetrics.
func (ConnectionMetrics) TableName() string {
	return MainTableName
}

// ConnectionInfo holds metadata about the connection
type ConnectionInfo struct {
	ClientIP      string            `json:"client_ip"`
	UserAgent     string            `json:"user_agent"`
	Origin        string            `json:"origin"`
	Protocol      string            `json:"protocol"`
	AuthMethod    string            `json:"auth_method"`
	APIVersion    string            `json:"api_version"`
	CustomHeaders map[string]string `json:"custom_headers,omitempty"`
}

// TableName returns the DynamoDB table backing ConnectionInfo.
func (ConnectionInfo) TableName() string {
	return MainTableName
}

// WebSocketConnection represents a WebSocket connection with complete lifecycle management
type WebSocketConnection struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// DynamoDB Keys - preserving legacy patterns
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // CONN#{connectionID}
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // CONN#{connectionID}

	// GSI keys for querying
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK" json:"gsi1pk"` // USER#{userID}
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK" json:"gsi1sk"` // CONN#{timestamp}

	// GSI2 for state-based queries
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsI2PK" json:"gsi2pk"` // STATE#{state}
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsI2SK" json:"gsi2sk"` // CONN#{connectionID}

	// Business fields
	ConnectionID string    `dynamorm:"attr:connectionID" json:"connection_id"`
	UserID       string    `dynamorm:"attr:userID" json:"user_id"`
	Username     string    `dynamorm:"attr:username" json:"username"`
	Streams      []string  `dynamorm:"attr:streams" json:"streams"` // subscribed streams
	Established  time.Time `dynamorm:"attr:established" json:"established"`
	LastActivity time.Time `dynamorm:"attr:lastActivity" json:"last_activity"`

	// Connection lifecycle fields
	State          ConnectionState `dynamorm:"attr:state" json:"state"`
	StateChangedAt time.Time       `dynamorm:"attr:stateChangedAt" json:"state_changed_at"`
	CloseReason    string          `dynamorm:"attr:closeReason" json:"close_reason,omitempty"`
	CloseCode      int             `dynamorm:"attr:closeCode" json:"close_code,omitempty"`
	RetryCount     int             `dynamorm:"attr:retryCount" json:"retry_count"`
	MaxRetries     int             `dynamorm:"attr:maxRetries" json:"max_retries"`

	// Connection metadata and metrics
	Metrics ConnectionMetrics `dynamorm:"attr:metrics" json:"metrics"`
	Info    ConnectionInfo    `dynamorm:"attr:info" json:"info"`

	// Resource management
	IdleTimeout    time.Duration `dynamorm:"attr:idleTimeout" json:"idle_timeout"`
	MaxMessageSize int64         `dynamorm:"attr:maxMessageSize" json:"max_message_size"`
	RateLimit      int           `dynamorm:"attr:rateLimit" json:"rate_limit"` // messages per minute
	CurrentRate    int           `dynamorm:"attr:currentRate" json:"current_rate"`
	RateLimitReset time.Time     `dynamorm:"attr:rateLimitReset" json:"rate_limit_reset"`

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys sets the GSI keys based on the current values
func (w *WebSocketConnection) UpdateKeys() error {
	// Set primary keys
	w.PK = fmt.Sprintf("CONN#%s", w.ConnectionID)
	w.SK = fmt.Sprintf("CONN#%s", w.ConnectionID)

	// Set GSI1 keys for user-based queries
	if w.UserID != "" {
		w.GSI1PK = fmt.Sprintf(KeyPatternUser, w.UserID)
		w.GSI1SK = fmt.Sprintf("CONN#%s", w.Established.Format(time.RFC3339))
	}

	// Set GSI2 keys for state-based queries
	w.GSI2PK = fmt.Sprintf("STATE#%s", w.State)
	w.GSI2SK = fmt.Sprintf("CONN#%s", w.ConnectionID)

	return nil
}

// GetPK returns the partition key
func (w *WebSocketConnection) GetPK() string {
	return w.PK
}

// GetSK returns the sort key
func (w *WebSocketConnection) GetSK() string {
	return w.SK
}

// TableName returns the DynamoDB table backing WebSocketConnection.
func (w *WebSocketConnection) TableName() string {
	return MainTableName
}

// UpdateState changes the connection state and records the timestamp
func (w *WebSocketConnection) UpdateState(newState ConnectionState) {
	w.State = newState
	w.StateChangedAt = time.Now()
	_ = w.UpdateKeys() // Update GSI2 keys since state changed (ignore error as this is internal)
}

// IsHealthy returns true if the connection is in a healthy state
func (w *WebSocketConnection) IsHealthy() bool {
	return w.State == ConnectionStateConnected && w.Metrics.ErrorCount < 10
}

// IsActive returns true if the connection has been active recently
func (w *WebSocketConnection) IsActive(threshold time.Duration) bool {
	return time.Since(w.LastActivity) < threshold
}

// ShouldRetry returns true if the connection should attempt to reconnect
func (w *WebSocketConnection) ShouldRetry() bool {
	return w.State == ConnectionStateError && w.RetryCount < w.MaxRetries
}

// CalculateConnectionQuality computes a quality score based on metrics
func (w *WebSocketConnection) CalculateConnectionQuality() float64 {
	if w.Metrics.MessagesReceived == 0 {
		return 1.0 // New connection, assume good quality
	}

	// Base quality starts at 1.0
	quality := 1.0

	// Reduce quality based on error rate
	errorRate := float64(w.Metrics.ErrorCount) / float64(w.Metrics.MessagesReceived+w.Metrics.MessagesSent)
	quality -= errorRate * 0.5

	// Reduce quality based on ping latency (>1s is poor)
	if w.Metrics.PingLatencyMs > 1000 {
		latencyPenalty := float64(w.Metrics.PingLatencyMs-1000) / 1000.0 * 0.3
		quality -= latencyPenalty
	}

	// Ensure quality stays within bounds
	if quality < 0.0 {
		quality = 0.0
	} else if quality > 1.0 {
		quality = 1.0
	}

	w.Metrics.ConnectionQuality = quality
	return quality
}

// IncrementError increments the error counter and updates the last error
func (w *WebSocketConnection) IncrementError(errorMsg string) {
	w.Metrics.ErrorCount++
	w.Metrics.LastError = errorMsg
	w.CalculateConnectionQuality()
}

// RecordMessage records statistics for sent/received messages
func (w *WebSocketConnection) RecordMessage(sent bool, bytes int64) {
	if sent {
		w.Metrics.MessagesSent++
		w.Metrics.BytesSent += bytes
	} else {
		w.Metrics.MessagesReceived++
		w.Metrics.BytesReceived += bytes
	}
	w.LastActivity = time.Now()
	w.CalculateConnectionQuality()
}

// RecordPing records a ping timestamp
func (w *WebSocketConnection) RecordPing() {
	w.Metrics.LastPingTime = time.Now()
}

// RecordPong records a pong timestamp and calculates latency
func (w *WebSocketConnection) RecordPong() {
	w.Metrics.LastPongTime = time.Now()
	if !w.Metrics.LastPingTime.IsZero() {
		w.Metrics.PingLatencyMs = w.Metrics.LastPongTime.Sub(w.Metrics.LastPingTime).Milliseconds()
	}
	w.CalculateConnectionQuality()
}

// WebSocketSubscription represents a stream subscription for a WebSocket connection
type WebSocketSubscription struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// DynamoDB Keys - preserving legacy patterns
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // SUB#{stream}
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // CONN#{connectionID}

	// GSI keys for querying
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK" json:"gsi1pk"` // CONN#{connectionID}
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK" json:"gsi1sk"` // STREAM#{stream}

	// Business fields
	ConnectionID string    `dynamorm:"attr:connectionID" json:"connection_id"`
	UserID       string    `dynamorm:"attr:userID" json:"user_id"`
	Stream       string    `dynamorm:"attr:stream" json:"stream"`
	SubscribedAt time.Time `dynamorm:"attr:subscribedAt" json:"subscribed_at"`

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys sets the GSI keys based on the current values
func (w *WebSocketSubscription) UpdateKeys() error {
	// Set primary keys
	w.PK = fmt.Sprintf("SUB#%s", w.Stream)
	w.SK = fmt.Sprintf("CONN#%s", w.ConnectionID)

	// Set GSI1 keys for connection-based queries
	w.GSI1PK = fmt.Sprintf("CONN#%s", w.ConnectionID)
	w.GSI1SK = fmt.Sprintf("STREAM#%s", w.Stream)

	return nil
}

// GetPK returns the partition key
func (w *WebSocketSubscription) GetPK() string {
	return w.PK
}

// GetSK returns the sort key
func (w *WebSocketSubscription) GetSK() string {
	return w.SK
}

// TableName returns the DynamoDB table name for this model
// This is REQUIRED by DynamORM to route operations to the correct table
func (w *WebSocketSubscription) TableName() string {
	return MainTableName
}
