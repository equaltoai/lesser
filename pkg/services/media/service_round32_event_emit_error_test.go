package media

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type failingPublisherRound32 struct{}

func (f *failingPublisherRound32) PublishToUser(context.Context, string, *streaming.Event) error {
	return errors.New("boom")
}

func (f *failingPublisherRound32) PublishToStream(context.Context, string, *streaming.Event) error {
	return errors.New("boom")
}

func (f *failingPublisherRound32) PublishToConversation(context.Context, string, *streaming.Event) error {
	return errors.New("boom")
}

func (f *failingPublisherRound32) Close() error { return nil }

func TestService_emitMediaUploadedEvents_SwallowsPublishErrors(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, &failingPublisherRound32{}, nil, zap.NewNop(), "bucket", "cdn.example.com")
	events := service.emitMediaUploadedEvents(context.Background(), &models.Media{MediaID: "m1", UserID: "alice"})
	require.Empty(t, events)
}
