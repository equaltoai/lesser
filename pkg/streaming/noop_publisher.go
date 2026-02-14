package streaming

import "context"

// NewNoopPublisher returns a Publisher implementation that drops all events.
//
// This is used as a safe default when streaming is not configured so API paths
// never panic when emitting optional real-time events.
func NewNoopPublisher() Publisher {
	return &noopPublisher{}
}

type noopPublisher struct{}

func (p *noopPublisher) PublishToUser(_ context.Context, _ string, _ *Event) error {
	return nil
}

func (p *noopPublisher) PublishToStream(_ context.Context, _ string, _ *Event) error {
	return nil
}

func (p *noopPublisher) PublishToConversation(_ context.Context, _ string, _ *Event) error {
	return nil
}

func (p *noopPublisher) Close() error {
	return nil
}
