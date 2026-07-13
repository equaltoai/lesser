package cms

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
)

func TestCMS_Round25_cmsHostFromURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", cmsHostFromURL(""))
	assert.Equal(t, "", cmsHostFromURL("not a url"))
	assert.Equal(t, "example.com", cmsHostFromURL("https://example.com/articles/123"))
	assert.Equal(t, "example.com", cmsHostFromURL("https://Example.COM/articles/123"))
	assert.Equal(t, "example.com", cmsNormalizeTenant(" Example.COM "))
}

func TestCMS_Round25_cmsEnsureSlugIndex(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("create succeeds returns created", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		q.On("Create").Return(nil).Once()

		created, err := cmsEnsureSlugIndex(ctx, db, "PK#1", "slug", "target", "item")
		require.NoError(t, err)
		assert.True(t, created)
	})

	t.Run("condition failed and existing matches returns not created", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		q.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
		q.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0)
			idx := dest.(*models.CMSSlugIndex)
			idx.TargetID = "target"
		}).Return(nil).Once()

		created, err := cmsEnsureSlugIndex(ctx, db, "PK#2", "slug", "target", "item")
		require.NoError(t, err)
		assert.False(t, created)
	})

	t.Run("condition failed and existing differs returns already exists", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		q.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
		q.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0)
			idx := dest.(*models.CMSSlugIndex)
			idx.TargetID = "different"
		}).Return(nil).Once()

		created, err := cmsEnsureSlugIndex(ctx, db, "PK#3", "slug", "target", "item")
		require.Error(t, err)
		assert.False(t, created)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeAlreadyExists))
	})

	t.Run("non-condition error returns original error", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		q.On("Create").Return(errors.New("boom")).Once()

		created, err := cmsEnsureSlugIndex(ctx, db, "PK#4", "slug", "target", "item")
		require.Error(t, err)
		assert.False(t, created)
	})

	t.Run("condition failed and lookup fails returns create error", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		q.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
		q.On("First", mock.Anything).Return(errors.New("read failed")).Once()

		created, err := cmsEnsureSlugIndex(ctx, db, "PK#5", "slug", "target", "item")
		require.Error(t, err)
		assert.False(t, created)
		assert.True(t, dynamormerrors.IsConditionFailed(err))
	})
}

func TestCMS_Round25_cmsDeleteSlugIndex(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("empty pk no-ops", func(t *testing.T) {
		db, _ := newCMSMockDB(t)
		cmsDeleteSlugIndex(ctx, db, "")
	})

	t.Run("valid pk issues delete", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		q.On("Delete").Return(nil).Once()
		cmsDeleteSlugIndex(ctx, db, "PK#1")
	})
}
