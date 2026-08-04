package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestM4DraftPublishCreatesCanonicalArticleWithAttribution(t *testing.T) {
	t.Parallel()

	repo := newReviewMemRepo()
	articles := newMemArticleService()
	svc := NewDraftService(repo, nil, "example.com", false, zap.NewNop())
	svc.articleService = articles
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "principal", nil })

	draft := &models.Draft{
		ID:            "draft-1",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Title:         "Agent Draft",
		Slug:          "agent-draft",
		Content:       "# Agent Draft\n\nBody.",
		ContentFormat: "markdown",
		Status:        draftStatusDraft,
		GeneratedBy:   " https://example.com/users/agent-0 ",
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))
	_, err := svc.ShareDraftForReview(context.Background(), "alice", "draft-1", "principal")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(context.Background(), "principal", "alice", "draft-1", DraftReviewApproved, "approved")
	require.NoError(t, err)

	article, err := svc.PublishDraft(context.Background(), "alice", "draft-1")
	require.NoError(t, err)
	require.NotNil(t, article)
	require.Equal(t, "https://example.com/articles/agent-draft", article.ID)
	require.Equal(t, "agent-draft", article.Slug)
	require.Equal(t, "https://example.com/users/agent-0", article.GeneratedBy)
	require.Equal(t, "principal", article.ReviewedBy)
	require.Equal(t, "https://example.com/users/alice", article.PublishedBy)
}

func TestM4DraftPublishRejectsCanonicalArticleSlugChangeSafely(t *testing.T) {
	t.Parallel()

	db, q := newCMSMockDB(t)
	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()

	articleID := "https://example.com/articles/original"
	articleRepo := &fakeArticleRepo{
		db: db,
		articles: map[string]*models.Article{
			articleID: {
				Object: models.Object{
					ID:           articleID,
					Type:         activitypub.ArticleType,
					Name:         "Original",
					Content:      "before",
					AttributedTo: "https://example.com/users/alice",
					Published:    time.Now(),
				},
				Slug:          "original",
				ContentFormat: "markdown",
			},
		},
	}
	articleSvc := NewArticleService(articleRepo, fakeActorRepo{err: errors.New("no actor")}, nil, nil, nil, &fakeFederation{}, zap.NewNop())
	draftRepo := newMemDraftRepo()
	draftSvc := &DraftService{
		draftRepo:      draftRepo,
		articleService: articleSvc,
		domain:         "example.com",
		logger:         zap.NewNop(),
	}

	draft := &models.Draft{
		ID:            "draft-1",
		AuthorID:      "alice",
		ObjectID:      &articleID,
		ContentType:   activitypub.ArticleType,
		Title:         "Original renamed",
		Slug:          "renamed",
		Content:       "after",
		ContentFormat: "markdown",
		Status:        draftStatusDraft,
	}
	require.NoError(t, draftRepo.CreateDraft(context.Background(), draft))

	article, err := draftSvc.PublishDraft(context.Background(), "alice", "draft-1")
	require.Error(t, err)
	require.Nil(t, article)
	require.Contains(t, err.Error(), "immutable")

	afterDraft, getErr := draftRepo.GetDraft(context.Background(), "alice", "draft-1")
	require.NoError(t, getErr)
	require.Equal(t, draftStatusFailed, afterDraft.Status)

	stored, getArticleErr := articleRepo.GetArticle(context.Background(), articleID)
	require.NoError(t, getArticleErr)
	require.Equal(t, "Original", stored.Name)
	require.Equal(t, "before", stored.Content)
	require.Equal(t, "original", stored.Slug)
}

func TestM4ArticleUpdateRecordsRevisionAttributionAndFederatesUpdate(t *testing.T) {
	t.Parallel()

	db, q := newCMSMockDB(t)
	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()

	articleID := "https://example.com/articles/hello"
	published := time.Now().Add(-time.Hour)
	existing := &models.Article{
		Object: models.Object{
			ID:           articleID,
			Type:         activitypub.ArticleType,
			Name:         "Before",
			Content:      "# Before\n\nBody.",
			AttributedTo: "https://example.com/users/alice",
			Published:    published,
			Updated:      published,
			CreatedAt:    published,
			To:           []string{activitypub.PublicAddress},
		},
		Slug:          "hello",
		ContentFormat: "markdown",
		GeneratedBy:   "https://example.com/users/agent-0",
		ReviewedBy:    "https://example.com/users/alice",
		PublishedBy:   "https://example.com/users/alice",
		UpdatedAt:     published,
	}

	articleRepo := &fakeArticleRepo{
		db:       db,
		articles: map[string]*models.Article{articleID: existing},
	}
	revisionRepo := newMemRevisionRepo()
	revisionSvc := NewRevisionService(revisionRepo, articleRepo, nil, nil, 0, zap.NewNop())
	fed := &fakeFederation{}
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: "Person",
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
	}
	svc := NewArticleService(articleRepo, fakeActorRepo{actor: actor}, nil, nil, revisionSvc, fed, zap.NewNop())

	updated := cloneArticle(existing)
	updated.Name = "After"
	updated.Content = "# After\n\nBody."
	updated.UpdatedAt = time.Now()
	updated.Updated = updated.UpdatedAt

	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, fakeClaims{username: "alice"})
	require.NoError(t, svc.UpdateArticle(ctx, updated))

	require.Len(t, revisionRepo.created, 1)
	rev := revisionRepo.created[0]
	require.Equal(t, articleID, rev.ObjectID)
	require.Equal(t, "alice", rev.ChangedBy)
	require.Equal(t, revisionChangeTypeUpdate, rev.ChangeType)
	require.Equal(t, "https://example.com/users/agent-0", rev.GeneratedBy)
	require.Equal(t, "https://example.com/users/alice", rev.ReviewedBy)
	require.Equal(t, "https://example.com/users/alice", rev.PublishedBy)
	require.Contains(t, rev.MetadataJSON, `"generatedBy":"https://example.com/users/agent-0"`)
	require.Contains(t, rev.MetadataJSON, `"reviewedBy":"https://example.com/users/alice"`)
	require.Contains(t, rev.MetadataJSON, `"publishedBy":"https://example.com/users/alice"`)

	assert.Eventually(t, func() bool {
		fed.mu.Lock()
		defer fed.mu.Unlock()
		return len(fed.recipientActivities) > 0 && fed.recipientActivities[0].Type == activitypub.UpdateType
	}, 2*time.Second, 10*time.Millisecond)
}
