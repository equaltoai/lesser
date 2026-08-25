package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

type draftMediaCASRecordingDynamo struct {
	*fakedb.Fake
	updateInputs []*dynamodb.UpdateItemInput
}

func (d *draftMediaCASRecordingDynamo) UpdateItem(ctx context.Context, input *dynamodb.UpdateItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	d.updateInputs = append(d.updateInputs, input)
	return d.Fake.UpdateItem(ctx, input, opts...)
}

func casDraftFixture(id string) *models.Draft {
	return &models.Draft{
		AuthorID:      "owner",
		ID:            id,
		ContentType:   "Article",
		Title:         "title",
		Slug:          "slug",
		Content:       "body",
		ContentFormat: "markdown",
		Status:        "draft",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
}

func TestDraftRepositoryUpdateDraftEditorialMediaCASBumpsVersion(t *testing.T) {
	ctx := context.Background()
	client := &draftMediaCASRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Draft{}))
	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)

	persisted := casDraftFixture("draft-cas")
	require.NoError(t, persisted.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(persisted).Create())

	// First media-set on a pre-version row succeeds (migration-safe) and
	// stamps version 1.
	first := casDraftFixture("draft-cas")
	first.EditorialMedia = []models.DraftMediaUsage{{MediaID: "m1", Role: models.EditorialMediaRoleHero}}
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", first))
	require.Equal(t, 1, first.ModelVersion)

	got, err := repo.GetDraft(ctx, "owner", "draft-cas")
	require.NoError(t, err)
	require.Equal(t, 1, got.ModelVersion)
	require.Len(t, got.EditorialMedia, 1)

	// The real expression carries the migration-safe OR disjunct.
	input := client.updateInputs[len(client.updateInputs)-1]
	condition := aws.ToString(input.ConditionExpression)
	require.Contains(t, condition, "attribute_not_exists")
	require.Contains(t, condition, "OR")

	// A current media-set bumps to 2.
	next := casDraftFixture("draft-cas")
	next.ModelVersion = 1
	next.EditorialMedia = []models.DraftMediaUsage{{MediaID: "m2", Role: models.EditorialMediaRoleHero}}
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", next))
	got, err = repo.GetDraft(ctx, "owner", "draft-cas")
	require.NoError(t, err)
	require.Equal(t, 2, got.ModelVersion)
	require.Equal(t, "m2", got.EditorialMedia[0].MediaID)
}

func TestDraftRepositoryUpdateDraftEditorialMediaStaleVersionConflicts(t *testing.T) {
	ctx := context.Background()
	client := &draftMediaCASRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Draft{}))
	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)

	persisted := casDraftFixture("draft-cas")
	require.NoError(t, persisted.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(persisted).Create())

	// Two concurrent media-sets read version 0.
	first := casDraftFixture("draft-cas")
	first.EditorialMedia = []models.DraftMediaUsage{{MediaID: "alice-media", Role: models.EditorialMediaRoleHero}}
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", first)) // wins, bumps to 1

	// The second writer's snapshot is stale (version 0): it must conflict, not
	// silently lose its update — this is the setDraftEditorialMedia lost-update
	// seam (M4 fold-in).
	stale := casDraftFixture("draft-cas")
	stale.EditorialMedia = []models.DraftMediaUsage{{MediaID: "bob-media", Role: models.EditorialMediaRoleHero}}
	err = repo.UpdateDraftEditorialMedia(ctx, "owner", stale)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeConflict), "stale media-set surfaces CONFLICT: %v", err)
	require.ErrorIs(t, err, storage.ErrVersionConflict)

	got, err := repo.GetDraft(ctx, "owner", "draft-cas")
	require.NoError(t, err)
	require.Equal(t, "alice-media", got.EditorialMedia[0].MediaID, "the losing write must not land")
	require.Equal(t, 1, got.ModelVersion)
}

func TestDraftRepositoryUpdateDraftEditorialMediaMigratesPreVersionRow(t *testing.T) {
	ctx := context.Background()
	client := &draftMediaCASRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Draft{}))
	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)

	// A pre-M4 row (no version attribute) is simulated by removing the
	// attribute through a field-scoped builder after creation.
	persisted := casDraftFixture("draft-cas")
	require.NoError(t, persisted.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(persisted).Create())
	remove := db.WithContext(ctx).
		Model(&models.Draft{}).
		Where("PK", "=", "USER#owner#DRAFT").
		Where("SK", "=", "ID#draft-cas").
		UpdateBuilder()
	remove.Remove("ModelVersion")
	remove.ConditionExists("PK")
	require.NoError(t, remove.Execute())

	first := casDraftFixture("draft-cas")
	first.EditorialMedia = []models.DraftMediaUsage{{MediaID: "m1", Role: models.EditorialMediaRoleHero}}
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", first))
	require.Equal(t, 1, first.ModelVersion)

	got, err := repo.GetDraft(ctx, "owner", "draft-cas")
	require.NoError(t, err)
	require.Equal(t, 1, got.ModelVersion)

	// Once versioned, a stale write fails closed.
	stale := casDraftFixture("draft-cas")
	err = repo.UpdateDraftEditorialMedia(ctx, "owner", stale)
	require.ErrorIs(t, err, storage.ErrVersionConflict)
}

func TestDraftRepositoryUpdateDraftContentPreservesVersion(t *testing.T) {
	ctx := context.Background()
	client := &draftMediaCASRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Draft{}))
	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)

	persisted := casDraftFixture("draft-cas")
	require.NoError(t, persisted.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(persisted).Create())

	// Bump the version through the media lane.
	first := casDraftFixture("draft-cas")
	first.EditorialMedia = []models.DraftMediaUsage{{MediaID: "m1", Role: models.EditorialMediaRoleHero}}
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", first))

	// A full-model content write round-trips the version read from GetDraft and
	// must not reset it.
	fetched, err := repo.GetDraft(ctx, "owner", "draft-cas")
	require.NoError(t, err)
	require.Equal(t, 1, fetched.ModelVersion)
	fetched.Content = "edited body"
	require.NoError(t, repo.UpdateDraft(ctx, "owner", fetched))

	got, err := repo.GetDraft(ctx, "owner", "draft-cas")
	require.NoError(t, err)
	require.Equal(t, 1, got.ModelVersion, "content writes never reset the CAS version")
	require.Equal(t, "edited body", got.Content)
	require.Len(t, got.EditorialMedia, 1, "content writes never clobber the media association")

	// The media lane still CASes against the preserved version.
	next := casDraftFixture("draft-cas")
	next.ModelVersion = 1
	next.EditorialMedia = []models.DraftMediaUsage{{MediaID: "m2", Role: models.EditorialMediaRoleHero}}
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", next))
}
