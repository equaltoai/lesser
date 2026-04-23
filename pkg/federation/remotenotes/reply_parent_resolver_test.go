package remotenotes

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	commonerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubStatusRepo struct {
	byID      map[string]*models.Status
	byURL     map[string]*models.Status
	createErr error
}

func (s *stubStatusRepo) GetStatus(_ context.Context, statusID string) (*models.Status, error) {
	if s.byID != nil {
		if status := s.byID[statusID]; status != nil {
			return status, nil
		}
	}
	return nil, storage.ErrNotFound
}

func (s *stubStatusRepo) GetStatusByURL(_ context.Context, raw string) (*models.Status, error) {
	if s.byURL != nil {
		if status := s.byURL[raw]; status != nil {
			return status, nil
		}
	}
	return nil, storage.ErrNotFound
}

func (s *stubStatusRepo) CreateStatus(_ context.Context, status *models.Status) error {
	if s.createErr != nil {
		return s.createErr
	}
	if s.byID == nil {
		s.byID = map[string]*models.Status{}
	}
	if s.byURL == nil {
		s.byURL = map[string]*models.Status{}
	}
	s.byID[status.StatusID] = status
	if status.Note != nil && status.Note.ID != "" {
		s.byURL[status.Note.ID] = status
	}
	return nil
}

type stubObjectRepo struct {
	created   []any
	createErr error
}

func (s *stubObjectRepo) CreateObject(_ context.Context, object any) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.created = append(s.created, object)
	return nil
}

type stubDomainBlockRepo struct {
	blocked map[string]bool
	err     error
}

func (s *stubDomainBlockRepo) IsDomainBlocked(_ context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	if s.err != nil {
		return false, nil, s.err
	}
	blocked := s.blocked != nil && s.blocked[domain]
	return blocked, nil, nil
}

type stubFetcher struct {
	obj      any
	err      error
	gotURL   string
	gotActor *activitypub.Actor
}

func (s *stubFetcher) FetchObject(_ context.Context, objectURL string, signingActor *activitypub.Actor) (any, error) {
	s.gotURL = objectURL
	s.gotActor = signingActor
	if s.err != nil {
		return nil, s.err
	}
	return s.obj, nil
}

func TestReplyParentResolver_ResolveReplyParent(t *testing.T) {
	t.Run("returns stored parent without remote fetch", func(t *testing.T) {
		parentURL := "https://remote.example/users/steward/statuses/seed"
		stored := remoteStatus(parentURL, models.VisibilityPublic)
		fetcher := &stubFetcher{}
		resolver := NewReplyParentResolver(
			&stubStatusRepo{
				byID:  map[string]*models.Status{stored.StatusID: stored},
				byURL: map[string]*models.Status{parentURL: stored},
			},
			&stubObjectRepo{},
			&stubDomainBlockRepo{},
			fetcher,
			"example.com",
			zap.NewNop(),
		)

		result, err := resolver.ResolveReplyParent(context.Background(), localAuthorAccount("alice"), parentURL, models.VisibilityPublic)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Fetched)
		assert.True(t, result.Remote)
		assert.Equal(t, stored.StatusID, result.CanonicalStatusID)
		assert.Empty(t, fetcher.gotURL)
	})

	t.Run("stored private parent cannot be broadened to public", func(t *testing.T) {
		parentURL := "https://remote.example/users/steward/statuses/private-stored"
		stored := remoteStatus(parentURL, models.VisibilityPrivate)
		fetcher := &stubFetcher{}
		resolver := NewReplyParentResolver(
			&stubStatusRepo{
				byID:  map[string]*models.Status{stored.StatusID: stored},
				byURL: map[string]*models.Status{parentURL: stored},
			},
			&stubObjectRepo{},
			&stubDomainBlockRepo{},
			fetcher,
			"example.com",
			zap.NewNop(),
		)

		_, err := resolver.ResolveReplyParent(context.Background(), localAuthorAccount("alice"), parentURL, models.VisibilityPublic)
		appErr, ok := commonerrors.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeUnprocessableEntity, appErr.Code)
		assert.Empty(t, fetcher.gotURL)
	})

	t.Run("materializes unresolved remote parent url through authorized fetch", func(t *testing.T) {
		parentURL := "https://remote.example/users/steward/statuses/fresh"
		statusRepo := &stubStatusRepo{}
		objectRepo := &stubObjectRepo{}
		fetcher := &stubFetcher{obj: remoteNoteObject(parentURL, models.VisibilityPublic)}
		resolver := NewReplyParentResolver(
			statusRepo,
			objectRepo,
			&stubDomainBlockRepo{},
			fetcher,
			"example.com",
			zap.NewNop(),
		)

		result, err := resolver.ResolveReplyParent(context.Background(), localAuthorAccount("alice"), parentURL, models.VisibilityPublic)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Status)
		require.Len(t, objectRepo.created, 1)
		assert.True(t, result.Fetched)
		assert.True(t, result.Remote)
		assert.Equal(t, parentURL, result.CanonicalObjectURL)
		assert.Equal(t, models.CanonicalStatusIDForDomain(parentURL, "example.com"), result.CanonicalStatusID)
		assert.Equal(t, parentURL, fetcher.gotURL)
		require.NotNil(t, fetcher.gotActor)
		assert.Equal(t, "alice", fetcher.gotActor.PreferredUsername)
		assert.Equal(t, "https://example.com/users/alice", fetcher.gotActor.ID)
	})

	t.Run("timeout maps to 408", func(t *testing.T) {
		parentURL := "https://remote.example/users/steward/statuses/slow"
		resolver := NewReplyParentResolver(
			&stubStatusRepo{},
			&stubObjectRepo{},
			&stubDomainBlockRepo{},
			&stubFetcher{err: context.DeadlineExceeded},
			"example.com",
			zap.NewNop(),
		)

		_, err := resolver.ResolveReplyParent(context.Background(), localAuthorAccount("alice"), parentURL, models.VisibilityPublic)
		appErr, ok := commonerrors.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeTimeout, appErr.Code)
		assert.Equal(t, 408, appErr.HTTPStatusCode)
	})

	t.Run("unreachable remote parent maps to 503", func(t *testing.T) {
		parentURL := "https://remote.example/users/steward/statuses/down"
		resolver := NewReplyParentResolver(
			&stubStatusRepo{},
			&stubObjectRepo{},
			&stubDomainBlockRepo{},
			&stubFetcher{err: stdErrors.New("dial tcp: no route to host")},
			"example.com",
			zap.NewNop(),
		)

		_, err := resolver.ResolveReplyParent(context.Background(), localAuthorAccount("alice"), parentURL, models.VisibilityPublic)
		appErr, ok := commonerrors.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeExternalServiceUnavailable, appErr.Code)
		assert.Equal(t, 503, appErr.HTTPStatusCode)
	})

	t.Run("not found parent maps to 503", func(t *testing.T) {
		parentURL := "https://remote.example/users/steward/statuses/missing"
		resolver := NewReplyParentResolver(
			&stubStatusRepo{},
			&stubObjectRepo{},
			&stubDomainBlockRepo{},
			&stubFetcher{err: commonerrors.RemoteFetchNotFound(parentURL)},
			"example.com",
			zap.NewNop(),
		)

		_, err := resolver.ResolveReplyParent(context.Background(), localAuthorAccount("alice"), parentURL, models.VisibilityPublic)
		appErr, ok := commonerrors.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeExternalServiceUnavailable, appErr.Code)
		assert.Equal(t, 503, appErr.HTTPStatusCode)
	})

	t.Run("fetched but semantically unusable parent maps to 422", func(t *testing.T) {
		parentURL := "https://remote.example/users/steward/statuses/article"
		resolver := NewReplyParentResolver(
			&stubStatusRepo{},
			&stubObjectRepo{},
			&stubDomainBlockRepo{},
			&stubFetcher{obj: map[string]any{
				"@context":     []any{"https://www.w3.org/ns/activitystreams"},
				"id":           parentURL,
				"type":         "Article",
				"content":      "not a note",
				"attributedTo": "https://remote.example/users/steward",
			}},
			"example.com",
			zap.NewNop(),
		)

		_, err := resolver.ResolveReplyParent(context.Background(), localAuthorAccount("alice"), parentURL, models.VisibilityPublic)
		appErr, ok := commonerrors.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeUnprocessableEntity, appErr.Code)
		assert.Equal(t, 422, appErr.HTTPStatusCode)
	})

	t.Run("private parent cannot be broadened to public", func(t *testing.T) {
		parentURL := "https://remote.example/users/steward/statuses/private"
		resolver := NewReplyParentResolver(
			&stubStatusRepo{},
			&stubObjectRepo{},
			&stubDomainBlockRepo{},
			&stubFetcher{obj: remoteNoteObject(parentURL, models.VisibilityPrivate)},
			"example.com",
			zap.NewNop(),
		)

		_, err := resolver.ResolveReplyParent(context.Background(), localAuthorAccount("alice"), parentURL, models.VisibilityPublic)
		appErr, ok := commonerrors.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeUnprocessableEntity, appErr.Code)
	})

	t.Run("direct replies stay out of scope for notes create path", func(t *testing.T) {
		parentURL := "https://remote.example/users/steward/statuses/direct"
		fetcher := &stubFetcher{obj: remoteNoteObject(parentURL, models.VisibilityPrivate)}
		resolver := NewReplyParentResolver(
			&stubStatusRepo{},
			&stubObjectRepo{},
			&stubDomainBlockRepo{},
			fetcher,
			"example.com",
			zap.NewNop(),
		)

		_, err := resolver.ResolveReplyParent(context.Background(), localAuthorAccount("alice"), parentURL, models.VisibilityDirect)
		appErr, ok := commonerrors.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeUnprocessableEntity, appErr.Code)
		assert.Empty(t, fetcher.gotURL)
	})

	t.Run("blocked parent domain is rejected before fetch", func(t *testing.T) {
		parentURL := "https://blocked.example/users/steward/statuses/private"
		fetcher := &stubFetcher{obj: remoteNoteObject(parentURL, models.VisibilityPrivate)}
		resolver := NewReplyParentResolver(
			&stubStatusRepo{},
			&stubObjectRepo{},
			&stubDomainBlockRepo{blocked: map[string]bool{"blocked.example": true}},
			fetcher,
			"example.com",
			zap.NewNop(),
		)

		_, err := resolver.ResolveReplyParent(context.Background(), localAuthorAccount("alice"), parentURL, models.VisibilityPrivate)
		appErr, ok := commonerrors.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeUnprocessableEntity, appErr.Code)
		assert.Empty(t, fetcher.gotURL)
	})
}

func localAuthorAccount(username string) *storage.Account {
	actorID := "https://example.com/users/" + username
	return &storage.Account{
		User: &storage.User{Username: username},
		Actor: &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: actorID},
			PreferredUsername: username,
			PublicKey: &activitypub.PublicKey{
				ID:    actorID + "#main-key",
				Owner: actorID,
			},
		},
	}
}

func remoteStatus(parentURL string, visibility string) *models.Status {
	at := time.Now().UTC()
	return &models.Status{
		StatusID:       models.CanonicalStatusIDForDomain(parentURL, "example.com"),
		AuthorID:       "https://remote.example/users/steward",
		AuthorUsername: "steward@remote.example",
		ConversationID: "conv-1",
		Visibility:     visibility,
		PublishedAt:    at,
		CreatedAt:      at,
		UpdatedAt:      at,
		ModifiedAt:     at,
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   parentURL,
				To:   replyAudienceForVisibility(visibility),
				CC:   replyCCForVisibility(visibility),
				Type: activitypub.NoteType,
			},
			AttributedTo: "https://remote.example/users/steward",
			Content:      "seed",
			Visibility:   visibility,
		},
	}
}

func remoteNoteObject(parentURL string, visibility string) map[string]any {
	return map[string]any{
		"@context":     []any{"https://www.w3.org/ns/activitystreams"},
		"id":           parentURL,
		"type":         activitypub.NoteType,
		"content":      "seed",
		"attributedTo": "https://remote.example/users/steward",
		"to":           stringSliceToAny(replyAudienceForVisibility(visibility)),
		"cc":           stringSliceToAny(replyCCForVisibility(visibility)),
		"visibility":   visibility,
	}
}

func stringSliceToAny(values []string) []any {
	if len(values) == 0 {
		return nil
	}

	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}

	return out
}

func replyAudienceForVisibility(visibility string) []string {
	if visibility == models.VisibilityPrivate {
		return []string{"https://remote.example/users/steward/followers"}
	}
	return []string{activitypub.PublicAddress}
}

func replyCCForVisibility(visibility string) []string {
	if visibility == models.VisibilityPrivate {
		return nil
	}
	return []string{"https://remote.example/users/steward/followers"}
}
