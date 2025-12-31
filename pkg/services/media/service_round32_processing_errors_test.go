package media

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestService_MarkMediaProcessed_ErrorPaths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mediaID := "media-err"

	t.Run("get media error returns retrieval failed", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)

		mediaRepo.On("GetMedia", ctx, mediaID).Return(&models.Media{}, errors.New("boom")).Once()
		err := service.MarkMediaProcessed(ctx, mediaID, map[string]models.MediaVariant{})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrMediaRetrievalFailed)
	})

	t.Run("update error returns update failed", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		testMedia := createTestMedia()

		mediaRepo.On("GetMedia", ctx, mediaID).Return(testMedia, nil).Once()
		mediaRepo.On("UpdateMedia", ctx, testMedia).Return(errors.New("boom")).Once()

		err := service.MarkMediaProcessed(ctx, mediaID, map[string]models.MediaVariant{})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrMediaUpdateFailed)
	})
}

func TestService_MarkMediaFailed_ErrorPaths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mediaID := "media-err"

	t.Run("get media error returns retrieval failed", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)

		mediaRepo.On("GetMedia", ctx, mediaID).Return(&models.Media{}, errors.New("boom")).Once()
		err := service.MarkMediaFailed(ctx, mediaID, "failed")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrMediaRetrievalFailed)
	})

	t.Run("update error returns update failed", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		testMedia := createTestMedia()

		mediaRepo.On("GetMedia", ctx, mediaID).Return(testMedia, nil).Once()
		mediaRepo.On("UpdateMedia", ctx, testMedia).Return(errors.New("boom")).Once()

		err := service.MarkMediaFailed(ctx, mediaID, "failed")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrMediaUpdateFailed)
	})
}
