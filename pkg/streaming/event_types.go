package streaming

import (
	"encoding/json"
	"fmt"
	"time"
)

// EventType represents the type of internal event
type EventType string

const (
	// Status/Timeline Events
	EventTypeStatus         EventType = "status"
	EventTypeStatusUpdate   EventType = "status.update"
	EventTypeStatusDelete   EventType = "status.delete"
	EventTypeStatusFavourite EventType = "status.favourite"
	EventTypeStatusReblog   EventType = "status.reblog"
	
	// Timeline Events
	EventTypeTimelineUpdate EventType = "timeline.update"
	EventTypeTimelineRefresh EventType = "timeline.refresh"
	
	// Account Events
	EventTypeAccountUpdate  EventType = "account.update"
	EventTypeAccountFollow  EventType = "account.follow"
	EventTypeAccountUnfollow EventType = "account.unfollow"
	
	// Notification Events
	EventTypeNotification   EventType = "notification"
	EventTypeNotificationRead EventType = "notification.read"
	
	// Moderation Events
	EventTypeModeration     EventType = "moderation"
	EventTypeModerationFlag EventType = "moderation.flag"
	EventTypeModerationReview EventType = "moderation.review"
	
	// Trust/Reputation Events
	EventTypeTrustUpdate    EventType = "trust.update"
	EventTypeReputationUpdate EventType = "reputation.update"
	EventTypeVouchUpdate    EventType = "vouch.update"
	
	// AI Analysis Events
	EventTypeAIAnalysis     EventType = "ai.analysis"
	EventTypeAIClassification EventType = "ai.classification"
	EventTypeAIModeration   EventType = "ai.moderation"
	
	// Hashtag/Trend Events
	EventTypeHashtagTrend   EventType = "hashtag.trend"
	EventTypeHashtagUpdate  EventType = "hashtag.update"
	
	// Media Stream Events
	EventTypeMediaUpdate    EventType = "media.update"
	EventTypeMediaProcess   EventType = "media.process"
	
	// Cost Tracking Events
	EventTypeCostUpdate     EventType = "cost.update"
	EventTypeCostAlert      EventType = "cost.alert"
	
	// System Events
	EventTypeSystemAlert    EventType = "system.alert"
	EventTypeHealthCheck    EventType = "health.check"
)

// EventAction represents the action that triggered the event
type EventAction string

const (
	ActionCreate EventAction = "create"
	ActionUpdate EventAction = "update"
	ActionDelete EventAction = "delete"
	ActionRead   EventAction = "read"
	ActionFollow EventAction = "follow"
	ActionUnfollow EventAction = "unfollow"
	ActionFavourite EventAction = "favourite"
	ActionUnfavourite EventAction = "unfavourite"
	ActionReblog EventAction = "reblog"
	ActionUnreblog EventAction = "unreblog"
	ActionFlag   EventAction = "flag"
	ActionReview EventAction = "review"
	ActionApprove EventAction = "approve"
	ActionReject EventAction = "reject"
)

// InternalEvent represents an event in the internal event bus
type InternalEvent struct {
	// Event identification
	ID        string    `json:"id"`
	Type      EventType `json:"type"`
	Action    EventAction `json:"action"`
	
	// Event context
	ActorID   string    `json:"actor_id,omitempty"`   // Who triggered the event
	TargetID  string    `json:"target_id,omitempty"`  // What was affected
	UserID    string    `json:"user_id,omitempty"`    // User context for the event
	TenantID  string    `json:"tenant_id,omitempty"`  // Multi-tenant support
	
	// Timing
	Timestamp time.Time `json:"timestamp"`
	
	// Event payload - the actual data
	Data      interface{} `json:"data"`
	
	// Metadata for filtering and routing
	Metadata  map[string]string `json:"metadata,omitempty"`
	
	// Stream context
	Streams   []string `json:"streams,omitempty"`    // Which streams this event should go to
	
	// Priority for event processing
	Priority  EventPriority `json:"priority"`
}

// EventPriority represents the priority of an event
type EventPriority int

const (
	PriorityLow    EventPriority = 1
	PriorityNormal EventPriority = 2
	PriorityHigh   EventPriority = 3
	PriorityUrgent EventPriority = 4
)

// EventFilter represents criteria for filtering events
type EventFilter struct {
	Types    []EventType     `json:"types,omitempty"`    // Filter by event types
	Actions  []EventAction   `json:"actions,omitempty"`  // Filter by actions
	ActorID  string          `json:"actor_id,omitempty"` // Filter by actor
	UserID   string          `json:"user_id,omitempty"`  // Filter by user
	TenantID string          `json:"tenant_id,omitempty"` // Filter by tenant
	Streams  []string        `json:"streams,omitempty"`  // Filter by streams
	Metadata map[string]string `json:"metadata,omitempty"` // Filter by metadata
	MinPriority EventPriority `json:"min_priority,omitempty"` // Minimum priority
}

// Matches checks if an event matches the filter criteria
func (f *EventFilter) Matches(event *InternalEvent) bool {
	// Check event types
	if len(f.Types) > 0 {
		found := false
		for _, t := range f.Types {
			if t == event.Type {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	// Check actions
	if len(f.Actions) > 0 {
		found := false
		for _, a := range f.Actions {
			if a == event.Action {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	// Check actor ID
	if f.ActorID != "" && f.ActorID != event.ActorID {
		return false
	}
	
	// Check user ID
	if f.UserID != "" && f.UserID != event.UserID {
		return false
	}
	
	// Check tenant ID
	if f.TenantID != "" && f.TenantID != event.TenantID {
		return false
	}
	
	// Check streams
	if len(f.Streams) > 0 {
		found := false
		for _, filterStream := range f.Streams {
			for _, eventStream := range event.Streams {
				if filterStream == eventStream {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}
	
	// Check metadata
	if len(f.Metadata) > 0 {
		for key, value := range f.Metadata {
			if eventValue, exists := event.Metadata[key]; !exists || eventValue != value {
				return false
			}
		}
	}
	
	// Check priority
	if f.MinPriority > 0 && event.Priority < f.MinPriority {
		return false
	}
	
	return true
}

// StatusEventPayload represents the payload for status-related events
type StatusEventPayload struct {
	StatusID        string                 `json:"status_id"`
	AuthorID        string                 `json:"author_id"`
	AuthorUsername  string                 `json:"author_username"`
	Content         string                 `json:"content,omitempty"`
	Visibility      string                 `json:"visibility,omitempty"`
	InReplyToID     string                 `json:"in_reply_to_id,omitempty"`
	ReblogOfID      string                 `json:"reblog_of_id,omitempty"`
	Sensitive       bool                   `json:"sensitive"`
	Language        string                 `json:"language,omitempty"`
	Hashtags        []string               `json:"hashtags,omitempty"`
	Mentions        []string               `json:"mentions,omitempty"`
	URL             string                 `json:"url,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at,omitempty"`
	ExtraData       map[string]interface{} `json:"extra_data,omitempty"`
}

// AccountEventPayload represents the payload for account-related events
type AccountEventPayload struct {
	AccountID       string    `json:"account_id"`
	Username        string    `json:"username"`
	DisplayName     string    `json:"display_name,omitempty"`
	Avatar          string    `json:"avatar,omitempty"`
	Header          string    `json:"header,omitempty"`
	Bio             string    `json:"bio,omitempty"`
	URL             string    `json:"url,omitempty"`
	FollowersCount  int64     `json:"followers_count"`
	FollowingCount  int64     `json:"following_count"`
	StatusesCount   int64     `json:"statuses_count"`
	LastStatusAt    time.Time `json:"last_status_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
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
	ItemID       string                 `json:"item_id"`
	ItemType     string                 `json:"item_type"` // status, account, media
	Action       string                 `json:"action"`    // flag, review, approve, reject
	ModeratorID  string                 `json:"moderator_id,omitempty"`
	Reason       string                 `json:"reason,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

// TrustEventPayload represents the payload for trust/reputation events
type TrustEventPayload struct {
	SubjectID    string                 `json:"subject_id"`
	SubjectType  string                 `json:"subject_type"` // user, content, domain
	Score        float64                `json:"score"`
	PreviousScore float64               `json:"previous_score,omitempty"`
	Reason       string                 `json:"reason,omitempty"`
	Evidence     map[string]interface{} `json:"evidence,omitempty"`
	UpdatedBy    string                 `json:"updated_by,omitempty"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// AIEventPayload represents the payload for AI analysis events
type AIEventPayload struct {
	AnalysisID   string                 `json:"analysis_id"`
	ContentID    string                 `json:"content_id"`
	ContentType  string                 `json:"content_type"` // status, media, profile
	AnalysisType string                 `json:"analysis_type"` // sentiment, toxicity, classification
	Results      map[string]interface{} `json:"results"`
	Confidence   float64                `json:"confidence"`
	ModelVersion string                 `json:"model_version,omitempty"`
	ProcessedAt  time.Time              `json:"processed_at"`
}

// CostEventPayload represents the payload for cost tracking events
type CostEventPayload struct {
	Operation    string                 `json:"operation"`
	Service      string                 `json:"service"` // dynamodb, lambda, s3, etc.
	CostUSD      float64                `json:"cost_usd"`
	Units        map[string]interface{} `json:"units,omitempty"` // RCU, WCU, requests, etc.
	UserID       string                 `json:"user_id,omitempty"`
	TenantID     string                 `json:"tenant_id,omitempty"`
	Timestamp    time.Time              `json:"timestamp"`
}

// HashtagEventPayload represents the payload for hashtag/trend events
type HashtagEventPayload struct {
	Hashtag      string    `json:"hashtag"`
	Count        int64     `json:"count"`
	PreviousCount int64    `json:"previous_count,omitempty"`
	TrendScore   float64   `json:"trend_score,omitempty"`
	Period       string    `json:"period"` // hour, day, week
	UpdatedAt    time.Time `json:"updated_at"`
}

// MediaEventPayload represents the payload for media stream events
type MediaEventPayload struct {
	MediaID      string                 `json:"media_id"`
	URL          string                 `json:"url"`
	MediaType    string                 `json:"media_type"` // image, video, audio
	Status       string                 `json:"status"`     // processing, ready, error
	Size         int64                  `json:"size,omitempty"`
	Duration     int64                  `json:"duration,omitempty"` // for video/audio
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	ProcessedAt  time.Time              `json:"processed_at,omitempty"`
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