package patterns

import (
	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// ExampleStreamHandler demonstrates DynamoDB Stream handler pattern
type ExampleStreamHandler struct {
	logger *zap.Logger
}

// HandleStream processes DynamoDB stream events
func (h *ExampleStreamHandler) HandleStream(_ *lift.Context, event events.DynamoDBEvent) error {
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
//     app := lift.New()
//     _ = app.DynamoDB("*", func(ctx *lift.Context) error {
//         records, err := ctx.DynamoDBRecords()
//         if err != nil {
//             return err
//         }
//         return handler.HandleStream(ctx, events.DynamoDBEvent{Records: records})
//     })
//     lambda.Start(app.HandleRequest)
// }

// ExampleSQSHandler demonstrates SQS handler pattern
type ExampleSQSHandler struct {
	logger *zap.Logger
}

// HandleSQS processes SQS events with partial batch failure support
func (h *ExampleSQSHandler) HandleSQS(ctx *lift.Context, event events.SQSEvent) error {
	// Process messages with partial batch failure support
	return ProcessSQSBatch(ctx, event, func(_ *lift.Context, msg events.SQSMessage) error {
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

// ExampleScheduledHandler demonstrates scheduled task handler pattern
type ExampleScheduledHandler struct {
	logger *zap.Logger
}

// HandleScheduledEvent processes scheduled events from CloudWatch Events
func (h *ExampleScheduledHandler) HandleScheduledEvent(_ *lift.Context) error {
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
