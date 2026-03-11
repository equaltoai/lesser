package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type accountsRound20EnsuringActorRepo struct {
	interfaces.ActorRepository
	usernames []string
}

func (r *accountsRound20EnsuringActorRepo) EnsureNumericIDMapping(_ context.Context, username string) error {
	r.usernames = append(r.usernames, username)
	return nil
}

type accountsRound20PlainActorRepo struct {
	interfaces.ActorRepository
}

type accountsRound20FailingEnsuringActorRepo struct {
	interfaces.ActorRepository
	usernames []string
}

func (r *accountsRound20FailingEnsuringActorRepo) EnsureNumericIDMapping(_ context.Context, username string) error {
	r.usernames = append(r.usernames, username)
	return errors.New("boom")
}

type accountsRound20LookupActorRepo struct {
	interfaces.ActorRepository
	actor    *activitypub.Actor
	err      error
	username string
}

func (r *accountsRound20LookupActorRepo) GetActor(_ context.Context, username string) (*activitypub.Actor, error) {
	r.username = username
	return r.actor, r.err
}

func TestAccountsRound20_PublicAccountFallbackHelpers(t *testing.T) {
	cfg := round10TestConfig()
	h := &Handler{cfg: cfg}

	t.Run("public account helpers handle nil inputs", func(t *testing.T) {
		require.Empty(t, h.publicAccountFromStorageAccount(nil))
		require.Empty(t, h.publicAccountFromActor(context.Background(), nil))
	})

	t.Run("publicAccountFromStorageAccount falls back to actor conversion when user is missing", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/alice", Type: "Person"},
			PreferredUsername: "alice@example.net",
			Name:              "Alice",
			URL:               "https://remote.example/@alice",
		}

		account := h.publicAccountFromStorageAccount(&storage.Account{Actor: actor})
		require.Equal(t, actor.PreferredUsername, account.Username)
		require.Equal(t, actor.Name, account.DisplayName)
		require.Equal(t, actor.URL, account.URL)
		require.NotNil(t, account.Emojis)
		require.NotNil(t, account.Fields)
	})

	t.Run("publicAccountFromActor synthesizes local storage account data", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
			PreferredUsername: "alice",
			Name:              "Alice",
		}

		account := h.publicAccountFromActor(context.Background(), actor)
		require.Equal(t, "alice", account.Username)
		require.Equal(t, "Alice", account.DisplayName)
		require.Equal(t, cfg.BaseURL()+"/@alice", account.URL)
		require.NotEmpty(t, account.ID)
		require.NotNil(t, account.Emojis)
		require.NotNil(t, account.Fields)
	})

	t.Run("publicAccountFromActor preserves remote actor fallback", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/bob", Type: "Person"},
			PreferredUsername: "bob@example.net",
			Name:              "Bob",
			URL:               "https://remote.example/@bob",
		}

		account := h.publicAccountFromActor(context.Background(), actor)
		require.Equal(t, actor.PreferredUsername, account.Username)
		require.Equal(t, actor.Name, account.DisplayName)
		require.Equal(t, actor.URL, account.URL)
		require.NotNil(t, account.Emojis)
		require.NotNil(t, account.Fields)
	})
}

func TestAccountsRound20_AccountResolutionHelpers(t *testing.T) {
	t.Run("storageAccountFromActor handles nil empty and valid actors", func(t *testing.T) {
		require.Nil(t, storageAccountFromActor(nil))
		require.Nil(t, storageAccountFromActor(&activitypub.Actor{PreferredUsername: "   "}))

		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
			PreferredUsername: "alice",
			Name:              "Alice",
		}

		account := storageAccountFromActor(actor)
		require.NotNil(t, account)
		require.Equal(t, "alice", account.User.Username)
		require.Equal(t, "Alice", account.User.DisplayName)
		require.Same(t, actor, account.Actor)
	})

	t.Run("shouldFallbackAccountResolution recognizes public id shapes", func(t *testing.T) {
		require.False(t, shouldFallbackAccountResolution(""))
		require.False(t, shouldFallbackAccountResolution("alice"))
		require.True(t, shouldFallbackAccountResolution("1234567890"))
		require.True(t, shouldFallbackAccountResolution("https://example.com/users/alice"))
		require.True(t, shouldFallbackAccountResolution("alice@example.com"))
	})

	t.Run("actorAppearsLocal honors base url and username style", func(t *testing.T) {
		cfg := round10TestConfig()
		h := &Handler{cfg: cfg}

		require.False(t, h.actorAppearsLocal(nil))
		require.True(t, h.actorAppearsLocal(&activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
			PreferredUsername: "alice@example.com",
		}))
		require.True(t, h.actorAppearsLocal(&activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/alice", Type: "Person"},
			PreferredUsername: "alice",
		}))
		require.False(t, h.actorAppearsLocal(&activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/alice", Type: "Person"},
			PreferredUsername: "alice@example.com",
		}))
	})

	t.Run("ensureLocalNumericIDMapping uses actor repo opt-in helper", func(t *testing.T) {
		repos := &MockRepositoryStorage{}
		actorRepo := &accountsRound20EnsuringActorRepo{}
		repos.On("Actor").Return(actorRepo).Once()

		h := &Handler{repos: repos, logger: zap.NewNop()}
		h.ensureLocalNumericIDMapping(context.Background(), "alice")

		require.Equal(t, []string{"alice"}, actorRepo.usernames)
		repos.AssertExpectations(t)
	})

	t.Run("ensureLocalNumericIDMapping ignores blank usernames", func(t *testing.T) {
		repos := &MockRepositoryStorage{}
		h := &Handler{repos: repos, logger: zap.NewNop()}
		h.ensureLocalNumericIDMapping(context.Background(), "   ")
		repos.AssertExpectations(t)
	})

	t.Run("ensureLocalNumericIDMapping tolerates nil handlers", func(t *testing.T) {
		var h *Handler
		h.ensureLocalNumericIDMapping(context.Background(), "alice")
	})

	t.Run("ensureLocalNumericIDMapping stops when actor repo is unavailable", func(t *testing.T) {
		repos := &MockRepositoryStorage{}
		repos.On("Actor").Return(nil).Once()

		h := &Handler{repos: repos, logger: zap.NewNop()}
		h.ensureLocalNumericIDMapping(context.Background(), "alice")
		repos.AssertExpectations(t)
	})

	t.Run("ensureLocalNumericIDMapping ignores actor repos without opt-in helper", func(t *testing.T) {
		repos := &MockRepositoryStorage{}
		repos.On("Actor").Return(&accountsRound20PlainActorRepo{}).Once()

		h := &Handler{repos: repos, logger: zap.NewNop()}
		h.ensureLocalNumericIDMapping(context.Background(), "alice")
		repos.AssertExpectations(t)
	})

	t.Run("ensureLocalNumericIDMapping tolerates helper errors", func(t *testing.T) {
		repos := &MockRepositoryStorage{}
		actorRepo := &accountsRound20FailingEnsuringActorRepo{}
		repos.On("Actor").Return(actorRepo).Once()

		h := &Handler{repos: repos, logger: zap.NewNop()}
		h.ensureLocalNumericIDMapping(context.Background(), "alice")

		require.Equal(t, []string{"alice"}, actorRepo.usernames)
		repos.AssertExpectations(t)
	})

	t.Run("localStorageAccountForActor uses registry account and backfills mapping", func(t *testing.T) {
		cfg := round10TestConfig()
		repos := &MockRepositoryStorage{}
		actorRepo := &accountsRound20EnsuringActorRepo{}
		repos.On("Actor").Return(actorRepo).Once()

		h := &Handler{
			cfg:    cfg,
			repos:  repos,
			logger: zap.NewNop(),
			registry: &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
						require.Equal(t, "alice", username)
						return &storage.Account{User: &storage.User{Username: "alice"}}, nil
					},
				},
			},
		}

		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
			PreferredUsername: "alice",
			Name:              "Alice",
		}

		account, err := h.localStorageAccountForActor(context.Background(), actor)
		require.NoError(t, err)
		require.NotNil(t, account)
		require.Same(t, actor, account.Actor)
		require.Equal(t, []string{"alice"}, actorRepo.usernames)
		repos.AssertExpectations(t)
	})

	t.Run("localStorageAccountForActor ignores local actors without usernames", func(t *testing.T) {
		cfg := round10TestConfig()
		h := &Handler{cfg: cfg}

		account, err := h.localStorageAccountForActor(context.Background(), &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/missing", Type: "Person"},
			PreferredUsername: "   ",
		})

		require.NoError(t, err)
		require.Nil(t, account)
	})

	t.Run("localStorageAccountForActor fills missing repository actors from the resolved actor", func(t *testing.T) {
		cfg := round10TestConfig()
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"alice": {
					Username:     "alice",
					DisplayName:  "Alice",
					CreatedAt:    time.Now().Add(-time.Hour),
					UpdatedAt:    time.Now().Add(-time.Hour),
					Discoverable: true,
					Role:         "user",
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		h.registry = nil

		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
			PreferredUsername: "alice",
			Name:              "Alice",
		}

		account, err := h.localStorageAccountForActor(context.Background(), actor)
		require.NoError(t, err)
		require.NotNil(t, account)
		require.NotNil(t, account.Actor)
		require.Equal(t, actor.ID, account.Actor.ID)
		require.Equal(t, "alice", account.User.Username)
	})

	t.Run("lookupStorageAccountByID falls back to repository account lookup", func(t *testing.T) {
		cfg := round10TestConfig()
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"alice": {
					Username:     "alice",
					DisplayName:  "Alice",
					CreatedAt:    time.Now().Add(-time.Hour),
					UpdatedAt:    time.Now().Add(-time.Hour),
					Discoverable: true,
					Role:         "user",
				},
			},
			actorsByUser: map[string]storagemodels.Actor{
				"alice": {
					Username: "alice",
					Actor: &activitypub.Actor{
						BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
						PreferredUsername: "alice",
						Name:              "Alice",
					},
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		account, err := h.lookupStorageAccountByID(context.Background(), "alice")
		require.NoError(t, err)
		require.NotNil(t, account)
		require.Equal(t, "alice", account.User.Username)
		require.Equal(t, "Alice", account.User.DisplayName)
	})

	t.Run("lookupStorageAccountByID returns not found when handler has no repositories", func(t *testing.T) {
		var h *Handler
		account, err := h.lookupStorageAccountByID(context.Background(), "alice")
		require.Nil(t, account)
		require.Error(t, err)
	})

	t.Run("lookupStorageAccountByID returns registry errors for stable usernames", func(t *testing.T) {
		cfg := round10TestConfig()
		h := &Handler{
			cfg: cfg,
			registry: &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
						return nil, errors.New("lookup failed")
					},
				},
			},
		}

		account, err := h.lookupStorageAccountByID(context.Background(), "alice")
		require.Nil(t, account)
		require.EqualError(t, err, "lookup failed")
	})

	t.Run("lookupStorageAccountByID returns not found when registry misses stable usernames", func(t *testing.T) {
		cfg := round10TestConfig()
		h := &Handler{
			cfg: cfg,
			registry: &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
						return nil, nil
					},
				},
			},
		}

		account, err := h.lookupStorageAccountByID(context.Background(), "alice")
		require.Nil(t, account)
		require.EqualError(t, err, "account not found")
	})

	t.Run("lookupStorageAccountByID falls back to actor resolution for local URLs", func(t *testing.T) {
		cfg := round10TestConfig()
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"alice": {
					Username:     "alice",
					DisplayName:  "Alice",
					CreatedAt:    time.Now().Add(-time.Hour),
					UpdatedAt:    time.Now().Add(-time.Hour),
					Discoverable: true,
					Role:         "user",
				},
			},
			actorsByUser: map[string]storagemodels.Actor{
				"alice": {
					Username: "alice",
					Actor: &activitypub.Actor{
						BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
						PreferredUsername: "alice",
						Name:              "Alice",
					},
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		account, err := h.lookupStorageAccountByID(context.Background(), cfg.BaseURL()+"/users/alice")
		require.NoError(t, err)
		require.NotNil(t, account)
		require.Equal(t, "alice", account.User.Username)
		require.Equal(t, cfg.BaseURL()+"/users/alice", account.Actor.ID)
	})

	t.Run("lookupStorageAccountByID synthesizes remote actor accounts after fallback resolution", func(t *testing.T) {
		cfg := round10TestConfig()
		repos := &MockRepositoryStorage{}
		actorRepo := &accountsRound20LookupActorRepo{
			actor: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/alice", Type: "Person"},
				PreferredUsername: "alice@example.com",
				Name:              "Alice",
			},
		}
		repos.On("Account").Return(nil).Once()
		repos.On("Actor").Return(actorRepo).Once()

		h := &Handler{cfg: cfg, repos: repos}
		account, err := h.lookupStorageAccountByID(context.Background(), "alice")
		require.NoError(t, err)
		require.NotNil(t, account)
		require.Equal(t, "alice", actorRepo.username)
		require.Equal(t, "alice@example.com", account.User.Username)
		require.Equal(t, "Alice", account.User.DisplayName)
		require.Equal(t, "https://remote.example/users/alice", account.Actor.ID)
		repos.AssertExpectations(t)
	})

	t.Run("lookupStorageAccountByID propagates actor resolution errors during fallback", func(t *testing.T) {
		cfg := round10TestConfig()
		repos := &MockRepositoryStorage{}
		actorRepo := &accountsRound20LookupActorRepo{err: errors.New("actor lookup failed")}
		repos.On("Account").Return(nil).Once()
		repos.On("Actor").Return(actorRepo).Once()

		h := &Handler{cfg: cfg, repos: repos}
		account, err := h.lookupStorageAccountByID(context.Background(), "alice")
		require.Nil(t, account)
		require.EqualError(t, err, "actor lookup failed")
		require.Equal(t, "alice", actorRepo.username)
		repos.AssertExpectations(t)
	})

	t.Run("lookupStorageAccountByID returns not found when fallback resolution finds no actor", func(t *testing.T) {
		cfg := round10TestConfig()
		repos := &MockRepositoryStorage{}
		actorRepo := &accountsRound20LookupActorRepo{}
		repos.On("Account").Return(nil).Once()
		repos.On("Actor").Return(actorRepo).Once()

		h := &Handler{cfg: cfg, repos: repos}
		account, err := h.lookupStorageAccountByID(context.Background(), "alice")
		require.Nil(t, account)
		require.EqualError(t, err, "account not found")
		require.Equal(t, "alice", actorRepo.username)
		repos.AssertExpectations(t)
	})

	t.Run("resolveRelationshipUsername falls back to actor preferred username", func(t *testing.T) {
		cfg := round10TestConfig()
		h := &Handler{
			cfg: cfg,
			registry: &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
						return &storage.Account{
							Actor: &activitypub.Actor{PreferredUsername: "actor-only"},
						}, nil
					},
				},
			},
		}

		username, err := h.resolveRelationshipUsername(context.Background(), "actor-only")
		require.NoError(t, err)
		require.Equal(t, "actor-only", username)
	})

	t.Run("resolveRelationshipUsername prefers storage usernames", func(t *testing.T) {
		cfg := round10TestConfig()
		h := &Handler{
			cfg: cfg,
			registry: &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
						return &storage.Account{
							User:  &storage.User{Username: "stored-user"},
							Actor: &activitypub.Actor{PreferredUsername: "actor-only"},
						}, nil
					},
				},
			},
		}

		username, err := h.resolveRelationshipUsername(context.Background(), "stored-user")
		require.NoError(t, err)
		require.Equal(t, "stored-user", username)
	})

	t.Run("resolveRelationshipUsername returns error when account has no usable username", func(t *testing.T) {
		cfg := round10TestConfig()
		h := &Handler{
			cfg: cfg,
			registry: &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
						return &storage.Account{User: &storage.User{}}, nil
					},
				},
			},
		}

		_, err := h.resolveRelationshipUsername(context.Background(), "missing")
		require.Error(t, err)
	})

	t.Run("resolveRelationshipUsername propagates account lookup errors", func(t *testing.T) {
		cfg := round10TestConfig()
		h := &Handler{
			cfg: cfg,
			registry: &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
						return nil, errors.New("lookup failed")
					},
				},
			},
		}

		_, err := h.resolveRelationshipUsername(context.Background(), "missing")
		require.EqualError(t, err, "lookup failed")
	})
}
