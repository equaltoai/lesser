package streaming

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Subscriber represents a subscriber to the event bus
type Subscriber struct {
	ID       string
	Filter   *EventFilter
	Channel  chan *InternalEvent
	Quit     chan struct{}
	Active   bool
	Created  time.Time
	LastSeen time.Time
}

// NewSubscriber creates a new subscriber with a buffered channel
func NewSubscriber(id string, filter *EventFilter, bufferSize int) *Subscriber {
	if bufferSize <= 0 {
		bufferSize = DefaultBufferSize
	}
	
	return &Subscriber{
		ID:       id,
		Filter:   filter,
		Channel:  make(chan *InternalEvent, bufferSize),
		Quit:     make(chan struct{}),
		Active:   true,
		Created:  time.Now(),
		LastSeen: time.Now(),
	}
}

// Close closes the subscriber and cleans up resources
func (s *Subscriber) Close() {
	if s.Active {
		s.Active = false
		close(s.Quit)
		close(s.Channel)
	}
}

// EventBus represents the internal event bus for real-time event distribution
type EventBus struct {
	subscribers    map[string]*Subscriber
	subscribersMux sync.RWMutex
	eventChan      chan *InternalEvent
	quit           chan struct{}
	logger         *zap.Logger
	metrics        *EventBusMetrics
	running        bool
	runningMux     sync.RWMutex
}

// EventBusConfig contains configuration for the event bus
type EventBusConfig struct {
	BufferSize         int           // Size of the main event buffer
	SubscriberTimeout  time.Duration // Timeout for slow subscribers
	CleanupInterval    time.Duration // How often to clean up inactive subscribers
	MaxSubscribers     int           // Maximum number of subscribers
	EventTimeout       time.Duration // Timeout for event delivery
	MetricsEnabled     bool          // Whether to collect metrics
}

// DefaultEventBusConfig returns default configuration
func DefaultEventBusConfig() *EventBusConfig {
	return &EventBusConfig{
		BufferSize:         1000,
		SubscriberTimeout:  30 * time.Second,
		CleanupInterval:    5 * time.Minute,
		MaxSubscribers:     1000,
		EventTimeout:       5 * time.Second,
		MetricsEnabled:     true,
	}
}

// EventBusMetrics tracks event bus performance
type EventBusMetrics struct {
	EventsPublished    int64
	EventsDelivered    int64
	EventsDropped      int64
	SubscribersActive  int64
	SubscribersTotal   int64
	DeliveryErrors     int64
	LastEventTime      time.Time
	AverageDeliveryTime time.Duration
	mu                 sync.RWMutex
}

// Constants for event bus
const (
	DefaultBufferSize     = 100
	MaxEventSize         = 1024 * 1024 // 1MB max event size
	CleanupTickerInterval = 1 * time.Minute
)

// NewEventBus creates a new internal event bus
func NewEventBus(config *EventBusConfig, logger *zap.Logger) *EventBus {
	if config == nil {
		config = DefaultEventBusConfig()
	}
	
	if logger == nil {
		logger = zap.NewNop()
	}
	
	var metrics *EventBusMetrics
	if config.MetricsEnabled {
		metrics = &EventBusMetrics{}
	}
	
	return &EventBus{
		subscribers: make(map[string]*Subscriber),
		eventChan:   make(chan *InternalEvent, config.BufferSize),
		quit:        make(chan struct{}),
		logger:      logger,
		metrics:     metrics,
		running:     false,
	}
}

// Start starts the event bus processing loop
func (eb *EventBus) Start(ctx context.Context) error {
	eb.runningMux.Lock()
	defer eb.runningMux.Unlock()
	
	if eb.running {
		return fmt.Errorf("event bus is already running")
	}
	
	eb.running = true
	
	// Start the main event processing loop
	go eb.processEvents(ctx)
	
	// Start the cleanup loop
	go eb.cleanupLoop(ctx)
	
	eb.logger.Info("internal event bus started")
	return nil
}

// Stop stops the event bus
func (eb *EventBus) Stop() error {
	eb.runningMux.Lock()
	defer eb.runningMux.Unlock()
	
	if !eb.running {
		return nil
	}
	
	eb.running = false
	close(eb.quit)
	
	// Close all subscribers
	eb.subscribersMux.Lock()
	for _, subscriber := range eb.subscribers {
		subscriber.Close()
	}
	eb.subscribers = make(map[string]*Subscriber)
	eb.subscribersMux.Unlock()
	
	eb.logger.Info("internal event bus stopped")
	return nil
}

// IsRunning returns whether the event bus is running
func (eb *EventBus) IsRunning() bool {
	eb.runningMux.RLock()
	defer eb.runningMux.RUnlock()
	return eb.running
}

// Publish publishes an event to the event bus
func (eb *EventBus) Publish(event *InternalEvent) error {
	if !eb.IsRunning() {
		return fmt.Errorf("event bus is not running")
	}
	
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}
	
	// Validate event size
	if eventData, err := event.ToJSON(); err != nil {
		return fmt.Errorf("failed to serialize event: %w", err)
	} else if len(eventData) > MaxEventSize {
		return fmt.Errorf("event size %d exceeds maximum %d", len(eventData), MaxEventSize)
	}
	
	// Set timestamp if not already set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	select {
	case eb.eventChan <- event:
		if eb.metrics != nil {
			eb.metrics.mu.Lock()
			eb.metrics.EventsPublished++
			eb.metrics.LastEventTime = time.Now()
			eb.metrics.mu.Unlock()
		}
		
		eb.logger.Debug("event published to internal bus",
			zap.String("event_id", event.ID),
			zap.String("event_type", string(event.Type)),
			zap.String("action", string(event.Action)))
		
		return nil
	case <-eb.quit:
		return fmt.Errorf("event bus is shutting down")
	default:
		// Non-blocking publish - drop event if buffer is full
		if eb.metrics != nil {
			eb.metrics.mu.Lock()
			eb.metrics.EventsDropped++
			eb.metrics.mu.Unlock()
		}
		
		eb.logger.Warn("event dropped - buffer full",
			zap.String("event_id", event.ID),
			zap.String("event_type", string(event.Type)))
		
		return fmt.Errorf("event bus buffer is full")
	}
}

// Subscribe subscribes to events with an optional filter
func (eb *EventBus) Subscribe(subscriberID string, filter *EventFilter, bufferSize int) (*Subscriber, error) {
	if !eb.IsRunning() {
		return nil, fmt.Errorf("event bus is not running")
	}
	
	if subscriberID == "" {
		subscriberID = fmt.Sprintf("sub_%d", time.Now().UnixNano())
	}
	
	eb.subscribersMux.Lock()
	defer eb.subscribersMux.Unlock()
	
	// Check if subscriber already exists
	if _, exists := eb.subscribers[subscriberID]; exists {
		return nil, fmt.Errorf("subscriber %s already exists", subscriberID)
	}
	
	// Check max subscribers limit
	if len(eb.subscribers) >= 1000 { // TODO: make configurable
		return nil, fmt.Errorf("maximum number of subscribers reached")
	}
	
	subscriber := NewSubscriber(subscriberID, filter, bufferSize)
	eb.subscribers[subscriberID] = subscriber
	
	if eb.metrics != nil {
		eb.metrics.mu.Lock()
		eb.metrics.SubscribersActive++
		eb.metrics.SubscribersTotal++
		eb.metrics.mu.Unlock()
	}
	
	eb.logger.Info("new subscriber added to internal event bus",
		zap.String("subscriber_id", subscriberID),
		zap.Int("buffer_size", bufferSize),
		zap.Int("total_subscribers", len(eb.subscribers)))
	
	return subscriber, nil
}

// Unsubscribe removes a subscriber from the event bus
func (eb *EventBus) Unsubscribe(subscriberID string) error {
	eb.subscribersMux.Lock()
	defer eb.subscribersMux.Unlock()
	
	subscriber, exists := eb.subscribers[subscriberID]
	if !exists {
		return fmt.Errorf("subscriber %s not found", subscriberID)
	}
	
	subscriber.Close()
	delete(eb.subscribers, subscriberID)
	
	if eb.metrics != nil {
		eb.metrics.mu.Lock()
		eb.metrics.SubscribersActive--
		eb.metrics.mu.Unlock()
	}
	
	eb.logger.Info("subscriber removed from internal event bus",
		zap.String("subscriber_id", subscriberID),
		zap.Int("total_subscribers", len(eb.subscribers)))
	
	return nil
}

// GetSubscribers returns a list of active subscriber IDs
func (eb *EventBus) GetSubscribers() []string {
	eb.subscribersMux.RLock()
	defer eb.subscribersMux.RUnlock()
	
	subscribers := make([]string, 0, len(eb.subscribers))
	for id := range eb.subscribers {
		subscribers = append(subscribers, id)
	}
	
	return subscribers
}

// GetMetrics returns current event bus metrics
func (eb *EventBus) GetMetrics() *EventBusMetrics {
	if eb.metrics == nil {
		return nil
	}
	
	eb.metrics.mu.RLock()
	defer eb.metrics.mu.RUnlock()
	
	// Return a copy of the metrics
	return &EventBusMetrics{
		EventsPublished:     eb.metrics.EventsPublished,
		EventsDelivered:     eb.metrics.EventsDelivered,
		EventsDropped:       eb.metrics.EventsDropped,
		SubscribersActive:   eb.metrics.SubscribersActive,
		SubscribersTotal:    eb.metrics.SubscribersTotal,
		DeliveryErrors:      eb.metrics.DeliveryErrors,
		LastEventTime:       eb.metrics.LastEventTime,
		AverageDeliveryTime: eb.metrics.AverageDeliveryTime,
	}
}

// processEvents is the main event processing loop
func (eb *EventBus) processEvents(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			// Only log if the event bus is still running (avoid race with tests)
			eb.runningMux.RLock()
			if eb.running {
				eb.logger.Error("event bus processing loop panicked", zap.Any("panic", r))
			}
			eb.runningMux.RUnlock()
		}
	}()
	
	for {
		select {
		case event := <-eb.eventChan:
			eb.distributeEvent(event)
		case <-eb.quit:
			// Only log if the event bus is still marked as running
			eb.runningMux.RLock()
			if eb.running {
				eb.logger.Info("event bus processing loop shutting down")
			}
			eb.runningMux.RUnlock()
			return
		case <-ctx.Done():
			eb.runningMux.RLock()
			if eb.running {
				eb.logger.Info("event bus processing loop cancelled")
			}
			eb.runningMux.RUnlock()
			return
		}
	}
}

// distributeEvent distributes an event to matching subscribers
func (eb *EventBus) distributeEvent(event *InternalEvent) {
	if event == nil {
		return
	}
	
	start := time.Now()
	delivered := 0
	errors := 0
	
	eb.subscribersMux.RLock()
	subscribers := make([]*Subscriber, 0, len(eb.subscribers))
	for _, subscriber := range eb.subscribers {
		if subscriber.Active {
			subscribers = append(subscribers, subscriber)
		}
	}
	eb.subscribersMux.RUnlock()
	
	// Distribute to matching subscribers
	for _, subscriber := range subscribers {
		if subscriber.Filter == nil || subscriber.Filter.Matches(event) {
			if err := eb.deliverToSubscriber(subscriber, event); err != nil {
				eb.logger.Warn("failed to deliver event to subscriber",
					zap.String("subscriber_id", subscriber.ID),
					zap.String("event_id", event.ID),
					zap.Error(err))
				errors++
			} else {
				delivered++
				subscriber.LastSeen = time.Now()
			}
		}
	}
	
	// Update metrics
	if eb.metrics != nil {
		eb.metrics.mu.Lock()
		eb.metrics.EventsDelivered += int64(delivered)
		eb.metrics.DeliveryErrors += int64(errors)
		eb.metrics.AverageDeliveryTime = time.Since(start)
		eb.metrics.mu.Unlock()
	}
	
	eb.logger.Debug("event distributed",
		zap.String("event_id", event.ID),
		zap.String("event_type", string(event.Type)),
		zap.Int("delivered", delivered),
		zap.Int("errors", errors),
		zap.Duration("duration", time.Since(start)))
}

// deliverToSubscriber delivers an event to a specific subscriber
func (eb *EventBus) deliverToSubscriber(subscriber *Subscriber, event *InternalEvent) error {
	if !subscriber.Active {
		return fmt.Errorf("subscriber is not active")
	}
	
	select {
	case subscriber.Channel <- event:
		return nil
	case <-subscriber.Quit:
		return fmt.Errorf("subscriber is shutting down")
	default:
		// Non-blocking delivery - drop event if subscriber buffer is full
		eb.logger.Warn("dropping event for slow subscriber",
			zap.String("subscriber_id", subscriber.ID),
			zap.String("event_id", event.ID))
		return fmt.Errorf("subscriber buffer is full")
	}
}

// cleanupLoop periodically cleans up inactive subscribers
func (eb *EventBus) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(CleanupTickerInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			eb.cleanupInactiveSubscribers()
		case <-eb.quit:
			return
		case <-ctx.Done():
			return
		}
	}
}

// cleanupInactiveSubscribers removes subscribers that haven't been seen recently
func (eb *EventBus) cleanupInactiveSubscribers() {
	eb.subscribersMux.Lock()
	defer eb.subscribersMux.Unlock()
	
	cutoff := time.Now().Add(-5 * time.Minute) // TODO: make configurable
	toRemove := make([]string, 0)
	
	for id, subscriber := range eb.subscribers {
		if !subscriber.Active || subscriber.LastSeen.Before(cutoff) {
			toRemove = append(toRemove, id)
		}
	}
	
	for _, id := range toRemove {
		if subscriber, exists := eb.subscribers[id]; exists {
			subscriber.Close()
			delete(eb.subscribers, id)
			
			if eb.metrics != nil {
				eb.metrics.mu.Lock()
				eb.metrics.SubscribersActive--
				eb.metrics.mu.Unlock()
			}
			
			eb.logger.Info("cleaned up inactive subscriber",
				zap.String("subscriber_id", id))
		}
	}
	
	if len(toRemove) > 0 {
		eb.logger.Info("cleanup completed",
			zap.Int("removed_subscribers", len(toRemove)),
			zap.Int("active_subscribers", len(eb.subscribers)))
	}
}

// Global event bus instance (singleton pattern for stream-router)
var (
	globalEventBus     *EventBus
	globalEventBusMux  sync.Mutex
	globalEventBusOnce sync.Once
)

// GetGlobalEventBus returns the global event bus instance
func GetGlobalEventBus(logger *zap.Logger) *EventBus {
	globalEventBusOnce.Do(func() {
		globalEventBus = NewEventBus(DefaultEventBusConfig(), logger)
	})
	return globalEventBus
}

// InitializeGlobalEventBus initializes the global event bus with custom config
func InitializeGlobalEventBus(config *EventBusConfig, logger *zap.Logger) *EventBus {
	globalEventBusMux.Lock()
	defer globalEventBusMux.Unlock()
	
	if globalEventBus != nil && globalEventBus.IsRunning() {
		globalEventBus.Stop()
	}
	
	globalEventBus = NewEventBus(config, logger)
	return globalEventBus
}

// PublishGlobal publishes an event to the global event bus
func PublishGlobal(event *InternalEvent) error {
	if globalEventBus == nil {
		return fmt.Errorf("global event bus not initialized")
	}
	return globalEventBus.Publish(event)
}

// SubscribeGlobal subscribes to the global event bus
func SubscribeGlobal(subscriberID string, filter *EventFilter, bufferSize int) (*Subscriber, error) {
	if globalEventBus == nil {
		return nil, fmt.Errorf("global event bus not initialized")
	}
	return globalEventBus.Subscribe(subscriberID, filter, bufferSize)
}

// UnsubscribeGlobal unsubscribes from the global event bus
func UnsubscribeGlobal(subscriberID string) error {
	if globalEventBus == nil {
		return fmt.Errorf("global event bus not initialized")
	}
	return globalEventBus.Unsubscribe(subscriberID)
}