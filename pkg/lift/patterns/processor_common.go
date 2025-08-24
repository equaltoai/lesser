package patterns

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// ProcessEventConfig contains configuration for generic event processing
type ProcessEventConfig struct {
	ProcessorName string
	RequestIDKey  string
	RecordCount   int
	Logger        *zap.Logger
}

// ProcessEventWithTiming provides generic event processing with logging and timing
func ProcessEventWithTiming(
	ctx *lift.Context,
	config ProcessEventConfig,
	handler func(*lift.Context) error,
) error {
	start := time.Now()
	requestID := ctx.GetRequestID()

	if err := common.ValidateRequiredParam("requestID", requestID); err != nil {
		requestID = fmt.Sprintf("%s-%d", config.ProcessorName, time.Now().UnixNano())
		ctx.Set("requestID", requestID)
	}

	config.Logger.Info("processing event batch",
		zap.String("processor", config.ProcessorName),
		zap.String("request_id", requestID),
		zap.Int("record_count", config.RecordCount),
	)

	// Call the actual handler
	err := handler(ctx)

	duration := time.Since(start)
	if err != nil {
		config.Logger.Error("failed to process event batch",
			zap.String("processor", config.ProcessorName),
			zap.String("request_id", requestID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
		return err
	}

	config.Logger.Info("successfully processed event batch",
		zap.String("processor", config.ProcessorName),
		zap.String("request_id", requestID),
		zap.Int("record_count", config.RecordCount),
		zap.Duration("duration", duration),
	)

	return nil
}
