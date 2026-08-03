package routing

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestInboxHandler_Round11_RemoteFollowIntakeCanonicalizesNestedBaseObjectLocalActorRow(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	env.handler.actorRepository = newNestedBaseObjectLocalActorRepository(t, env, "alice", true)

	relationshipRepo := testmocks.NewMockRelationshipRepository()
	notificationRepo := testmocks.NewMockNotificationRepository()
	env.handler.relationshipRepository = relationshipRepo
	env.handler.notificationRepository = notificationRepo

	loadedActor, err := env.handler.actorRepository.GetActorByUsername(context.Background(), "alice")
	require.NoError(t, err)

	targetActorID := loadedActor.ID
	followerHandle := env.handler.extractHandleFromActorID(env.remoteActorID)
	followActivityID := env.cfg.BaseURL() + "/activities/follow-nested-baseobject"

	relationshipRepo.On("IsBlockedBidirectional", mock.Anything, env.remoteActorID, targetActorID).Return(false, nil).Once()
	relationshipRepo.On("CreateRelationship", mock.Anything, followerHandle, "alice", followActivityID).Return(nil).Once()
	notificationRepo.On("CreateNotification", mock.Anything, mock.MatchedBy(func(notification *models.Notification) bool {
		return notification != nil &&
			notification.UserID == "alice" &&
			notification.Type == "follow_request" &&
			notification.ActorID == followerHandle &&
			notification.TargetID == followerHandle
	})).Return(nil).Once()

	activity := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.FollowType,
		"id":       followActivityID,
		"actor":    env.remoteActorID,
		"object":   targetActorID,
		"to":       []string{targetActorID, targetActorID + "/inbox"},
	}
	body, err := json.Marshal(activity)
	require.NoError(t, err)

	headers := map[string]string{
		"Host":         env.cfg.Domain,
		"Content-Type": "application/activity+json",
		"User-Agent":   "Mastodon/4.0.0",
	}
	ctx := newAppTheoryContext("POST", "/users/alice/inbox", headers, nil, body)
	ctx.Params["username"] = "alice"

	signAppTheoryRequest(t, env, ctx, body)

	resp, err := env.handler.handlePostInbox(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 202, resp.Status)

	relationshipRepo.AssertExpectations(t)
	notificationRepo.AssertExpectations(t)
}

func newNestedBaseObjectLocalActorRepository(t *testing.T, env *inboxTestEnv, username string, manuallyApprovesFollowers bool) *repositories.ActorRepository {
	t.Helper()

	actorDB := new(dynamormmocks.MockDB)
	actorQuery := new(dynamormmocks.MockQuery)

	actorDB.On("WithContext", mock.Anything).Return(actorDB).Maybe()
	actorDB.On("Model", mock.Anything).Return(actorQuery).Maybe()
	actorQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(actorQuery).Maybe()
	actorQuery.On("Select", mock.Anything).Return(actorQuery).Maybe()
	actorQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		row := args.Get(0).(*models.Actor)
		row.Username = username
		row.Actor = &activitypub.Actor{
			PreferredUsername:         username,
			ManuallyApprovesFollowers: manuallyApprovesFollowers,
		}
		row.CreatedAt = time.Date(2026, 4, 7, 11, 0, 0, 0, time.UTC)
		row.UpdatedAt = time.Date(2026, 4, 7, 11, 5, 0, 0, time.UTC)
	}).Twice()

	t.Cleanup(func() {
		actorDB.AssertExpectations(t)
		actorQuery.AssertExpectations(t)
	})

	return repositories.NewActorRepository(actorDB, env.cfg.DynamoTableName, zap.NewNop(), env.cfg.Domain)
}
