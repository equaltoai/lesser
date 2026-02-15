package handlers

import (
	"context"
	"errors"
	"testing"

	pkgmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandler_accountCountHelpers_round28_more_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	relRepo := pkgmocks.NewMockRelationshipRepository()
	relRepo.On("CountFollowers", mock.Anything, "alice").Return(5, nil)
	relRepo.On("CountFollowing", mock.Anything, "alice").Return(2, nil)
	relRepo.On("CountFollowers", mock.Anything, "err").Return(0, errors.New("boom"))
	relRepo.On("CountFollowing", mock.Anything, "err").Return(0, errors.New("boom"))

	objectRepo := pkgmocks.NewMockObjectRepository()
	objectRepo.On("GetUserStatusCount", mock.Anything, "alice").Return(7, nil)
	objectRepo.On("GetUserStatusCount", mock.Anything, "err").Return(0, errors.New("boom"))

	repos := &MockRepositoryStorage{}
	repos.On("Relationship").Return(relRepo).Maybe()
	repos.On("Object").Return(objectRepo).Maybe()

	h := &Handler{
		repos:  repos,
		logger: zap.NewNop(),
	}

	assert.Equal(t, 5, h.getAccountFollowersCountLift(ctx, "alice"))
	assert.Equal(t, 2, h.getAccountFollowingCountLift(ctx, "alice"))
	assert.Equal(t, 7, h.getAccountStatusesCountLift(ctx, "alice"))
	assert.Equal(t, 0, h.getAccountFollowersCountLift(ctx, "err"))
	assert.Equal(t, 0, h.getAccountFollowingCountLift(ctx, "err"))
	assert.Equal(t, 0, h.getAccountStatusesCountLift(ctx, "err"))
}
