package patterns

import (
	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Example: DynamoDB Stream Handler
type ExampleStreamHandler struct {
	logger *zap.Logger
}

func (h *ExampleStreamHandler) HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error {
	for _, record := range event.Records {
		h.logger.Info("processing record",
			zap.String("event_name", record.EventName),
			zap.String("event_id", record.EventID),
		)
		// Process the record...
	}
	return nil
}

// Example usage in main.go:
// func main() {
//     logger := common.Logger()
//     handler := &ExampleStreamHandler{logger: logger}
//     StartDynamoDBStreamLambda("my-processor", handler, logger)
// }

// Example: SQS Handler
type ExampleSQSHandler struct {
	logger *zap.Logger
}

func (h *ExampleSQSHandler) HandleSQS(ctx *lift.Context, event events.SQSEvent) error {
	// Process messages with partial batch failure support
	return ProcessSQSBatch(ctx, event, func(ctx *lift.Context, msg events.SQSMessage) error {
		h.logger.Info("processing message",
			zap.String("message_id", msg.MessageId),
			zap.String("body", msg.Body),
		)
		// Process the message...
		return nil
	})
}

// Example usage in main.go:
// func main() {
//     logger := common.Logger()
//     handler := &ExampleSQSHandler{logger: logger}
//     StartSQSLambda("my-queue", handler, logger)
// }

// Example: Scheduled Task Handler
type ExampleScheduledHandler struct {
	logger *zap.Logger
}

func (h *ExampleScheduledHandler) HandleScheduledEvent(ctx *lift.Context) error {
	h.logger.Info("running scheduled task")
	// Perform scheduled work...
	return nil
}

// Example usage in main.go:
// func main() {
//     logger := common.Logger()
//     handler := &ExampleScheduledHandler{logger: logger}
//     StartScheduledLambda("daily-aggregation", handler, logger)
// }