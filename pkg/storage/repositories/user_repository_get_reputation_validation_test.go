package repositories

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestUserRepository_GetReputation_InvalidActorID(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	rep, err := repo.GetReputation(context.Background(), "")
	assert.Error(t, err)
	assert.Nil(t, rep)
}

func TestUserRepository_GetReputation_InvalidUsernameFromActorID(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	rep, err := repo.GetReputation(context.Background(), "not-a-url")
	assert.Error(t, err)
	assert.Nil(t, rep)
}
