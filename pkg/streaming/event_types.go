// Package streaming provides WebSocket event types and data structures for real-time activity updates.
package streaming

import (
	"encoding/json"
	"fmt"
	"time"
)

// EventType represents the type of internal event
type EventType string

const (
	// EventTypeStatus represents basic status events
	EventTypeStatus EventType = "status"
	// EventTypeStatusUpdate represents status update events
	EventTypeStatusUpdate EventType = "status.update"
	// EventTypeStatusDelete represents status deletion events
	EventTypeStatusDelete EventType = "status.delete"
	// EventTypeStatusFavourite represents status favoriting events
	EventTypeStatusFavourite EventType = "status.favourite"
	// EventTypeStatusReblog represents status reblogging events
	EventTypeStatusReblog EventType = "status.reblog"

	// EventTypeTimelineUpdate represents timeline update events
	EventTypeTimelineUpdate EventType = "timeline.update"
	// EventTypeTimelineRefresh represents timeline refresh events
	EventTypeTimelineRefresh EventType = "timeline.refresh"

	// EventTypeAccountUpdate represents account update events
	EventTypeAccountUpdate EventType = "account.update"
	// EventTypeAccountFollow represents account follow events
	EventTypeAccountFollow EventType = "account.follow"
	// EventTypeAccountUnfollow represents account unfollow events
	EventTypeAccountUnfollow EventType = "account.unfollow"

	// EventTypeNotification represents basic notification events
	EventTypeNotification EventType = "notification"
	// EventTypeNotificationRead represents notification read events
	EventTypeNotificationRead EventType = "notification.read"

	// EventTypeModeration represents basic moderation events
	EventTypeModeration EventType = "moderation"
	// EventTypeModerationFlag represents content flagging events
	EventTypeModerationFlag EventType = "moderation.flag"
	// EventTypeModerationReview represents moderation review events
	EventTypeModerationReview EventType = "moderation.review"

	// EventTypeTrustUpdate represents trust score update events
	EventTypeTrustUpdate EventType = "trust.update"
	// EventTypeReputationUpdate represents reputation score update events
	EventTypeReputationUpdate EventType = "reputation.update"
	// EventTypeVouchUpdate represents vouch update events
	EventTypeVouchUpdate EventType = "vouch.update"

	// EventTypeAIAnalysis represents AI content analysis events
	EventTypeAIAnalysis EventType = "ai.analysis"
	// EventTypeAIClassification represents AI classification events
	EventTypeAIClassification EventType = "ai.classification"
	// EventTypeAIModeration represents AI moderation events
	EventTypeAIModeration EventType = "ai.moderation"

	// EventTypeHashtagTrend represents hashtag trending events
	EventTypeHashtagTrend EventType = "hashtag.trend"
	// EventTypeHashtagUpdate represents hashtag update events
	EventTypeHashtagUpdate EventType = "hashtag.update"

	// EventTypeMediaUpdate represents media update events
	EventTypeMediaUpdate EventType = "media.update"
	// EventTypeMediaProcess represents media processing events
	EventTypeMediaProcess EventType = "media.process"

	// EventTypeCostUpdate represents cost update events
	EventTypeCostUpdate EventType = "cost.update"
	// EventTypeCostAlert represents cost alert events
	EventTypeCostAlert EventType = "cost.alert"

	// EventTypeSystemAlert represents system alert events
	EventTypeSystemAlert EventType = "system.alert"
	// EventTypeHealthCheck represents health check events
	EventTypeHealthCheck EventType = "health.check"

	// EventTypeMetricsUpdate represents real-time metrics update events for GraphQL subscriptions
	EventTypeMetricsUpdate EventType = "metrics.update"
	
	// EventTypeFederationHealthUpdate represents federation health update events
	EventTypeFederationHealthUpdate EventType = "federation.health.update"
	// EventTypeFederationFailure represents federation failure events
	EventTypeFederationFailure EventType = "federation.failure"
	// EventTypeFederationRecovery represents federation recovery events
	EventTypeFederationRecovery EventType = "federation.recovery"
)

// EventAction represents the action that triggered the event
type EventAction string

const (
	// ActionCreate represents content creation actions
	ActionCreate EventAction = "create"
	// ActionUpdate represents content update actions
	ActionUpdate EventAction = "update"
	// ActionDelete represents content deletion actions
	ActionDelete EventAction = "delete"
	// ActionRead represents content read actions
	ActionRead EventAction = "read"
	// ActionFollow represents follow actions
	ActionFollow EventAction = "follow"
	// ActionUnfollow represents unfollow actions
	ActionUnfollow EventAction = "unfollow"
	// ActionFavourite represents favorite actions
	ActionFavourite EventAction = "favourite"
	// ActionUnfavourite represents unfavorite actions
	ActionUnfavourite EventAction = "unfavourite"
	// ActionReblog represents reblog actions
	ActionReblog EventAction = "reblog"
	// ActionUnreblog represents unreblog actions
	ActionUnreblog EventAction = "unreblog"
	// ActionFlag represents content flagging actions
	ActionFlag EventAction = "flag"
	// ActionReview represents content review actions
	ActionReview EventAction = "review"
	// ActionApprove represents content approval actions
	ActionApprove EventAction = "approve"
	// ActionReject represents content rejection actions
	ActionReject EventAction = "reject"
)

// InternalEvent represents an event in the internal event bus
type InternalEvent struct {
	// Event identification
	ID     string      `json:"id"`
	Type   EventType   `json:"type"`
	Action EventAction `json:"action"`

	// Event context
	ActorID  string `json:"actor_id,omitempty"`  // Who triggered the event
	TargetID string `json:"target_id,omitempty"` // What was affected
	UserID   string `json:"user_id,omitempty"`   // User context for the event
	TenantID string `json:"tenant_id,omitempty"` // Multi-tenant support

	// Timing
	Timestamp time.Time `json:"timestamp"`

	// Event payload - the actual data
	Data interface{} `json:"data"`

	// Metadata for filtering and routing
	Metadata map[string]string `json:"metadata,omitempty"`

	// Stream context
	Streams []string `json:"streams,omitempty"` // Which streams this event should go to

	// Priority for event processing
	Priority EventPriority `json:"priority"`
}

// EventPriority represents the priority of an event
type EventPriority int

const (
	// PriorityLow represents low priority events
	PriorityLow EventPriority = 1
	// PriorityNormal represents normal priority events
	PriorityNormal EventPriority = 2
	// PriorityHigh represents high priority events
	PriorityHigh EventPriority = 3
	// PriorityUrgent represents urgent priority events
	PriorityUrgent EventPriority = 4
)

// EventFilter represents criteria for filtering events
type EventFilter struct {
	Types       []EventType       `json:"types,omitempty"`        // Filter by event types
	Actions     []EventAction     `json:"actions,omitempty"`      // Filter by actions
	ActorID     string            `json:"actor_id,omitempty"`     // Filter by actor
	UserID      string            `json:"user_id,omitempty"`      // Filter by user
	TenantID    string            `json:"tenant_id,omitempty"`    // Filter by tenant
	Streams     []string          `json:"streams,omitempty"`      // Filter by streams
	Metadata    map[string]string `json:"metadata,omitempty"`     // Filter by metadata
	MinPriority EventPriority     `json:"min_priority,omitempty"` // Minimum priority
}

// Matches checks if an event matches the filter criteria
func (f *EventFilter) Matches(event *InternalEvent) bool {
	// Check all filter criteria
	if !f.matchesTypes(event.Type) {
		return false
	}
	if !f.matchesActions(event.Action) {
		return false
	}
	if !f.matchesStringField(f.ActorID, event.ActorID) {
		return false
	}
	if !f.matchesStringField(f.UserID, event.UserID) {
		return false
	}
	if !f.matchesStringField(f.TenantID, event.TenantID) {
		return false
	}
	if !f.matchesStreams(event.Streams) {
		return false
	}
	if !f.matchesMetadata(event.Metadata) {
		return false
	}
	if !f.matchesPriority(event.Priority) {
		return false
	}

	return true
}

// matchesTypes checks if event type matches filter
func (f *EventFilter) matchesTypes(eventType EventType) bool {
	if len(f.Types) == 0 {
		return true
	}
	return f.containsType(eventType)
}

// containsType checks if a type is in the filter types
func (f *EventFilter) containsType(eventType EventType) bool {
	for _, t := range f.Types {
		if t == eventType {
			return true
		}
	}
	return false
}

// matchesActions checks if event action matches filter
func (f *EventFilter) matchesActions(eventAction EventAction) bool {
	if len(f.Actions) == 0 {
		return true
	}
	return f.containsAction(eventAction)
}

// containsAction checks if an action is in the filter actions
func (f *EventFilter) containsAction(eventAction EventAction) bool {
	for _, a := range f.Actions {
		if a == eventAction {
			return true
		}
	}
	return false
}

// matchesStringField checks if a string field matches (empty filter means match all)
func (f *EventFilter) matchesStringField(filterValue, eventValue string) bool {
	return filterValue == "" || filterValue == eventValue
}

// matchesStreams checks if event streams match filter
func (f *EventFilter) matchesStreams(eventStreams []string) bool {
	if len(f.Streams) == 0 {
		return true
	}
	return f.hasCommonStream(eventStreams)
}

// hasCommonStream checks if there's any common stream between filter and event
func (f *EventFilter) hasCommonStream(eventStreams []string) bool {
	for _, filterStream := range f.Streams {
		for _, eventStream := range eventStreams {
			if filterStream == eventStream {
				return true
			}
		}
	}
	return false
}

// matchesMetadata checks if event metadata contains all filter metadata
func (f *EventFilter) matchesMetadata(eventMetadata map[string]string) bool {
	for key, value := range f.Metadata {
		if eventValue, exists := eventMetadata[key]; !exists || eventValue != value {
			return false
		}
	}
	return true
}

// matchesPriority checks if event priority meets minimum requirement
func (f *EventFilter) matchesPriority(eventPriority EventPriority) bool {
	return f.MinPriority == 0 || eventPriority >= f.MinPriority
}

// StatusEventPayload represents the payload for status-related events
type StatusEventPayload struct {
	StatusID       string                 `json:"status_id"`
	AuthorID       string                 `json:"author_id"`
	AuthorUsername string                 `json:"author_username"`
	Content        string                 `json:"content,omitempty"`
	Visibility     string                 `json:"visibility,omitempty"`
	InReplyToID    string                 `json:"in_reply_to_id,omitempty"`
	ReblogOfID     string                 `json:"reblog_of_id,omitempty"`
	Sensitive      bool                   `json:"sensitive"`
	Language       string                 `json:"language,omitempty"`
	Hashtags       []string               `json:"hashtags,omitempty"`
	Mentions       []string               `json:"mentions,omitempty"`
	URL            string                 `json:"url,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at,omitempty"`
	ExtraData      map[string]interface{} `json:"extra_data,omitempty"`
}

// AccountEventPayload represents the payload for account-related events
type AccountEventPayload struct {
	AccountID      string    `json:"account_id"`
	Username       string    `json:"username"`
	DisplayName    string    `json:"display_name,omitempty"`
	Avatar         string    `json:"avatar,omitempty"`
	Header         string    `json:"header,omitempty"`
	Bio            string    `json:"bio,omitempty"`
	URL            string    `json:"url,omitempty"`
	FollowersCount int64     `json:"followers_count"`
	FollowingCount int64     `json:"following_count"`
	StatusesCount  int64     `json:"statuses_count"`
	LastStatusAt   time.Time `json:"last_status_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

// NotificationEventPayload represents the payload for notification events
type NotificationEventPayload struct {
	NotificationID string    `json:"notification_id"`
	Type           string    `json:"type"` // follow, mention, favourite, reblog, etc.
	RecipientID    string    `json:"recipient_id"`
	ActorID        string    `json:"actor_id"`
	StatusID       string    `json:"status_id,omitempty"`
	Read           bool      `json:"read"`
	CreatedAt      time.Time `json:"created_at"`
}

// ModerationEventPayload represents the payload for moderation events
type ModerationEventPayload struct {
	ItemID      string                 `json:"item_id"`
	ItemType    string                 `json:"item_type"` // status, account, media
	Action      string                 `json:"action"`    // flag, review, approve, reject
	ModeratorID string                 `json:"moderator_id,omitempty"`
	Reason      string                 `json:"reason,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// TrustEventPayload represents the payload for trust/reputation events
type TrustEventPayload struct {
	SubjectID     string                 `json:"subject_id"`
	SubjectType   string                 `json:"subject_type"` // user, content, domain
	Score         float64                `json:"score"`
	PreviousScore float64                `json:"previous_score,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
	Evidence      map[string]interface{} `json:"evidence,omitempty"`
	UpdatedBy     string                 `json:"updated_by,omitempty"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// AIEventPayload represents the payload for AI analysis events
type AIEventPayload struct {
	AnalysisID   string                 `json:"analysis_id"`
	ContentID    string                 `json:"content_id"`
	ContentType  string                 `json:"content_type"`  // status, media, profile
	AnalysisType string                 `json:"analysis_type"` // sentiment, toxicity, classification
	Results      map[string]interface{} `json:"results"`
	Confidence   float64                `json:"confidence"`
	ModelVersion string                 `json:"model_version,omitempty"`
	ProcessedAt  time.Time              `json:"processed_at"`
}

// CostEventPayload represents the payload for cost tracking events
type CostEventPayload struct {
	Operation string                 `json:"operation"`
	Service   string                 `json:"service"` // dynamodb, lambda, s3, etc.
	CostUSD   float64                `json:"cost_usd"`
	Units     map[string]interface{} `json:"units,omitempty"` // RCU, WCU, requests, etc.
	UserID    string                 `json:"user_id,omitempty"`
	TenantID  string                 `json:"tenant_id,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// HashtagEventPayload represents the payload for hashtag/trend events
type HashtagEventPayload struct {
	Hashtag       string    `json:"hashtag"`
	Count         int64     `json:"count"`
	PreviousCount int64     `json:"previous_count,omitempty"`
	TrendScore    float64   `json:"trend_score,omitempty"`
	Period        string    `json:"period"` // hour, day, week
	UpdatedAt     time.Time `json:"updated_at"`
}

// MediaEventPayload represents the payload for media stream events
type MediaEventPayload struct {
	MediaID     string                 `json:"media_id"`
	URL         string                 `json:"url"`
	MediaType   string                 `json:"media_type"` // image, video, audio
	Status      string                 `json:"status"`     // processing, ready, error
	Size        int64                  `json:"size,omitempty"`
	Duration    int64                  `json:"duration,omitempty"` // for video/audio
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	ProcessedAt time.Time              `json:"processed_at,omitempty"`
}

// MetricsEventPayload represents the payload for metrics update events sent to GraphQL subscriptions
type MetricsEventPayload struct {
	MetricID             string            `json:"metric_id"`
	ServiceName          string            `json:"service_name"`
	MetricType           string            `json:"metric_type"`
	SubscriptionCategory string            `json:"subscription_category"` // moderation, security, performance, etc.
	AggregationLevel     string            `json:"aggregation_level"`     // raw, 5min, hourly, daily
	Timestamp            time.Time         `json:"timestamp"`
	Count                int64             `json:"count,omitempty"`
	Sum                  float64           `json:"sum,omitempty"`
	Min                  float64           `json:"min,omitempty"`
	Max                  float64           `json:"max,omitempty"`
	Average              float64           `json:"average,omitempty"`
	P50                  float64           `json:"p50,omitempty"`
	P95                  float64           `json:"p95,omitempty"`
	P99                  float64           `json:"p99,omitempty"`
	Unit                 string            `json:"unit,omitempty"`
	UserCostMicrocents   int64             `json:"user_cost_microcents,omitempty"`
	TotalCostMicrocents  int64             `json:"total_cost_microcents,omitempty"`
	Dimensions           map[string]string `json:"dimensions,omitempty"`
	UserID               string            `json:"user_id,omitempty"`
	TenantID             string            `json:"tenant_id,omitempty"`
	InstanceDomain       string            `json:"instance_domain,omitempty"`
}

// CreateEvent creates a new internal event with common defaults
func CreateEvent(eventType EventType, action EventAction, data interface{}) *InternalEvent {
	return &InternalEvent{
		ID:        generateEventID(),
		Type:      eventType,
		Action:    action,
		Timestamp: time.Now(),
		Data:      data,
		Priority:  PriorityNormal,
		Metadata:  make(map[string]string),
	}
}

// WithActor sets the actor ID for the event
func (e *InternalEvent) WithActor(actorID string) *InternalEvent {
	e.ActorID = actorID
	return e
}

// WithTarget sets the target ID for the event
func (e *InternalEvent) WithTarget(targetID string) *InternalEvent {
	e.TargetID = targetID
	return e
}

// WithUser sets the user ID for the event
func (e *InternalEvent) WithUser(userID string) *InternalEvent {
	e.UserID = userID
	return e
}

// WithTenant sets the tenant ID for the event
func (e *InternalEvent) WithTenant(tenantID string) *InternalEvent {
	e.TenantID = tenantID
	return e
}

// WithStreams sets the streams for the event
func (e *InternalEvent) WithStreams(streams ...string) *InternalEvent {
	e.Streams = streams
	return e
}

// WithPriority sets the priority for the event
func (e *InternalEvent) WithPriority(priority EventPriority) *InternalEvent {
	e.Priority = priority
	return e
}

// WithMetadata adds metadata to the event
func (e *InternalEvent) WithMetadata(key, value string) *InternalEvent {
	if e.Metadata == nil {
		e.Metadata = make(map[string]string)
	}
	e.Metadata[key] = value
	return e
}

// ToJSON serializes the event to JSON
func (e *InternalEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// FromJSON deserializes an event from JSON
func FromJSON(data []byte) (*InternalEvent, error) {
	var event InternalEvent
	err := json.Unmarshal(data, &event)
	return &event, err
}

// generateEventID generates a unique event ID
func generateEventID() string {
	return fmt.Sprintf("evt_%d_%s", time.Now().UnixNano(), randomString(8))
}

// randomString generates a random string of specified length
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
