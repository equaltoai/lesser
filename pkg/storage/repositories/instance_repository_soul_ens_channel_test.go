package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestInstanceRepository_GetSoulENSChannel_ReturnsNilWhenMissing(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceSoulENSChannel")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.GetSoulENSChannel(ctx, "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab")
	require.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestInstanceRepository_SetSoulENSChannel_NormalizesAndCreatesWhenMissing(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("Update", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg := &models.InstanceSoulENSChannel{
		AgentID:         " 0X8DB124B1D48E366002DB4E61CC1501EEB8561E1EF06FD6F9ABF9F984501D13AB ",
		Name:            " Agent-Alice.Lesserlab.ETH ",
		ResolverAddress: " 0x000000000000000000000000000000000000cAFe ",
		Chain:           " Sepolia ",
	}

	require.NoError(t, repo.SetSoulENSChannel(ctx, cfg))
	assert.Equal(t, "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab", cfg.AgentID)
	assert.Equal(t, "agent-alice.lesserlab.eth", cfg.Name)
	assert.Equal(t, "sepolia", cfg.Chain)
	assert.Equal(t, "0x000000000000000000000000000000000000cAFe", cfg.ResolverAddress)
	assert.Equal(t, models.SoulENSChannelSortKey(cfg.AgentID), cfg.SK)
}
