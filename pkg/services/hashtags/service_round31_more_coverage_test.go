package hashtags

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_getHashtagInfoWithFallback_ReturnsNilOnNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := new(mockHashtagRepository)
	service := NewService(repo, nil, nil, nil, zap.NewNop())

	repo.On("GetHashtagInfo", mock.Anything, "missing").Return((*storage.Hashtag)(nil), storage.ErrNotFound).Once()
	info, err := service.getHashtagInfoWithFallback(ctx, "missing")
	require.NoError(t, err)
	require.Nil(t, info)

	repo.On("GetHashtagInfo", mock.Anything, "missing2").Return((*storage.Hashtag)(nil), errors.New("not found")).Once()
	info, err = service.getHashtagInfoWithFallback(ctx, "missing2")
	require.NoError(t, err)
	require.Nil(t, info)
}

func TestService_getHashtagInfoWithFallback_ReturnsErrGetHashtagOnOtherErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := new(mockHashtagRepository)
	service := NewService(repo, nil, nil, nil, zap.NewNop())

	repo.On("GetHashtagInfo", mock.Anything, "boom").Return((*storage.Hashtag)(nil), errors.New("boom")).Once()
	info, err := service.getHashtagInfoWithFallback(ctx, "boom")
	require.Error(t, err)
	require.Nil(t, info)
	require.ErrorIs(t, err, ErrGetHashtag)
}

func TestService_loadHashtagStats_HandlesNilErrorAndUnexpectedTypes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := new(mockHashtagRepository)
	service := NewService(repo, nil, nil, nil, zap.NewNop())

	repo.On("GetHashtagStats", mock.Anything, "tag").Return((*storage.HashtagStats)(nil), errors.New("boom")).Once()
	require.Nil(t, service.loadHashtagStats(ctx, "tag"))

	repo.On("GetHashtagStats", mock.Anything, "tag2").Return(nil, nil).Once()
	require.Nil(t, service.loadHashtagStats(ctx, "tag2"))

	stats := &storage.HashtagStats{Name: "tag3", UsageCount: 10}
	repo.On("GetHashtagStats", mock.Anything, "tag3").Return(stats, nil).Once()
	require.Same(t, stats, service.loadHashtagStats(ctx, "tag3"))

	repo.On("GetHashtagStats", mock.Anything, "tag4").Return(map[string]any{"name": "tag4"}, nil).Once()
	require.Nil(t, service.loadHashtagStats(ctx, "tag4"))
}

func TestService_relatedHashtags_SortsByFrequency(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := new(mockHashtagRepository)
	service := NewService(repo, nil, nil, nil, zap.NewNop())

	repo.On("GetHashtagTimelineAdvanced", mock.Anything, "golang", (*string)(nil), defaultRelatedHashtagSampleSize, "").Return([]*storage.StatusSearchResult{
		{Content: "#GoLang #testing #go"},
		{Content: "Learning #golang with #testing"},
		{Content: "No tags here"},
		{Content: "#dev #testing #dev"},
	}, nil).Once()

	out := service.relatedHashtags(ctx, "golang", 2)
	require.Len(t, out, 2)
	assert.Equal(t, "testing", out[0])
}

func TestService_relatedHashtags_ReturnsEmptyOnInvalidInputOrRepoError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := new(mockHashtagRepository)
	service := NewService(repo, nil, nil, nil, zap.NewNop())

	require.Empty(t, service.relatedHashtags(ctx, "golang", 0))

	repo.On("GetHashtagTimelineAdvanced", mock.Anything, "golang", (*string)(nil), defaultRelatedHashtagSampleSize, "").Return([]*storage.StatusSearchResult{}, errors.New("boom")).Once()
	require.Empty(t, service.relatedHashtags(ctx, "golang", 5))
}

func TestCloneNotificationSettings_DeepCopiesAndEnforcesIdentity(t *testing.T) {
	t.Parallel()

	settings := &storage.HashtagNotificationSettings{
		UserID:  "wrong",
		Hashtag: "wrong",
		Level:   "all",
		Filters: []*storage.NotificationFilter{
			{Types: []string{"mention"}},
			nil,
		},
		Metadata: map[string]interface{}{"k": "v"},
	}

	clone := cloneNotificationSettings(settings, "alice", "golang")
	require.NotNil(t, clone)
	require.NotSame(t, settings, clone)
	require.Equal(t, "alice", clone.UserID)
	require.Equal(t, "golang", clone.Hashtag)

	require.Equal(t, "v", clone.Metadata["k"])
	settings.Metadata["k"] = "changed"
	require.Equal(t, "v", clone.Metadata["k"])

	require.Len(t, clone.Filters, 2)
	require.NotNil(t, clone.Filters[0])
	require.NotSame(t, settings.Filters[0], clone.Filters[0])
	require.Equal(t, []string{"mention"}, clone.Filters[0].Types)
	require.Nil(t, clone.Filters[1])
}

func TestExtractHashtagsFromContent_NormalizesAndTrimsPunctuation(t *testing.T) {
	t.Parallel()

	out := extractHashtagsFromContent("hello #GoLang, #testing! ##ignored #x")
	assert.Equal(t, []string{"golang", "testing", "ignored", "x"}, out)
}

func TestUniqueNormalizedHashtags_DedupesAndNormalizes(t *testing.T) {
	t.Parallel()

	out := uniqueNormalizedHashtags([]string{"#GoLang", "golang", "  #testing ", "", "#"})
	assert.Equal(t, []string{"golang", "testing"}, out)
}

func TestService_publishUserEvent_HandlesPublisherErrors(t *testing.T) {
	t.Parallel()

	publisher := new(mockPublisher)
	service := NewService(nil, nil, nil, publisher, zap.NewNop())

	publisher.On("PublishToUser", mock.Anything, "alice", mock.Anything).Return(errors.New("user publish failed")).Once()
	publisher.On("PublishToStream", mock.Anything, streaming.HashtagStreamName("golang"), mock.Anything).Return(errors.New("stream publish failed")).Once()

	service.publishUserEvent(context.Background(), streaming.HashtagFollowed, "alice", "golang")

	publisher.AssertExpectations(t)
}

func TestService_publishInternalHashtagEvent_SkipsWithoutPublisher(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, nil, nil, zap.NewNop())
	service.publishInternalHashtagEvent(streaming.ActionUpdate, "golang")
}
