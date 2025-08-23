package dlq

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
)

// ProcessorWithDLQ demonstrates how to integrate DLQ support into existing processors
type ProcessorWithDLQ struct {
	service   string
	dlqSender *DLQSender
	logger    *zap.Logger
}

// NewProcessorWithDLQ creates a processor with DLQ integration
func NewProcessorWithDLQ(service string, logger *zap.Logger) *ProcessorWithDLQ {
	return &ProcessorWithDLQ{
		service:   service,
		dlqSender: NewDLQSender(logger),
		logger:    logger,
	}
}

// ProcessSQSBatchWithDLQ processes an SQS batch with automatic DLQ support
func (p *ProcessorWithDLQ) ProcessSQSBatchWithDLQ(ctx context.Context, event events.SQSEvent, messageProcessor func(context.Context, events.SQSMessage) error) error {
	// Initialize DLQ sender
	if err := p.dlqSender.InitializeAWSClients(ctx); err != nil {
		p.logger.Error("failed to initialize DLQ sender", zap.Error(err))
		// Continue processing even if DLQ initialization fails
	}

	// Track failures for DLQ
	var failures []ProcessingFailure
	var failureMutex sync.Mutex

	// Process messages with error tracking
	var errors []error
	var errorMutex sync.Mutex

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // Limit concurrency

	for _, record := range event.Records {
		wg.Add(1)
		sem <- struct{}{}

		go func(record events.SQSMessage) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := messageProcessor(ctx, record); err != nil {
				// Track error for return
				errorMutex.Lock()
				errors = append(errors, err)
				errorMutex.Unlock()

				// Track failure for DLQ
				failureMutex.Lock()
				failures = append(failures, ProcessingFailure{
					OriginalMessage: record,
					Error:           err,
					Timestamp:       time.Now(),
				})
				failureMutex.Unlock()

				p.logger.Error("failed to process message",
					zap.String("service", p.service),
					zap.String("message_id", record.MessageId),
					zap.Error(err),
				)
			}
		}(record)
	}

	wg.Wait()

	// Send failures to DLQ
	if len(failures) > 0 {
		if err := p.dlqSender.SendBatchFailedMessages(ctx, p.service, failures); err != nil {
			p.logger.Error("failed to send failures to DLQ",
				zap.String("service", p.service),
				zap.Int("failure_count", len(failures)),
				zap.Error(err),
			)
		} else {
			p.logger.Info("sent failures to DLQ",
				zap.String("service", p.service),
				zap.Int("failure_count", len(failures)),
			)
		}
	}

	// Return processing errors (DLQ failures don't block processing)
	if len(errors) > 0 {
		return fmt.Errorf("%w: %d of %d messages failed", ErrBatchProcessingFailed, len(errors), len(event.Records))
	}

	return nil
}

// Example usage for notification processor:
/*
func (np *NotificationProcessor) HandleSQSWithDLQ(ctx *lift.Context, event events.SQSEvent) error {
	processor := dlq.NewProcessorWithDLQ("notification-processor", np.logger)

	return processor.ProcessSQSBatchWithDLQ(ctx.Request.Context(), event, np.processMessage)
}
*/

// Example usage for activity processor:
/*
func (ap *ActivityProcessor) HandleSQSWithDLQ(ctx *lift.Context, event events.SQSEvent) error {
	processor := dlq.NewProcessorWithDLQ("activity-processor", ap.logger)

	return processor.ProcessSQSBatchWithDLQ(ctx.Request.Context(), event, ap.processActivity)
}
*/

// Alternative integration pattern for processors that need more control:

// ProcessorDLQIntegration provides DLQ integration utilities
type ProcessorDLQIntegration struct {
	service   string
	dlqSender *DLQSender
	logger    *zap.Logger
}

// NewProcessorDLQIntegration creates DLQ integration utilities
func NewProcessorDLQIntegration(service string, logger *zap.Logger) *ProcessorDLQIntegration {
	return &ProcessorDLQIntegration{
		service:   service,
		dlqSender: NewDLQSender(logger),
		logger:    logger,
	}
}

// Initialize sets up the DLQ integration
func (p *ProcessorDLQIntegration) Initialize(ctx context.Context) error {
	return p.dlqSender.InitializeAWSClients(ctx)
}

// SendFailure sends a single failure to DLQ
func (p *ProcessorDLQIntegration) SendFailure(ctx context.Context, message events.SQSMessage, err error) {
	if dlqErr := p.dlqSender.SendFailedMessage(ctx, p.service, message, err); dlqErr != nil {
		p.logger.Error("failed to send message to DLQ",
			zap.String("service", p.service),
			zap.String("message_id", message.MessageId),
			zap.Error(dlqErr),
		)
	}
}

// SendBatchFailures sends multiple failures to DLQ
func (p *ProcessorDLQIntegration) SendBatchFailures(ctx context.Context, failures []ProcessingFailure) {
	if err := common.ValidateSliceNotEmpty("failures", failures); err != nil {
		return
	}

	if err := p.dlqSender.SendBatchFailedMessages(ctx, p.service, failures); err != nil {
		p.logger.Error("failed to send batch failures to DLQ",
			zap.String("service", p.service),
			zap.Int("failure_count", len(failures)),
			zap.Error(err),
		)
	} else {
		p.logger.Info("sent batch failures to DLQ",
			zap.String("service", p.service),
			zap.Int("failure_count", len(failures)),
		)
	}
}

// Middleware-style integration for Lift handlers:

// WithDLQSupport wraps a Lift SQS handler with DLQ support
func WithDLQSupport(service string, handler func(*lift.Context, events.SQSEvent) error) func(*lift.Context, events.SQSEvent) error {
	return func(ctx *lift.Context, event events.SQSEvent) error {
		// Create a zap logger for DLQ integration (it expects zap.Logger)
		zapLogger := zap.NewNop()

		// Create DLQ integration
		dlqIntegration := NewProcessorDLQIntegration(service, zapLogger)
		if err := dlqIntegration.Initialize(ctx.Request.Context()); err != nil {
			ctx.Logger.Error("failed to initialize DLQ integration", map[string]any{"error": err.Error()})
		}

		// Process normally
		err := handler(ctx, event)

		// If processing failed, send all messages to DLQ
		if err != nil {
			var failures []ProcessingFailure
			for _, record := range event.Records {
				failures = append(failures, ProcessingFailure{
					OriginalMessage: record,
					Error:           err,
					Timestamp:       time.Now(),
				})
			}
			dlqIntegration.SendBatchFailures(ctx.Request.Context(), failures)
		}

		return err
	}
}

// DLQConfig represents configuration for DLQ integration
//
// DLQConfig represents configuration for DLQ integration
//
//nolint:revive // DLQ prefix clarifies this is Dead Letter Queue config
type DLQConfig struct {
	Enabled         bool     `json:"enabled"`
	Service         string   `json:"service"`
	MaxRetries      int      `json:"max_retries"`
	RetryDelay      int      `json:"retry_delay_seconds"`
	FailFast        bool     `json:"fail_fast"`        // Send to DLQ immediately on certain errors
	PermanentErrors []string `json:"permanent_errors"` // Error patterns that should go straight to DLQ
	TransientErrors []string `json:"transient_errors"` // Error patterns that should be retried
}

// NewDLQConfigFromEnv creates DLQ config from centralized configuration
func NewDLQConfigFromEnv(service string) *DLQConfig {
	cfg := config.Get()
	return &DLQConfig{
		Enabled:         cfg.DLQEnabled,
		Service:         service,
		MaxRetries:      cfg.DLQMaxRetries,
		RetryDelay:      cfg.DLQRetryDelay,
		FailFast:        cfg.DLQFailFast,
		PermanentErrors: cfg.DLQPermanentErrors,
		TransientErrors: cfg.DLQTransientErrors,
	}
}

// parseIntEnv is deprecated - use centralized config instead
// This function is kept for backward compatibility but should not be used
func parseIntEnv(key string, defaultValue int) int {
	cfg := config.Get()
	// Map known DLQ keys to config fields
	switch key {
	case "DLQ_MAX_RETRIES":
		return cfg.DLQMaxRetries
	case "DLQ_RETRY_DELAY":
		return cfg.DLQRetryDelay
	default:
		return defaultValue
	}
}
