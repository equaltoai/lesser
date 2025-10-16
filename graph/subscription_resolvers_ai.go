package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// AiAnalysisUpdates implements SubscriptionResolver
func (r *subscriptionResolver) AiAnalysisUpdates(ctx context.Context, objectID *string) (<-chan *model.AIAnalysis, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Create channel for updates
	updatesChan := make(chan *model.AIAnalysis, 100)

	// Get AI service from registry
	aiSvc := r.Registry.AI()
	if aiSvc == nil {
		close(updatesChan)
		return updatesChan, nil // Return empty channel if no AI service
	}

	// Subscribe to AI analysis events from the service
	eventChan, err := aiSvc.SubscribeToAnalysisEvents(ctx, username, objectID)
	if err != nil {
		close(updatesChan)
		return nil, errors.Join(errors.New("failed to subscribe to AI analysis events"), err)
	}

	// Start goroutine to convert service events to GraphQL model
	go func() {
		defer close(updatesChan)

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-eventChan:
				if !ok {
					return // Event channel closed
				}

				// Convert service event to GraphQL model
				analysis := &model.AIAnalysis{
					ID:         event.ID,
					ObjectID:   event.ObjectID,
					ObjectType: event.ObjectType,
					// Convert results to proper analysis types
					TextAnalysis:  r.convertToTextAnalysis(event.Results),
					ImageAnalysis: r.convertToImageAnalysis(event.Results),
					AiDetection:   r.convertToAIDetection(event.Results),
					SpamAnalysis:  r.convertToSpamAnalysis(event.Results),
					OverallRisk:   0.0, // Calculate from results
					Confidence:    event.Confidence,
					AnalyzedAt:    model.Time(event.ProcessedAt),
				}

				// Add moderation action if present
				if event.ModerationAction != "" {
					analysis.ModerationAction = model.ModerationAction(event.ModerationAction)
				}

				select {
				case updatesChan <- analysis:
					r.Logger.Debug("Forwarded AI analysis update",
						zap.String("object_id", event.ObjectID),
						zap.String("analysis_id", event.ID))
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	r.Logger.Info("Started AI analysis updates subscription",
		zap.String("user", username),
		zap.Bool("filtered", objectID != nil))

	return updatesChan, nil
}

// ThreatIntelligence implements SubscriptionResolver
func (r *subscriptionResolver) ThreatIntelligence(ctx context.Context) (<-chan *model.ThreatAlert, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	alertChan := make(chan *model.ThreatAlert, 100)

	// For now, return empty channel
	// This would be implemented with real threat intelligence streaming
	go func() {
		<-ctx.Done()
		close(alertChan)
	}()

	r.Logger.Info("Started threat intelligence subscription",
		zap.String("user", username))

	return alertChan, nil
}
