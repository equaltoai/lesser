package handlers

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestHandler_MigrateInstanceFeatureConfigFromEnv_UsesBackgroundContext(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", "")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "")
	t.Setenv("TRANSLATION_ENABLED", "")
	t.Setenv("TIP_ENABLED", "")
	t.Setenv("TIP_CHAIN_ID", "")
	t.Setenv("TIP_CONTRACT_ADDRESS", "")

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(dynamormerrors.ErrItemNotFound).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Return(dynamormerrors.ErrItemNotFound).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(dynamormerrors.ErrItemNotFound).Maybe()

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	repos := &MockRepositoryStorage{}
	repos.On("Instance").Return(instanceRepo).Maybe()

	h := &Handler{
		repos:  repos,
		logger: zap.NewNop(),
	}

	require.NotPanics(t, func() {
		h.MigrateInstanceFeatureConfigFromEnv(nil)
	})
}

func TestHandler_MigrateInstanceFeatureConfigFromEnv_NoOpsWhenReposMissing(t *testing.T) {
	h := &Handler{logger: zap.NewNop()}
	require.NotPanics(t, func() {
		h.MigrateInstanceFeatureConfigFromEnv(nil)
	})
}
