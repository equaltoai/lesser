package services

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
)

type streamUsernameRecordingPublisher struct {
	userTargets   []string
	userStreams   []string
	streamTargets []string
}

func (p *streamUsernameRecordingPublisher) PublishToUser(_ context.Context, userID string, event *streaming.Event) error {
	p.userTargets = append(p.userTargets, userID)
	p.userStreams = append(p.userStreams, event.Stream)
	return nil
}

func (p *streamUsernameRecordingPublisher) PublishToStream(_ context.Context, streamName string, _ *streaming.Event) error {
	p.streamTargets = append(p.streamTargets, streamName)
	return nil
}

func (p *streamUsernameRecordingPublisher) PublishToConversation(_ context.Context, _ string, _ *streaming.Event) error {
	return nil
}

func (p *streamUsernameRecordingPublisher) Close() error { return nil }

func TestBusinessLogicEvents_RejectInvalidActorUsernameBeforeUserStreamRouting(t *testing.T) {
	svc, _ := newBusinessLogicServiceForRound18Test(t)
	publisher := &streamUsernameRecordingPublisher{}
	svc.publisher = publisher
	actor := &activitypub.Actor{}
	undo := &activitypub.Activity{Object: &activitypub.Activity{Object: "https://remote.example/objects/1"}}

	svc.emitUnfollowEvents(context.Background(), undo, actor)
	svc.emitUnlikeEvents(context.Background(), undo, actor)
	svc.emitPostUpdateEvents(context.Background(), &activitypub.Activity{}, actor, &activitypub.Note{Visibility: VisibilityPublic})

	require.Empty(t, publisher.userTargets)
	require.NotContains(t, publisher.userStreams, "user:")
	require.Equal(t, []string{"public"}, publisher.streamTargets, "a malformed actor must not suppress the safe public fanout")
}

func TestBusinessLogicEvents_SanitizeAndValidateActorUsername(t *testing.T) {
	svc, _ := newBusinessLogicServiceForRound18Test(t)
	publisher := &streamUsernameRecordingPublisher{}
	svc.publisher = publisher
	actor := &activitypub.Actor{PreferredUsername: " alice "}
	undo := &activitypub.Activity{Object: &activitypub.Activity{Object: "https://remote.example/objects/1"}}

	svc.emitUnfollowEvents(context.Background(), undo, actor)

	require.Equal(t, []string{"alice"}, publisher.userTargets)
	require.Equal(t, []string{"user:alice"}, publisher.userStreams)
}
