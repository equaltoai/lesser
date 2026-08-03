package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestValidateTimelineRoutingInputsFailsClosed(t *testing.T) {
	actor := "alice"
	hashtag := "golang"
	listID := "list-1"
	empty := " \t "
	invalidActor := "alice:admin"
	invalidHashtag := "go:lang"
	invalidList := "list:1"

	tests := []struct {
		name         string
		timelineType model.TimelineType
		inputs       timelineRoutingInputs
		wantField    string
		wantMessage  string
	}{
		{name: "actor missing", timelineType: model.TimelineTypeActor, wantField: "actorId", wantMessage: "parameter required"},
		{name: "actor blank", timelineType: model.TimelineTypeActor, inputs: timelineRoutingInputs{actorID: &empty}, wantField: "actorId", wantMessage: "valid actor stream username"},
		{name: "actor malformed", timelineType: model.TimelineTypeActor, inputs: timelineRoutingInputs{actorID: &invalidActor}, wantField: "actorId", wantMessage: "valid actor stream username"},
		{name: "actor extra hashtag", timelineType: model.TimelineTypeActor, inputs: timelineRoutingInputs{actorID: &actor, hashtag: &hashtag}, wantField: "hashtag", wantMessage: "not allowed for ACTOR timeline"},
		{name: "hashtag missing", timelineType: model.TimelineTypeHashtag, wantField: "hashtag", wantMessage: "parameter required"},
		{name: "hashtag blank", timelineType: model.TimelineTypeHashtag, inputs: timelineRoutingInputs{hashtag: &empty}, wantField: "hashtag", wantMessage: "valid hashtag"},
		{name: "hashtag malformed", timelineType: model.TimelineTypeHashtag, inputs: timelineRoutingInputs{hashtag: &invalidHashtag}, wantField: "hashtag", wantMessage: "valid hashtag"},
		{name: "hashtag extra list", timelineType: model.TimelineTypeHashtag, inputs: timelineRoutingInputs{hashtag: &hashtag, listID: &listID}, wantField: "listId", wantMessage: "not allowed for HASHTAG timeline"},
		{name: "list missing", timelineType: model.TimelineTypeList, wantField: "listId", wantMessage: "parameter required"},
		{name: "list blank", timelineType: model.TimelineTypeList, inputs: timelineRoutingInputs{listID: &empty}, wantField: "listId", wantMessage: "valid list ID"},
		{name: "list malformed", timelineType: model.TimelineTypeList, inputs: timelineRoutingInputs{listID: &invalidList}, wantField: "listId", wantMessage: "valid list ID"},
		{name: "list extra actor", timelineType: model.TimelineTypeList, inputs: timelineRoutingInputs{actorID: &actor, listID: &listID}, wantField: "actorId", wantMessage: "not allowed for LIST timeline"},
		{name: "public extra actor", timelineType: model.TimelineTypePublic, inputs: timelineRoutingInputs{actorID: &actor}, wantField: "actorId", wantMessage: "not allowed for PUBLIC timeline"},
		{name: "local extra hashtag", timelineType: model.TimelineTypeLocal, inputs: timelineRoutingInputs{hashtag: &hashtag}, wantField: "hashtag", wantMessage: "not allowed for LOCAL timeline"},
		{name: "home extra list", timelineType: model.TimelineTypeHome, inputs: timelineRoutingInputs{listID: &listID}, wantField: "listId", wantMessage: "not allowed for HOME timeline"},
		{name: "direct extra actor", timelineType: model.TimelineTypeDirect, inputs: timelineRoutingInputs{actorID: &actor}, wantField: "actorId", wantMessage: "not allowed for DIRECT timeline"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateTimelineRoutingInputs(test.timelineType, test.inputs)
			require.Error(t, err)
			require.True(t, apperrors.HasCode(err, apperrors.CodeValidationFailed), "unexpected error: %v", err)
			require.ErrorContains(t, err, test.wantMessage)

			var appErr *apperrors.AppError
			require.ErrorAs(t, err, &appErr)
			require.Equal(t, test.wantField, appErr.Metadata["field"])
		})
	}
}

func TestTimelineStreamNameCanonicalizesNewRoutingInputs(t *testing.T) {
	actor := " alice-1 "
	hashtag := " #GoLang "
	listID := " list_123 "

	tests := []struct {
		name         string
		timelineType model.TimelineType
		inputs       timelineRoutingInputs
		want         string
	}{
		{name: "actor", timelineType: model.TimelineTypeActor, inputs: timelineRoutingInputs{actorID: &actor}, want: streaming.UserStreamName("alice-1")},
		{name: "hashtag", timelineType: model.TimelineTypeHashtag, inputs: timelineRoutingInputs{hashtag: &hashtag}, want: streaming.HashtagStreamName("golang")},
		{name: "list", timelineType: model.TimelineTypeList, inputs: timelineRoutingInputs{listID: &listID}, want: streaming.ListStreamName("list_123")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := timelineStreamName("", test.timelineType, test.inputs)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestSubscribeToTimelinePersistsNewCanonicalRoutes(t *testing.T) {
	connRepo := inmemory.NewStreamingConnectionRepository()
	manager := NewGraphQLSubscriptionManager(connRepo, streaming.NewMockPublisher(), nil)
	ctx, cancel := context.WithCancel(WithConnectionID(context.Background(), "routing-inputs-conn"))
	t.Cleanup(cancel)
	require.NoError(t, manager.Start(ctx))
	t.Cleanup(func() { _ = manager.Stop() })

	actor := " alice "
	hashtag := " #GoLang "
	listID := " list-1 "
	tests := []struct {
		name         string
		timelineType model.TimelineType
		inputs       timelineRoutingInputs
		wantStream   string
	}{
		{name: "actor", timelineType: model.TimelineTypeActor, inputs: timelineRoutingInputs{actorID: &actor}, wantStream: streaming.UserStreamName("alice")},
		{name: "hashtag", timelineType: model.TimelineTypeHashtag, inputs: timelineRoutingInputs{hashtag: &hashtag}, wantStream: streaming.HashtagStreamName("golang")},
		{name: "list", timelineType: model.TimelineTypeList, inputs: timelineRoutingInputs{listID: &listID}, wantStream: streaming.ListStreamName("list-1")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updates, err := manager.SubscribeToTimeline(
				ctx, "alice", test.timelineType, test.inputs.actorID, test.inputs.hashtag, test.inputs.listID,
			)
			require.NoError(t, err)
			require.NotNil(t, updates)

			subscriptions, err := connRepo.GetSubscriptionsForStream(ctx, test.wantStream)
			require.NoError(t, err)
			require.Len(t, subscriptions, 1)
		})
	}
}

func TestSubscribeToTimelineRejectsMalformedRoutesBeforePersistence(t *testing.T) {
	connRepo := inmemory.NewStreamingConnectionRepository()
	manager := NewGraphQLSubscriptionManager(connRepo, streaming.NewMockPublisher(), nil)
	ctx, cancel := context.WithCancel(WithConnectionID(context.Background(), "invalid-routing-conn"))
	t.Cleanup(cancel)
	require.NoError(t, manager.Start(ctx))
	t.Cleanup(func() { _ = manager.Stop() })

	invalidActor := "alice:admin"
	invalidHashtag := "go:lang"
	invalidList := "list:private"
	tests := []struct {
		name          string
		timelineType  model.TimelineType
		inputs        timelineRoutingInputs
		forbiddenName string
	}{
		{name: "actor", timelineType: model.TimelineTypeActor, inputs: timelineRoutingInputs{actorID: &invalidActor}, forbiddenName: "user:alice:admin"},
		{name: "hashtag", timelineType: model.TimelineTypeHashtag, inputs: timelineRoutingInputs{hashtag: &invalidHashtag}, forbiddenName: "hashtag:go:lang"},
		{name: "list", timelineType: model.TimelineTypeList, inputs: timelineRoutingInputs{listID: &invalidList}, forbiddenName: "list:list:private"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := manager.SubscribeToTimeline(
				ctx, "alice", test.timelineType, test.inputs.actorID, test.inputs.hashtag, test.inputs.listID,
			)
			require.Error(t, err)

			subscriptions, lookupErr := connRepo.GetSubscriptionsForStream(ctx, test.forbiddenName)
			require.NoError(t, lookupErr)
			require.Empty(t, subscriptions)
		})
	}
}

func TestTimelineUpdatesListIsAuthenticatedOnly(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	connRepo := inmemory.NewStreamingConnectionRepository()
	manager := NewSubscriptionManager(connRepo, streaming.NewMockPublisher(), nil)
	resolver.SubscriptionManager = manager

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, manager.Start(ctx))
	t.Cleanup(func() { _ = manager.Stop() })

	listID := " list-1 "
	_, err := resolver.Subscription().TimelineUpdates(
		WithConnectionID(context.Background(), "anonymous-list"), model.TimelineTypeList, nil, nil, &listID,
	)
	require.ErrorIs(t, err, ErrAuthenticationRequired)

	updates, err := resolver.Subscription().TimelineUpdates(
		WithConnectionID(round12AuthContext("alice"), "authenticated-list"), model.TimelineTypeList, nil, nil, &listID,
	)
	require.NoError(t, err)
	require.NotNil(t, updates)

	subscriptions, err := connRepo.GetSubscriptionsForStream(ctx, streaming.ListStreamName("list-1"))
	require.NoError(t, err)
	require.Len(t, subscriptions, 1)
}
