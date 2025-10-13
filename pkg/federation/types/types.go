// Package types defines shared data structures for ActivityPub federation.
package types //nolint:revive // Standard types package name

import (
	"net/url"
	"time"
)

// MessageType represents the type of federation message
type MessageType string

// Message type constants
const (
	MessageTypeActivity MessageType = "activity"
	MessageTypeFollow   MessageType = "follow"
	MessageTypeAnnounce MessageType = "announce"
	MessageTypeLike     MessageType = "like"
	MessageTypeCreate   MessageType = "create"
	MessageTypeUpdate   MessageType = "update"
	MessageTypeDelete   MessageType = "delete"
	MessageTypeUndo     MessageType = "undo"
	MessageTypeBlock    MessageType = "block"
	MessageTypeFlag     MessageType = "flag"
)

// Instance represents a federated instance
type Instance struct {
	ID             string
	Domain         string
	InboxURL       string
	SharedInboxURL string
	PublicKeyPEM   string

	// Capabilities
	SupportedTypes []MessageType
	MaxMessageSize int64
	RateLimits     RateLimits

	// Status
	Status       InstanceStatus
	LastSeen     time.Time
	RegisteredAt time.Time

	// Performance metrics
	AvgResponseTime time.Duration
	SuccessRate     float64
	ErrorRate       float64

	// Cost tracking
	TierLevel    TierLevel
	MonthlyQuota int64
	CurrentUsage int64
}

// InstanceStatus represents the status of an instance
type InstanceStatus string

// Instance status constants
const (
	InstanceStatusActive      InstanceStatus = "active"
	InstanceStatusDegraded    InstanceStatus = "degraded"
	InstanceStatusUnreachable InstanceStatus = "unreachable"
	InstanceStatusBlocked     InstanceStatus = "blocked"
	InstanceStatusUnknown     InstanceStatus = "unknown"
)

// TierLevel represents the service tier of an instance
type TierLevel string

// Tier level constants
const (
	TierPremium  TierLevel = "premium"
	TierStandard TierLevel = "standard"
	TierLimited  TierLevel = "limited"
	TierBlocked  TierLevel = "blocked"
)

// Route represents a delivery route to an instance
type Route struct {
	ID         string
	InstanceID string
	Domain     string
	Endpoint   *url.URL
	Priority   int // Lower is higher priority

	// Performance
	Latency    time.Duration
	Bandwidth  int64 // bytes per second
	PacketLoss float64

	// Reliability
	SuccessRate      float64
	LastSuccess      time.Time
	LastFailure      time.Time
	ConsecutiveFails int

	// Circuit breaker
	CircuitStatus   CircuitStatus
	CircuitOpenedAt time.Time

	// Cost
	CostPerMessage float64
	CostPerByte    float64
}

// CircuitStatus represents circuit breaker status
type CircuitStatus string

// Circuit status constants
const (
	CircuitClosed   CircuitStatus = "closed"
	CircuitOpen     CircuitStatus = "open"
	CircuitHalfOpen CircuitStatus = "half_open"
)

// HealthStatus represents instance health metrics
type HealthStatus struct {
	Timestamp    time.Time
	Reachable    bool
	ResponseTime time.Duration
	StatusCode   int
	ErrorMessage string

	// Resource usage
	CPUUsage    float64
	MemoryUsage float64
	DiskUsage   float64

	// Federation metrics
	InboxBacklog    int
	ProcessingDelay time.Duration
	ErrorRate       float64
}

// RateLimits defines rate limiting configuration
type RateLimits struct {
	MessagesPerMinute int
	MessagesPerHour   int
	BytesPerMinute    int64
	BytesPerHour      int64
	BurstSize         int
}

// RouteMetrics contains routing performance metrics
type RouteMetrics struct {
	TotalMessages   int64
	SuccessfulCount int64
	FailedCount     int64
	RetryCount      int64

	AvgLatency time.Duration
	P95Latency time.Duration
	P99Latency time.Duration

	TotalBytes int64
	TotalCost  float64

	LastUpdated time.Time
}

// DeliveryOptions configures message delivery
type DeliveryOptions struct {
	Priority         DeliveryPriority
	MaxRetries       int
	RetryBackoff     time.Duration
	Timeout          time.Duration
	RequireSignature bool
	CompressPayload  bool
}

// DeliveryPriority defines message priority levels
type DeliveryPriority int

// Delivery priority constants
const (
	PriorityUrgent DeliveryPriority = 1
	PriorityHigh   DeliveryPriority = 2
	PriorityNormal DeliveryPriority = 3
	PriorityLow    DeliveryPriority = 4
	PriorityBulk   DeliveryPriority = 5
)

// DeliveryResult represents the result of a delivery attempt
type DeliveryResult struct {
	MessageID  string
	InstanceID string
	RouteID    string

	Success      bool
	StatusCode   int
	ErrorMessage string

	Attempts  int
	Duration  time.Duration
	BytesSent int64
	Cost      float64

	Timestamp time.Time
}

// SelectionOptions configures route selection
type SelectionOptions struct {
	PreferReliability bool
	PreferSpeed       bool
	PreferCost        bool
	MaxLatency        time.Duration
	MaxCost           float64
	RequiredTier      TierLevel
}

// FederationMessage represents a message to be delivered
type FederationMessage struct {
	ID     string
	Type   MessageType
	Actor  string
	Object any
	Target []string

	Payload     []byte
	PayloadSize int64
	ContentType string

	CreatedAt time.Time
	ExpiresAt time.Time
}

// QueuedMessage represents a message in the delivery queue
type QueuedMessage struct {
	Message *FederationMessage
	Options DeliveryOptions

	QueuedAt    time.Time
	Attempts    int
	LastAttempt time.Time
	NextRetry   time.Time

	RouteID string
	Status  QueueStatus
}

// QueueStatus represents message queue status
type QueueStatus string

// Queue status constants
const (
	QueueStatusPending    QueueStatus = "pending"
	QueueStatusProcessing QueueStatus = "processing"
	QueueStatusRetrying   QueueStatus = "retrying"
	QueueStatusFailed     QueueStatus = "failed"
	QueueStatusDelivered  QueueStatus = "delivered"
	QueueStatusExpired    QueueStatus = "expired"
)

// QueueMetrics contains queue performance metrics
type QueueMetrics struct {
	Depth           int64
	ProcessingCount int64
	RetryCount      int64
	DLQCount        int64

	EnqueueRate float64
	DequeueRate float64
	SuccessRate float64

	AvgWaitTime    time.Duration
	AvgProcessTime time.Duration
}

// RoutingConfig configures the routing system
type RoutingConfig struct {
	// Health checking
	HealthCheckInterval time.Duration
	HealthCheckTimeout  time.Duration
	UnhealthyThreshold  int
	HealthyThreshold    int

	// Circuit breaker
	CircuitBreakerThreshold int
	CircuitBreakerTimeout   time.Duration
	HalfOpenMaxAttempts     int

	// Routing
	MaxRoutesPerInstance    int
	RouteSelectionAlgorithm string
	EnableLoadBalancing     bool
	EnableCostOptimization  bool

	// Delivery
	DefaultTimeout time.Duration
	MaxRetries     int
	RetryBackoff   time.Duration
	MaxQueueDepth  int64

	// Performance
	EnableCompression  bool
	BatchDeliverySize  int
	ParallelDeliveries int
}

// Errors
var (
	ErrNoHealthyRoutes = &RoutingError{Code: "NO_HEALTHY_ROUTES", Message: "No healthy routes available"}
	ErrCircuitOpen     = &RoutingError{Code: "CIRCUIT_OPEN", Message: "Circuit breaker is open"}
	ErrQuotaExceeded   = &RoutingError{Code: "QUOTA_EXCEEDED", Message: "Instance quota exceeded"}
	ErrInstanceBlocked = &RoutingError{Code: "INSTANCE_BLOCKED", Message: "Instance is blocked"}
	ErrMessageExpired  = &RoutingError{Code: "MESSAGE_EXPIRED", Message: "Message has expired"}
)

// RoutingError represents a routing error
type RoutingError struct {
	Code    string
	Message string
	Details map[string]any
}

func (e *RoutingError) Error() string {
	return e.Message
}
