package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestFeaturedTagRepository_CreateFeaturedTag_DuplicateReturnsAlreadyExists(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	tagQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(tagQuery).Once()
	tagQuery.On("Where", "PK", "=", "USER#alice").Return(tagQuery).Once()
	tagQuery.On("Where", "SK", "BEGINS_WITH", "FEATURED_TAG#").Return(tagQuery).Once()
	tagQuery.On("OrderBy", "SK", "ASC").Return(tagQuery).Once()
	tagQuery.On("Limit", 101).Return(tagQuery).Once()
	tagQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.FeaturedTag)
		*dest = []*models.FeaturedTag{
			{ID: "id1", Username: "alice", Name: "golang", LastStatusAt: "", CreatedAt: time.Now()},
		}
	}).Return(nil).Once()

	repo := NewFeaturedTagRepository(mockDB, "test-table", zap.NewNop(), nil)

	err := repo.CreateFeaturedTag(ctx, &storage.FeaturedTag{Username: "alice", Name: "#GoLang"})
	assert.ErrorIs(t, err, storage.ErrAlreadyExists)
}

func TestFeaturedTagRepository_CreateFeaturedTag_SetsFieldsAndCreatesModel(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	tagQuery := new(dynamormmocks.MockQuery)
	statusQuery := new(dynamormmocks.MockQuery)
	createQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)

	// No existing featured tags.
	mockDB.On("Model", mock.Anything).Return(tagQuery).Once()
	tagQuery.On("Where", "PK", "=", "USER#alice").Return(tagQuery).Once()
	tagQuery.On("Where", "SK", "BEGINS_WITH", "FEATURED_TAG#").Return(tagQuery).Once()
	tagQuery.On("OrderBy", "SK", "ASC").Return(tagQuery).Once()
	tagQuery.On("Limit", 101).Return(tagQuery).Once()
	tagQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.FeaturedTag)
		*dest = []*models.FeaturedTag{}
	}).Return(nil).Once()

	// Tag statistics query.
	mockDB.On("Model", mock.Anything).Return(statusQuery).Once()
	statusQuery.On("Index", "gsi3").Return(statusQuery).Once()
	statusQuery.On("Where", "gsi3PK", "=", "USER_STATUS#alice").Return(statusQuery).Once()
	statusQuery.On("OrderBy", "gsi3SK", "DESC").Return(statusQuery).Once()
	published := time.Date(2024, 12, 28, 12, 0, 0, 0, time.UTC)
	statusQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Status)
		*dest = []models.Status{
			{
				Note: &activitypub.Note{
					BaseObject: activitypub.BaseObject{Published: &published},
					Content:    "Hello #GoLang",
				},
			},
			{
				Note: &activitypub.Note{
					BaseObject: activitypub.BaseObject{Published: &published},
					Content:    "No hashtag here",
				},
			},
			{
				Note: &activitypub.Note{
					BaseObject: activitypub.BaseObject{Published: &published},
					Content:    "Also contains #golang in lower",
				},
			},
		}
	}).Return(nil).Once()

	// Create the FeaturedTag model (ValidateAndCreate -> BaseRepository.Create).
	mockDB.On("Model", mock.Anything).Return(createQuery).Once()
	createQuery.On("Create").Return(nil).Once()

	repo := NewFeaturedTagRepository(mockDB, "test-table", zap.NewNop(), nil)
	input := &storage.FeaturedTag{Username: "alice", Name: "#GoLang"}

	require.NoError(t, repo.CreateFeaturedTag(ctx, input))
	assert.NotEmpty(t, input.ID)
	assert.Equal(t, "golang", input.Name)
	assert.Contains(t, input.URL, "/tags/golang")
	assert.Equal(t, 2, input.StatusesCount)
	require.NotNil(t, input.LastStatusAt)
	assert.True(t, input.CreatedAt.After(time.Time{}))
}

func TestFeaturedTagRepository_DeleteFeaturedTag_NotFoundWhenMissing(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	tagQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(tagQuery).Once()
	tagQuery.On("Where", "PK", "=", "USER#alice").Return(tagQuery).Once()
	tagQuery.On("Where", "SK", "BEGINS_WITH", "FEATURED_TAG#").Return(tagQuery).Once()
	tagQuery.On("OrderBy", "SK", "ASC").Return(tagQuery).Once()
	tagQuery.On("Limit", 101).Return(tagQuery).Once()
	tagQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.FeaturedTag)
		*dest = []*models.FeaturedTag{}
	}).Return(nil).Once()

	repo := NewFeaturedTagRepository(mockDB, "test-table", zap.NewNop(), nil)
	assert.ErrorIs(t, repo.DeleteFeaturedTag(ctx, "alice", "golang"), storage.ErrNotFound)
}

func TestFeaturedTagRepository_DeleteFeaturedTag_MapsNotFoundDeleteError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	tagQuery := new(dynamormmocks.MockQuery)
	deleteQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)

	// GetFeaturedTags returns one tag.
	mockDB.On("Model", mock.Anything).Return(tagQuery).Once()
	tagQuery.On("Where", "PK", "=", "USER#alice").Return(tagQuery).Once()
	tagQuery.On("Where", "SK", "BEGINS_WITH", "FEATURED_TAG#").Return(tagQuery).Once()
	tagQuery.On("OrderBy", "SK", "ASC").Return(tagQuery).Once()
	tagQuery.On("Limit", 101).Return(tagQuery).Once()
	tagQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.FeaturedTag)
		*dest = []*models.FeaturedTag{
			{ID: "id1", Username: "alice", Name: "golang", CreatedAt: time.Now()},
		}
	}).Return(nil).Once()

	// BaseRepository.Delete
	mockDB.On("Model", mock.Anything).Return(deleteQuery).Once()
	deleteQuery.On("Where", "PK", "=", "USER#alice").Return(deleteQuery).Once()
	deleteQuery.On("Where", "SK", "=", "FEATURED_TAG#id1").Return(deleteQuery).Once()
	deleteQuery.On("Delete").Return(fmt.Errorf("not found")).Once()

	repo := NewFeaturedTagRepository(mockDB, "test-table", zap.NewNop(), nil)
	err := repo.DeleteFeaturedTag(ctx, "alice", "golang")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to delete featured tag")
}

func TestFeaturedTagRepository_GetFeaturedTags_ParsesLastStatusAtAndPaginates(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	tagQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(tagQuery).Once()
	tagQuery.On("Where", "PK", "=", "USER#alice").Return(tagQuery).Once()
	tagQuery.On("Where", "SK", "BEGINS_WITH", "FEATURED_TAG#").Return(tagQuery).Once()
	tagQuery.On("OrderBy", "SK", "ASC").Return(tagQuery).Once()
	tagQuery.On("Limit", 101).Return(tagQuery).Once()
	tagQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.FeaturedTag)
		*dest = []*models.FeaturedTag{
			{ID: "id1", Username: "alice", Name: "golang", LastStatusAt: "2024-12-28T12:00:00Z", CreatedAt: time.Now()},
			{ID: "id2", Username: "alice", Name: "rust", LastStatusAt: "not-a-time", CreatedAt: time.Now()},
		}
	}).Return(nil).Once()

	repo := NewFeaturedTagRepository(mockDB, "test-table", zap.NewNop(), nil)
	tags, err := repo.GetFeaturedTags(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, tags, 2)
	require.NotNil(t, tags[0].LastStatusAt)
	assert.Nil(t, tags[1].LastStatusAt)
}

func TestFeaturedTagRepository_GetFeaturedTags_SwallowsQueryErrors(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	tagQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(tagQuery).Once()
	tagQuery.On("Where", "PK", "=", "USER#alice").Return(tagQuery).Once()
	tagQuery.On("Where", "SK", "BEGINS_WITH", "FEATURED_TAG#").Return(tagQuery).Once()
	tagQuery.On("OrderBy", "SK", "ASC").Return(tagQuery).Once()
	tagQuery.On("Limit", 101).Return(tagQuery).Once()
	tagQuery.On("All", mock.Anything).Return(stdErrors.New("boom")).Once()

	repo := NewFeaturedTagRepository(mockDB, "test-table", zap.NewNop(), nil)
	tags, err := repo.GetFeaturedTags(ctx, "alice")
	require.NoError(t, err)
	assert.Empty(t, tags)
}

func TestFeaturedTagRepository_GetTagSuggestions_ExcludesFeaturedAndSorts(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	featuredQuery := new(dynamormmocks.MockQuery)
	statusQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)

	// GetFeaturedTags -> one featured tag to exclude.
	mockDB.On("Model", mock.Anything).Return(featuredQuery).Once()
	featuredQuery.On("Where", "PK", "=", "USER#alice").Return(featuredQuery).Once()
	featuredQuery.On("Where", "SK", "BEGINS_WITH", "FEATURED_TAG#").Return(featuredQuery).Once()
	featuredQuery.On("OrderBy", "SK", "ASC").Return(featuredQuery).Once()
	featuredQuery.On("Limit", 101).Return(featuredQuery).Once()
	featuredQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.FeaturedTag)
		*dest = []*models.FeaturedTag{
			{ID: "id1", Username: "alice", Name: "golang", CreatedAt: time.Now()},
		}
	}).Return(nil).Once()

	// Status scan for suggestions.
	mockDB.On("Model", mock.Anything).Return(statusQuery).Once()
	statusQuery.On("Index", "gsi3").Return(statusQuery).Once()
	statusQuery.On("Where", "gsi3PK", "=", "USER_STATUS#alice").Return(statusQuery).Once()
	statusQuery.On("OrderBy", "gsi3SK", "DESC").Return(statusQuery).Once()
	statusQuery.On("Limit", 100).Return(statusQuery).Once()
	statusQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Status)
		*dest = []models.Status{
			{Note: &activitypub.Note{Content: "Tags #rust #rust #zig"}},
			{Note: &activitypub.Note{Content: "Also #rust and #GOLANG (excluded)"}},
			{Note: &activitypub.Note{Content: "One #zig"}},
			{Note: nil},
		}
	}).Return(nil).Once()

	repo := NewFeaturedTagRepository(mockDB, "test-table", zap.NewNop(), nil)
	suggestions, err := repo.GetTagSuggestions(ctx, "alice", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"rust", "zig"}, suggestions)
}

func TestFeaturedTagRepository_GetTagSuggestions_AllowsNotFoundStatuses(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	featuredQuery := new(dynamormmocks.MockQuery)
	statusQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)

	mockDB.On("Model", mock.Anything).Return(featuredQuery).Once()
	featuredQuery.On("Where", "PK", "=", "USER#alice").Return(featuredQuery).Once()
	featuredQuery.On("Where", "SK", "BEGINS_WITH", "FEATURED_TAG#").Return(featuredQuery).Once()
	featuredQuery.On("OrderBy", "SK", "ASC").Return(featuredQuery).Once()
	featuredQuery.On("Limit", 101).Return(featuredQuery).Once()
	featuredQuery.On("All", mock.Anything).Return(nil).Once()

	mockDB.On("Model", mock.Anything).Return(statusQuery).Once()
	statusQuery.On("Index", "gsi3").Return(statusQuery).Once()
	statusQuery.On("Where", "gsi3PK", "=", "USER_STATUS#alice").Return(statusQuery).Once()
	statusQuery.On("OrderBy", "gsi3SK", "DESC").Return(statusQuery).Once()
	statusQuery.On("Limit", 100).Return(statusQuery).Once()
	statusQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewFeaturedTagRepository(mockDB, "test-table", zap.NewNop(), nil)
	suggestions, err := repo.GetTagSuggestions(ctx, "alice", 3)
	require.NoError(t, err)
	assert.Empty(t, suggestions)
}
