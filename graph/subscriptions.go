package graph

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aron23/lesser/graph/model"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/moderation"
	"github.com/aron23/lesser/pkg/trust"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"go.uber.org/zap"
)

// SubscriptionManager manages GraphQL subscriptions by connecting to WebSocket streams
type SubscriptionManager struct {
	dynamoClient       *dynamodb.Client
	subscriptionsTable string
	logger             *zap.Logger
	mu                 sync.RWMutex
	// Keep track of active subscriptions separately by type for type safety
	activityChannels   map[string][]chan<- *activitypub.Activity
	objectChannels     map[string][]chan<- *model.Object
	costChannels       map[string][]chan<- *model.CostUpdate
	moderationChannels map[string][]chan<- *moderation.ModerationDecision
	trustChannels      map[string][]chan<- *trust.TrustEdge
	aiChannels         map[string][]chan<- *model.AIAnalysis
	quoteChannels      map[string][]chan<- *model.QuoteActivityUpdate
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager(dynamoClient *dynamodb.Client, subscriptionsTable string, logger *zap.Logger) *SubscriptionManager {
	return &SubscriptionManager{
		dynamoClient:       dynamoClient,
		subscriptionsTable: subscriptionsTable,
		logger:             logger,
		activityChannels:   make(map[string][]chan<- *activitypub.Activity),
		objectChannels:     make(map[string][]chan<- *model.Object),
		costChannels:       make(map[string][]chan<- *model.CostUpdate),
		moderationChannels: make(map[string][]chan<- *moderation.ModerationDecision),
		trustChannels:      make(map[string][]chan<- *trust.TrustEdge),
		aiChannels:         make(map[string][]chan<- *model.AIAnalysis),
		quoteChannels:      make(map[string][]chan<- *model.QuoteActivityUpdate),
	}
}

// SubscribeToActivityStream creates a channel for activity stream updates
func (sm *SubscriptionManager) SubscribeToActivityStream(ctx context.Context, username string, activityTypes []model.ActivityType) (<-chan *activitypub.Activity, error) {
	// Create buffered channel
	ch := make(chan *activitypub.Activity, 100)

	// Determine which stream to subscribe to
	streamName := fmt.Sprintf("user:%s", username)

	// Register channel
	sm.mu.Lock()
	sm.activityChannels[streamName] = append(sm.activityChannels[streamName], ch)
	sm.mu.Unlock()

	// Start goroutine to monitor stream
	go func() {
		defer close(ch)
		defer sm.unregisterActivityChannel(streamName, ch)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// In a real implementation, this would poll DynamoDB streams or
				// connect to the WebSocket infrastructure
				// For now, we'll simulate activity updates
				sm.checkForActivityUpdates(ctx, streamName, ch, activityTypes)
			}
		}
	}()

	return ch, nil
}

// SubscribeToTimelineUpdates creates a channel for timeline updates
func (sm *SubscriptionManager) SubscribeToTimelineUpdates(ctx context.Context, username string, timelineType model.TimelineType) (<-chan *model.Object, error) {
	// Create buffered channel
	ch := make(chan *model.Object, 100)

	// Determine stream based on timeline type
	var streamName string
	switch timelineType {
	case model.TimelineTypeHome:
		streamName = fmt.Sprintf("user:%s", username)
	case model.TimelineTypePublic:
		streamName = "public"
	case model.TimelineTypeLocal:
		streamName = "public:local"
	case model.TimelineTypeDirect:
		streamName = fmt.Sprintf("direct:%s", username)
	default:
		streamName = fmt.Sprintf("user:%s", username)
	}

	// Start goroutine to monitor stream
	go func() {
		defer close(ch)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Check for new timeline objects
				sm.checkForTimelineUpdates(ctx, streamName, ch)
			}
		}
	}()

	return ch, nil
}

// SubscribeToCostUpdates creates a channel for cost updates
func (sm *SubscriptionManager) SubscribeToCostUpdates(ctx context.Context, username string, threshold *int) (<-chan *model.CostUpdate, error) {
	// Create buffered channel
	ch := make(chan *model.CostUpdate, 10)

	// Default threshold
	costThreshold := 1000
	if threshold != nil {
		costThreshold = *threshold
	}

	// Start goroutine to monitor costs
	go func() {
		defer close(ch)

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		// Track cumulative costs
		var dailyTotal float64
		var monthlyProjection float64

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Simulate cost update
				// In real implementation, this would track actual DynamoDB/S3 costs
				operationCost := 100 + (time.Now().Unix() % 500)

				if int(operationCost) >= costThreshold {
					dailyTotal += float64(operationCost) / 1000000.0 // Convert micros to dollars
					monthlyProjection = dailyTotal * 30

					update := &model.CostUpdate{
						OperationCost:     int(operationCost),
						DailyTotal:        dailyTotal,
						MonthlyProjection: monthlyProjection,
					}

					select {
					case ch <- update:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return ch, nil
}

// SubscribeToModerationEvents creates a channel for moderation events
func (sm *SubscriptionManager) SubscribeToModerationEvents(ctx context.Context, actorID *string) (<-chan *moderation.ModerationDecision, error) {
	// Create buffered channel
	ch := make(chan *moderation.ModerationDecision, 50)

	// Start goroutine to monitor moderation events
	go func() {
		defer close(ch)

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Check for moderation events
				// In real implementation, this would query the moderation queue
				sm.checkForModerationEvents(ctx, actorID, ch)
			}
		}
	}()

	return ch, nil
}

// SubscribeToTrustUpdates creates a channel for trust score updates
func (sm *SubscriptionManager) SubscribeToTrustUpdates(ctx context.Context, actorID string) (<-chan *trust.TrustEdge, error) {
	// Create buffered channel
	ch := make(chan *trust.TrustEdge, 20)

	// Start goroutine to monitor trust updates
	go func() {
		defer close(ch)

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Check for trust updates
				// In real implementation, this would monitor trust score changes
				sm.checkForTrustUpdates(ctx, actorID, ch)
			}
		}
	}()

	return ch, nil
}

// SubscribeToAIAnalysisUpdates creates a channel for AI analysis updates
func (sm *SubscriptionManager) SubscribeToAIAnalysisUpdates(ctx context.Context, objectID *string) (<-chan *model.AIAnalysis, error) {
	// Create buffered channel
	ch := make(chan *model.AIAnalysis, 20)

	// Start goroutine to monitor AI analysis
	go func() {
		defer close(ch)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Check for AI analysis updates
				// In real implementation, this would monitor AI processing queue
				sm.checkForAIUpdates(ctx, objectID, ch)
			}
		}
	}()

	return ch, nil
}

// SubscribeToHashtagActivity creates a channel for hashtag activity updates
func (sm *SubscriptionManager) SubscribeToHashtagActivity(ctx context.Context, username string, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
	// Create buffered channel
	ch := make(chan *model.HashtagActivityUpdate, 100)

	// Validate hashtags
	if len(hashtags) == 0 {
		close(ch)
		return ch, fmt.Errorf("at least one hashtag must be specified")
	}

	// Start goroutine to monitor hashtag activity
	go func() {
		defer close(ch)

		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Check for hashtag activity updates
				// In real implementation, this would monitor hashtag streams from DynamoDB
				sm.checkForHashtagUpdates(ctx, username, hashtags, ch)
			}
		}
	}()

	return ch, nil
}

// SubscribeToQuoteActivity creates a channel for quote activity updates on a specific note
func (sm *SubscriptionManager) SubscribeToQuoteActivity(ctx context.Context, username string, noteID string, noteObj any) (<-chan *model.QuoteActivityUpdate, error) {
	// Create buffered channel for quote activity updates
	ch := make(chan *model.QuoteActivityUpdate, 50)

	// Validate inputs
	if noteID == "" {
		close(ch)
		return ch, fmt.Errorf("noteID cannot be empty")
	}
	if username == "" {
		close(ch)
		return ch, fmt.Errorf("username cannot be empty")
	}

	// Create stream key for this note
	streamName := fmt.Sprintf("quote:%s", noteID)

	// Register channel with subscription manager
	sm.mu.Lock()
	sm.quoteChannels[streamName] = append(sm.quoteChannels[streamName], ch)
	sm.mu.Unlock()

	// Start goroutine to monitor quote activity
	go func() {
		defer close(ch)
		defer sm.unregisterQuoteChannel(streamName, ch)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		sm.logger.Info("Started quote activity subscription",
			zap.String("username", username),
			zap.String("noteID", noteID),
			zap.String("streamName", streamName))

		for {
			select {
			case <-ctx.Done():
				sm.logger.Info("Quote activity subscription cancelled",
					zap.String("username", username),
					zap.String("noteID", noteID))
				return
			case <-ticker.C:
				// Check for new quote activity on this note
				// In a real implementation, this would:
				// 1. Query DynamoDB for recent quotes of this note
				// 2. Check for quote updates, deletions, edits
				// 3. Send QuoteActivityUpdate objects when changes occur
				sm.checkForQuoteUpdates(ctx, username, noteID, noteObj, ch)
			}
		}
	}()

	return ch, nil
}

// Helper methods

func (sm *SubscriptionManager) unregisterActivityChannel(streamName string, ch chan<- *activitypub.Activity) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	channels := sm.activityChannels[streamName]
	for i, c := range channels {
		if c == ch {
			sm.activityChannels[streamName] = append(channels[:i], channels[i+1:]...)
			break
		}
	}
}

func (sm *SubscriptionManager) unregisterQuoteChannel(streamName string, ch chan<- *model.QuoteActivityUpdate) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	channels := sm.quoteChannels[streamName]
	for i, c := range channels {
		if c == ch {
			sm.quoteChannels[streamName] = append(channels[:i], channels[i+1:]...)
			break
		}
	}
}

func (sm *SubscriptionManager) checkForActivityUpdates(ctx context.Context, streamName string, ch chan<- *activitypub.Activity, filterTypes []model.ActivityType) {
	// In a real implementation, this would:
	// 1. Query recent activities from DynamoDB
	// 2. Filter by activity types if specified
	// 3. Send new activities to the channel

	// For now, we'll create a sample activity
	if time.Now().Unix()%10 == 0 { // Send an update every 10 seconds
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   fmt.Sprintf("https://example.com/activities/%d", time.Now().Unix()),
				Type: activitypub.CreateType,
			},
			Actor: fmt.Sprintf("https://example.com/users/%s", strings.Split(streamName, ":")[1]),
		}

		// Check if we should filter by type
		if len(filterTypes) > 0 {
			typeMatch := false
			for _, t := range filterTypes {
				if string(t) == activity.Type {
					typeMatch = true
					break
				}
			}
			if !typeMatch {
				return
			}
		}

		select {
		case ch <- activity:
		case <-ctx.Done():
			return
		default:
			// Channel full, skip
		}
	}
}

func (sm *SubscriptionManager) checkForTimelineUpdates(ctx context.Context, _ string, ch chan<- *model.Object) {
	// In a real implementation, this would query timeline updates
	// For now, send a sample object occasionally
	if time.Now().Unix()%15 == 0 {
		obj := &model.Object{
			ID:        fmt.Sprintf("https://example.com/objects/%d", time.Now().Unix()),
			Type:      model.ObjectTypeNote,
			Content:   "Sample timeline update",
			CreatedAt: model.Time(time.Now()),
		}

		select {
		case ch <- obj:
		case <-ctx.Done():
			return
		default:
			// Channel full, skip
		}
	}
}

func (sm *SubscriptionManager) checkForModerationEvents(ctx context.Context, actorID *string, ch chan<- *moderation.ModerationDecision) {
	// In a real implementation, this would query moderation events
	// For demonstration, we'll skip implementation
}

func (sm *SubscriptionManager) checkForTrustUpdates(ctx context.Context, actorID string, ch chan<- *trust.TrustEdge) {
	// In a real implementation, this would monitor trust score changes
	// For demonstration, we'll skip implementation
}

func (sm *SubscriptionManager) checkForAIUpdates(ctx context.Context, objectID *string, ch chan<- *model.AIAnalysis) {
	// In a real implementation, this would monitor AI analysis queue
	// For demonstration, we'll skip implementation
}

func (sm *SubscriptionManager) checkForHashtagUpdates(ctx context.Context, username string, hashtags []string, ch chan<- *model.HashtagActivityUpdate) {
	// In a real implementation, this would:
	// 1. Query DynamoDB for recent posts containing the specified hashtags
	// 2. Filter by hashtags and timeframe
	// 3. Send new hashtag activity to the channel

	// For demonstration, we'll create sample hashtag activity updates
	if time.Now().Unix()%8 == 0 { // Send an update every 8 seconds approximately
		// Pick a random hashtag from the list
		selectedHashtag := hashtags[time.Now().Unix()%int64(len(hashtags))]

		update := &model.HashtagActivityUpdate{
			Hashtag: selectedHashtag,
			Post: &model.Object{
				ID:      fmt.Sprintf("https://example.com/objects/%d", time.Now().Unix()),
				Type:    model.ObjectTypeNote,
				Content: fmt.Sprintf("Sample post with #%s hashtag activity", selectedHashtag),
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   fmt.Sprintf("https://example.com/users/user%d", time.Now().Unix()%100),
						Type: "Person",
					},
					PreferredUsername: fmt.Sprintf("user%d", time.Now().Unix()%100),
				},
				CreatedAt: model.Time(time.Now()),
			},
			Author: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   fmt.Sprintf("https://example.com/users/user%d", time.Now().Unix()%100),
					Type: "Person",
				},
				PreferredUsername: fmt.Sprintf("user%d", time.Now().Unix()%100),
			},
			Timestamp: model.Time(time.Now()),
		}

		select {
		case ch <- update:
		case <-ctx.Done():
			return
		default:
			// Channel full, skip
		}
	}
}

func (sm *SubscriptionManager) checkForQuoteUpdates(ctx context.Context, username string, noteID string, noteObj any, ch chan<- *model.QuoteActivityUpdate) {
	// In a real implementation, this would:
	// 1. Query DynamoDB for recent quotes of this specific note
	// 2. Check for quote activity (new quotes, quote updates, quote deletions)
	// 3. Filter quotes that have changed since last check
	// 4. Send QuoteActivityUpdate objects when changes occur

	// For demonstration, we'll simulate quote activity updates periodically
	if time.Now().Unix()%12 == 0 { // Send an update every 12 seconds approximately
		// Simulate different types of quote activity
		activityTypes := []string{"quote_created", "quote_updated", "quote_removed"}
		activityType := activityTypes[time.Now().Unix()%int64(len(activityTypes))]

		var update *model.QuoteActivityUpdate

		switch activityType {
		case "quote_created":
			// Simulate a new quote being created
			update = &model.QuoteActivityUpdate{
				Type: "quote_created",
				Quote: &model.Object{
					ID:      fmt.Sprintf("https://example.com/objects/quote_%d", time.Now().Unix()),
					Type:    model.ObjectTypeNote,
					Content: fmt.Sprintf("This is a quote of note %s", noteID),
					Actor: &activitypub.Actor{
						BaseObject: activitypub.BaseObject{
							ID:   fmt.Sprintf("https://example.com/users/quoter%d", time.Now().Unix()%50),
							Type: "Person",
						},
						PreferredUsername: fmt.Sprintf("quoter%d", time.Now().Unix()%50),
					},
					CreatedAt: model.Time(time.Now()),
				},
				Quoter: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   fmt.Sprintf("https://example.com/users/quoter%d", time.Now().Unix()%50),
						Type: "Person",
					},
					PreferredUsername: fmt.Sprintf("quoter%d", time.Now().Unix()%50),
				},
				Timestamp: model.Time(time.Now()),
			}

		case "quote_updated":
			// Simulate an existing quote being updated
			update = &model.QuoteActivityUpdate{
				Type: "quote_updated",
				Quote: &model.Object{
					ID:      fmt.Sprintf("https://example.com/objects/quote_%d", time.Now().Unix()-300), // Use older ID
					Type:    model.ObjectTypeNote,
					Content: fmt.Sprintf("Updated quote of note %s - edited for clarity", noteID),
					Actor: &activitypub.Actor{
						BaseObject: activitypub.BaseObject{
							ID:   fmt.Sprintf("https://example.com/users/quoter%d", (time.Now().Unix()-300)%50),
							Type: "Person",
						},
						PreferredUsername: fmt.Sprintf("quoter%d", (time.Now().Unix()-300)%50),
					},
					CreatedAt: model.Time(time.Now().Add(-5 * time.Minute)), // Original creation time
				},
				Quoter: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   fmt.Sprintf("https://example.com/users/quoter%d", (time.Now().Unix()-300)%50),
						Type: "Person",
					},
					PreferredUsername: fmt.Sprintf("quoter%d", (time.Now().Unix()-300)%50),
				},
				Timestamp: model.Time(time.Now()),
			}

		case "quote_removed":
			// Simulate a quote being removed/deleted
			update = &model.QuoteActivityUpdate{
				Type: "quote_removed",
				Quote: &model.Object{
					ID:      fmt.Sprintf("https://example.com/objects/quote_%d", time.Now().Unix()-600), // Use older ID
					Type:    model.ObjectTypeNote,
					Content: "", // Empty content for removed quote
					Actor: &activitypub.Actor{
						BaseObject: activitypub.BaseObject{
							ID:   fmt.Sprintf("https://example.com/users/quoter%d", (time.Now().Unix()-600)%50),
							Type: "Person",
						},
						PreferredUsername: fmt.Sprintf("quoter%d", (time.Now().Unix()-600)%50),
					},
					CreatedAt: model.Time(time.Now().Add(-10 * time.Minute)), // Original creation time
				},
				Quoter: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   fmt.Sprintf("https://example.com/users/quoter%d", (time.Now().Unix()-600)%50),
						Type: "Person",
					},
					PreferredUsername: fmt.Sprintf("quoter%d", (time.Now().Unix()-600)%50),
				},
				Timestamp: model.Time(time.Now()),
			}
		}

		// Send the update to the channel
		if update != nil {
			select {
			case ch <- update:
				sm.logger.Debug("Sent quote activity update",
					zap.String("username", username),
					zap.String("noteID", noteID),
					zap.String("activityType", activityType),
					zap.String("quoterID", update.Quoter.ID))
			case <-ctx.Done():
				return
			default:
				// Channel full, skip this update
				sm.logger.Warn("Quote activity channel full, skipping update",
					zap.String("username", username),
					zap.String("noteID", noteID),
					zap.String("activityType", activityType))
			}
		}
	}
}
