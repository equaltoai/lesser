package main

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoadActorProfile_Round25_HelperFallbacks(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})

	t.Run("falls back to normalized actor repo when account load fails", func(t *testing.T) {
		cfg = &config.Config{Domain: "example.com"}
		logger = nil

		h := &Handler{
			accountRepo: &fakeAccountRepo{err: errors.New("account repo down")},
			actorRepo:   &fakeActorRepo{actor: &activitypub.Actor{}},
		}

		actor, err := h.loadActorProfile(context.Background(), "alice")
		require.NoError(t, err)
		require.NotNil(t, actor)
		require.Equal(t, cfg.ActorURL("alice"), actor.ID)
		require.Equal(t, activitypub.PersonType, actor.Type)
	})

	t.Run("prefers account error when fallback only reports not found", func(t *testing.T) {
		cfg = &config.Config{Domain: "example.com"}
		logger = zap.NewNop()
		wantErr := errors.New("account repo down")

		h := &Handler{
			accountRepo: &fakeAccountRepo{err: wantErr},
			actorRepo:   &fakeActorRepo{err: common.ActorNotFoundError{Username: "alice"}},
		}

		actor, err := h.loadActorProfile(context.Background(), "alice")
		require.Nil(t, actor)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("returns actor not found when no repositories are available", func(t *testing.T) {
		cfg = &config.Config{Domain: "example.com"}
		logger = zap.NewNop()

		actor, err := (&Handler{}).loadActorProfile(context.Background(), "alice")
		require.Nil(t, actor)
		require.Error(t, err)
		require.True(t, common.IsNotFound(err))
	})

	t.Run("helper methods stay safe with nil inputs", func(t *testing.T) {
		cfg = nil
		logger = zap.NewNop()

		h := &Handler{}
		require.Nil(t, h.hydrateAccountActor("alice", nil))
		require.Equal(t, "", h.actorBaseURL())
	})
}
