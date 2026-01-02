package testing

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/mock"
)

func TestMockAuthService_TokenLifecycle(t *testing.T) {
	mockAuth := SetupMockAuthService()

	claims := CreateTestClaims("alice", "standard")
	mockAuth.On("ValidateAccessToken", "good").Return(claims, nil)
	mockAuth.On("ValidateAccessToken", "bad").Return(nil, auth.ErrInvalidToken)
	mockAuth.On("GenerateAccessToken", mock.Anything).Return("token", nil)
	mockAuth.On("RefreshToken", "refresh").Return("newtoken", nil)
	mockAuth.On("RevokeToken", "tok").Return(nil)

	got, err := mockAuth.ValidateAccessToken("good")
	require.NoError(t, err)
	require.Equal(t, "alice", got.Username)

	got, err = mockAuth.ValidateAccessToken("bad")
	require.Error(t, err)
	require.Nil(t, got)

	token, err := mockAuth.GenerateAccessToken(&auth.Claims{Username: "alice"})
	require.NoError(t, err)
	require.Equal(t, "token", token)

	token, err = mockAuth.RefreshToken("refresh")
	require.NoError(t, err)
	require.Equal(t, "newtoken", token)

	require.NoError(t, mockAuth.RevokeToken("tok"))
	mockAuth.AssertExpectations(t)
}

func TestMockStorage_CommonOperations(t *testing.T) {
	storage := SetupMockStorage()

	actor := BuildTestActor("testuser")
	status := BuildTestStatus("testuser", "hello")
	timeline := &models.Timeline{PK: "pk", SK: "sk"}

	storage.On("CreateActor", mock.Anything, actor).Return(nil)
	storage.On("GetActor", mock.Anything, "testuser").Return(actor, nil)
	storage.On("GetActor", mock.Anything, "missing").Return(nil, errors.New("not found"))
	storage.On("UpdateActor", mock.Anything, actor).Return(nil)
	storage.On("DeleteActor", mock.Anything, actor.PK).Return(nil)

	storage.On("CreateStatus", mock.Anything, status).Return(nil)
	storage.On("GetStatus", mock.Anything, status.PK).Return(status, nil)
	storage.On("UpdateStatus", mock.Anything, status).Return(nil)
	storage.On("DeleteStatus", mock.Anything, status.PK).Return(nil)

	storage.On("CreateActivity", mock.Anything, "testuser", "Create").Return(nil)
	storage.On("GetActivity", mock.Anything, "act").Return(map[string]interface{}{"id": "act"}, nil)

	storage.On("GetTimeline", mock.Anything, "testuser", 10, "").Return([]*models.Timeline{timeline}, "next", nil)
	storage.On("AddToTimeline", mock.Anything, "testuser", timeline).Return(nil)

	storage.On("Follow", mock.Anything, "a", "b").Return(nil)
	storage.On("Unfollow", mock.Anything, "a", "b").Return(nil)
	storage.On("GetFollowers", mock.Anything, "a", 1, "").Return([]*models.Actor{actor}, "c", nil)
	storage.On("GetFollowing", mock.Anything, "a", 1, "").Return([]*models.Actor{actor}, "c", nil)

	ctx := context.Background()

	require.NoError(t, storage.CreateActor(ctx, actor))
	gotActor, err := storage.GetActor(ctx, "testuser")
	require.NoError(t, err)
	require.Equal(t, actor, gotActor)
	gotActor, err = storage.GetActor(ctx, "missing")
	require.Error(t, err)
	require.Nil(t, gotActor)
	require.NoError(t, storage.UpdateActor(ctx, actor))
	require.NoError(t, storage.DeleteActor(ctx, actor.PK))

	require.NoError(t, storage.CreateStatus(ctx, status))
	gotStatus, err := storage.GetStatus(ctx, status.PK)
	require.NoError(t, err)
	require.Equal(t, status, gotStatus)
	require.NoError(t, storage.UpdateStatus(ctx, status))
	require.NoError(t, storage.DeleteStatus(ctx, status.PK))

	require.NoError(t, storage.CreateActivity(ctx, "testuser", "Create"))
	act, err := storage.GetActivity(ctx, "act")
	require.NoError(t, err)
	require.Equal(t, "act", act["id"])

	items, cursor, err := storage.GetTimeline(ctx, "testuser", 10, "")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "next", cursor)
	require.NoError(t, storage.AddToTimeline(ctx, "testuser", timeline))

	require.NoError(t, storage.Follow(ctx, "a", "b"))
	require.NoError(t, storage.Unfollow(ctx, "a", "b"))
	_, _, err = storage.GetFollowers(ctx, "a", 1, "")
	require.NoError(t, err)
	_, _, err = storage.GetFollowing(ctx, "a", 1, "")
	require.NoError(t, err)

	storage.AssertExpectations(t)
}

func TestMockRepositoryHelpers_ExpectationsConfigureMocks(t *testing.T) {
	mockAuth := SetupMockAuthService()
	mockStorage := SetupMockStorage()

	claims := CreateTestClaims("alice", "standard")
	ExpectValidToken(mockAuth, "tok", claims)
	ExpectInvalidToken(mockAuth, "bad")

	actor := BuildTestActor("alice")
	ExpectActorExists(mockStorage, "alice", actor)
	ExpectActorNotFound(mockStorage, "missing")

	status := BuildTestStatus("alice", "hello")
	ExpectStatusExists(mockStorage, status.PK, status)
	ExpectStatusNotFound(mockStorage, "missing-status")

	ctx := context.Background()
	got, err := mockAuth.ValidateAccessToken("tok")
	require.NoError(t, err)
	require.Equal(t, "alice", got.Username)

	_, err = mockAuth.ValidateAccessToken("bad")
	require.Error(t, err)

	gotActor, err := mockStorage.GetActor(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, actor, gotActor)

	gotActor, err = mockStorage.GetActor(ctx, "missing")
	require.Error(t, err)
	require.Nil(t, gotActor)

	gotStatus, err := mockStorage.GetStatus(ctx, status.PK)
	require.NoError(t, err)
	require.Equal(t, status, gotStatus)

	gotStatus, err = mockStorage.GetStatus(ctx, "missing-status")
	require.Error(t, err)
	require.Nil(t, gotStatus)
}

func TestMockWithDelay_AndFailureHelpers(t *testing.T) {
	delay := &MockWithDelay{DelayMs: 10}
	delay.On("SimulateDelay", 10).Return()
	delay.SimulateDelay()
	delay.AssertExpectations(t)

	fail := &MockWithFailure{FailureRate: 0.3}
	fails := 0
	for i := 0; i < 10; i++ {
		if fail.ShouldFail() {
			fails++
		}
	}
	require.Equal(t, 3, fails)
}

func TestBuildTestFactories_ReturnSaneDefaults(t *testing.T) {
	actor := BuildTestActor("testuser")
	require.Equal(t, "actor#testuser", actor.PK)
	require.Equal(t, "USERNAME_SEARCH#te", actor.GSI1PK)
	require.Equal(t, "DOMAIN#test.example.com", actor.GSI3PK)

	status := BuildTestStatus("testuser", "hello")
	require.Equal(t, "testuser", status.AuthorID)
	require.Equal(t, "public", status.Visibility)

	activity := BuildTestActivity("testuser", "Create")
	require.Equal(t, "ACTOR#testuser", activity.PK)
	require.NotNil(t, activity.Activity)
}

func TestAdditionalMocks_RepositoriesAndHTTPClient(t *testing.T) {
	t.Parallel()

	actorRepo, statusRepo := SetupMockRepositories()
	require.NotNil(t, actorRepo)
	require.NotNil(t, statusRepo)

	ctx := context.Background()
	actor := BuildTestActor("testuser")
	status := BuildTestStatus("testuser", "hello")

	actorRepo.On("Create", mock.Anything, actor).Return(nil)
	actorRepo.On("GetByID", mock.Anything, "id").Return(actor, nil)
	actorRepo.On("GetByUsername", mock.Anything, "testuser").Return(actor, nil)
	actorRepo.On("Update", mock.Anything, actor).Return(nil)
	actorRepo.On("Delete", mock.Anything, "id").Return(nil)
	actorRepo.On("List", mock.Anything, 10, "").Return([]*models.Actor{actor}, "next", nil)

	require.NoError(t, actorRepo.Create(ctx, actor))
	_, err := actorRepo.GetByID(ctx, "id")
	require.NoError(t, err)
	_, err = actorRepo.GetByUsername(ctx, "testuser")
	require.NoError(t, err)
	require.NoError(t, actorRepo.Update(ctx, actor))
	require.NoError(t, actorRepo.Delete(ctx, "id"))
	_, _, err = actorRepo.List(ctx, 10, "")
	require.NoError(t, err)

	statusRepo.On("Create", mock.Anything, status).Return(nil)
	statusRepo.On("GetByID", mock.Anything, "id").Return(status, nil)
	statusRepo.On("Update", mock.Anything, status).Return(nil)
	statusRepo.On("Delete", mock.Anything, "id").Return(nil)
	statusRepo.On("GetByActor", mock.Anything, "actor", 10, "").Return([]*models.Status{status}, "next", nil)

	require.NoError(t, statusRepo.Create(ctx, status))
	_, err = statusRepo.GetByID(ctx, "id")
	require.NoError(t, err)
	require.NoError(t, statusRepo.Update(ctx, status))
	require.NoError(t, statusRepo.Delete(ctx, "id"))
	_, _, err = statusRepo.GetByActor(ctx, "actor", 10, "")
	require.NoError(t, err)

	httpClient := &MockHTTPClient{}
	ok := &Response{StatusCode: 200, Body: []byte("ok"), Headers: map[string]string{"Content-Type": "text/plain"}}

	httpClient.On("Get", "https://example.com", mock.Anything).Return(ok, nil)
	httpClient.On("Post", "https://example.com", mock.Anything, mock.Anything).Return(ok, nil)
	httpClient.On("Put", "https://example.com", mock.Anything, mock.Anything).Return(ok, nil)
	httpClient.On("Delete", "https://example.com", mock.Anything).Return(ok, nil)

	_, err = httpClient.Get("https://example.com", map[string]string{})
	require.NoError(t, err)
	_, err = httpClient.Post("https://example.com", []byte("x"), map[string]string{})
	require.NoError(t, err)
	_, err = httpClient.Put("https://example.com", []byte("x"), map[string]string{})
	require.NoError(t, err)
	_, err = httpClient.Delete("https://example.com", map[string]string{})
	require.NoError(t, err)
}
