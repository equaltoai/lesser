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
	MessagesReceived    int64     `json:"messages_received"`
	MessagesSent        int64     `json:"messages_sent"`
	BytesReceived      int64     `json:"bytes_received"`
	BytesSent          int64     `json:"bytes_sent"`
	LastPingTime       time.Time `json:"last_ping_time"`
	LastPongTime       time.Time `json:"last_pong_time"`
	PingLatencyMs      int64     `json:"ping_latency_ms"`
	ErrorCount         int32     `json:"error_count"`
	LastError          string    `json:"last_error,omitempty"`
	ConnectionQuality  float64   `json:"connection_quality"` // 0.0-1.0
}

// ConnectionInfo holds metadata about the connection
type ConnectionInfo struct {
	ClientIP        string            `json:"client_ip"`
	UserAgent       string            `json:"user_agent"`
	Origin          string            `json:"origin"`
	Protocol        string            `json:"protocol"`
	AuthMethod      string            `json:"auth_method"`
	APIVersion      string            `json:"api_version"`
	CustomHeaders   map[string]string `json:"custom_headers,omitempty"`
}

// WebSocketConnection represents a WebSocket connection with complete lifecycle management
type WebSocketConnection struct {
	// DynamoDB Keys - preserving legacy patterns
	PK string `dynamorm:"pk" json:"pk"` // CONN#{connectionID}
	SK string `dynamorm:"sk" json:"sk"` // CONN#{connectionID}

	// GSI keys for querying
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // USER#{userID}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // CONN#{timestamp}

	// GSI2 for state-based queries
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2pk"` // STATE#{state}
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2sk"` // CONN#{connectionID}

	// Business fields
	ConnectionID string    `json:"connection_id"`
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	Streams      []string  `json:"streams"` // subscribed streams
	Established  time.Time `json:"established"`
	LastActivity time.Time `json:"last_activity"`

	// Connection lifecycle fields
	State           ConnectionState    `json:"state"`
	StateChangedAt  time.Time         `json:"state_changed_at"`
	CloseReason     string            `json:"close_reason,omitempty"`
	CloseCode       int               `json:"close_code,omitempty"`
	RetryCount      int               `json:"retry_count"`
	MaxRetries      int               `json:"max_retries"`
	
	// Connection metadata and metrics
	Metrics      ConnectionMetrics `json:"metrics"`
	Info         ConnectionInfo    `json:"info"`

	// Resource management
	IdleTimeout     time.Duration `json:"idle_timeout"`
	MaxMessageSize  int64         `json:"max_message_size"`
	RateLimit       int           `json:"rate_limit"` // messages per minute
	CurrentRate     int           `json:"current_rate"`
	RateLimitReset  time.Time     `json:"rate_limit_reset"`

	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the GSI keys based on the current values
func (w *WebSocketConnection) UpdateKeys() {
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
}

// UpdateState changes the connection state and records the timestamp
func (w *WebSocketConnection) UpdateState(newState ConnectionState) {
	w.State = newState
	w.StateChangedAt = time.Now()
	w.UpdateKeys() // Update GSI2 keys since state changed
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
	// DynamoDB Keys - preserving legacy patterns
	PK string `dynamorm:"pk" json:"pk"` // SUB#{stream}
	SK string `dynamorm:"sk" json:"sk"` // CONN#{connectionID}

	// GSI keys for querying
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // CONN#{connectionID}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // STREAM#{stream}

	// Business fields
	ConnectionID string    `json:"connection_id"`
	UserID       string    `json:"user_id"`
	Stream       string    `json:"stream"`
	SubscribedAt time.Time `json:"subscribed_at"`

	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the GSI keys based on the current values
func (w *WebSocketSubscription) UpdateKeys() {
	// Set primary keys
	w.PK = fmt.Sprintf("SUB#%s", w.Stream)
	w.SK = fmt.Sprintf("CONN#%s", w.ConnectionID)

	// Set GSI1 keys for connection-based queries
	w.GSI1PK = fmt.Sprintf("CONN#%s", w.ConnectionID)
	w.GSI1SK = fmt.Sprintf("STREAM#%s", w.Stream)
}
