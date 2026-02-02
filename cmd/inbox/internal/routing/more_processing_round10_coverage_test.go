package routing

import (
	"context"
	stdliberrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInboxHandler_Round10_ProcessRejectHelpers_MoreBranches(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("processRejectInteraction object map and delete error", func(t *testing.T) {
		reject := &activitypub.Activity{Actor: env.remoteActorID}
		target := &activitypub.Activity{
			Actor: env.remoteActorID,
			Object: map[string]any{
				"id": env.cfg.BaseURL() + "/objects/1",
			},
		}

		err := env.handler.processRejectInteraction(context.Background(), reject, target, rejectActivityConfig{
			activityType:   "like",
			actorFieldName: "liker",
			deleteFunc: func(context.Context, string, string) error {
				return stdliberrors.New("no like")
			},
		})
		require.NoError(t, err)
	})

	t.Run("processRejectInteraction missing object id returns nil", func(t *testing.T) {
		reject := &activitypub.Activity{Actor: env.remoteActorID}
		target := &activitypub.Activity{Actor: env.remoteActorID, Object: map[string]any{}}

		require.NoError(t, env.handler.processRejectInteraction(context.Background(), reject, target, rejectActivityConfig{
			activityType:   "announce",
			actorFieldName: "announcer",
			deleteFunc: func(context.Context, string, string) error {
				return nil
			},
		}))
	})

	t.Run("processSimpleReject missing object id returns nil", func(t *testing.T) {
		reject := &activitypub.Activity{Actor: env.remoteActorID}
		target := &activitypub.Activity{Actor: env.remoteActorID}

		require.NoError(t, env.handler.processSimpleReject(context.Background(), reject, target, simpleRejectConfig{
			activityType:    "create",
			actorFieldName:  "creator",
			objectFieldName: "object",
			warningMessage:  "missing object",
			successMessage:  "ok",
			includeTarget:   true,
		}))
	})

	t.Run("processSimpleReject includeTarget uses target field", func(t *testing.T) {
		reject := &activitypub.Activity{Actor: env.remoteActorID}
		target := &activitypub.Activity{
			Actor:  env.remoteActorID,
			Object: env.cfg.BaseURL() + "/objects/1",
			Target: env.cfg.BaseURL() + "/users/alice/featured",
		}

		require.NoError(t, env.handler.processSimpleReject(context.Background(), reject, target, simpleRejectConfig{
			activityType:    "add",
			actorFieldName:  "adder",
			objectFieldName: "object",
			warningMessage:  "missing object",
			successMessage:  "ok",
			includeTarget:   true,
		}))
	})
}

func TestInboxHandler_Round10_MoveAuthorization_ErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("extract username error", func(t *testing.T) {
		require.Error(t, env.handler.validateMoveAuthorization(context.Background(), "old", "", &activitypub.Activity{}))
	})

	t.Run("alsoKnownAs missing", func(t *testing.T) {
		require.Error(t, env.handler.validateMoveAuthorization(context.Background(), "old", env.cfg.ActorURL("alice"), &activitypub.Activity{}))
	})

	t.Run("alsoKnownAs check error maps to verifyMoveAuthError", func(t *testing.T) {
		call := env.mockQuery.On("First", mock.Anything).Return(stdliberrors.New("boom")).Once()
		env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

		require.Error(t, env.handler.validateMoveAuthorization(context.Background(), "old", env.cfg.ActorURL("alice"), &activitypub.Activity{}))
	})
}

func TestInboxHandler_Round10_MoveFollowerMigration_ErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("get followers error", func(t *testing.T) {
		call := env.mockQuery.On("All", mock.Anything).Return(stdliberrors.New("boom")).Once()
		env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

		require.Error(t, env.handler.processMoveFollowerMigration(context.Background(), env.cfg.ActorURL("old"), env.cfg.ActorURL("alice")))
	})

	t.Run("invalid base domain skips local migration", func(t *testing.T) {
		handler := *env.handler
		handler.baseURL = "not-a-url"

		require.NoError(t, handler.processMoveFollowerMigration(context.Background(), env.cfg.ActorURL("old"), env.cfg.ActorURL("alice")))
	})

	t.Run("delete relationship error continues", func(t *testing.T) {
		call := env.mockQuery.On("Delete").Return(stdliberrors.New("boom")).Once()
		env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

		require.NoError(t, env.handler.processMoveFollowerMigration(context.Background(), env.cfg.ActorURL("old"), env.cfg.ActorURL("alice")))
	})

	t.Run("create relationship error continues", func(t *testing.T) {
		call := env.mockQuery.On("Create").Return(stdliberrors.New("boom")).Once()
		env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

		require.NoError(t, env.handler.processMoveFollowerMigration(context.Background(), env.cfg.ActorURL("old"), env.cfg.ActorURL("alice")))
	})
}

func TestInboxHandler_Round10_FlagAndBlock_MoreErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("processFlagActivity invalid flag object returns error", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.FlagType, ID: env.cfg.BaseURL() + "/activities/flag-invalid"},
			Actor:      env.remoteActorID,
			Object:     123,
		}
		require.Error(t, env.handler.processFlagActivity(context.Background(), activity, env.local))
	})

	t.Run("processFlagActivity no flagged objects returns error", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.FlagType, ID: env.cfg.BaseURL() + "/activities/flag-empty"},
			Actor:      env.remoteActorID,
			Object:     []any{123},
		}
		require.Error(t, env.handler.processFlagActivity(context.Background(), activity, env.local))
	})

	t.Run("processFlagActivity store error", func(t *testing.T) {
		call := env.mockQuery.On("Create").Return(stdliberrors.New("boom")).Once()
		env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.FlagType,
				ID:      env.cfg.BaseURL() + "/activities/flag-store-error",
				Summary: "spam",
			},
			Actor:  env.remoteActorID,
			Object: env.cfg.BaseURL() + "/objects/1",
		}
		require.Error(t, env.handler.processFlagActivity(context.Background(), activity, env.local))
	})

	t.Run("processBlockActivity missing blocked actor id returns nil", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.BlockType, ID: env.cfg.BaseURL() + "/activities/block-missing"},
			Actor:      env.remoteActorID,
			Object:     map[string]any{"type": activitypub.PersonType},
		}
		require.NoError(t, env.handler.processBlockActivity(context.Background(), activity, env.local))
	})

	t.Run("processBlockActivity create block error", func(t *testing.T) {
		call := env.mockQuery.On("Create").Return(stdliberrors.New("boom")).Once()
		env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.BlockType, ID: env.cfg.BaseURL() + "/activities/block-create-error"},
			Actor:      env.remoteActorID,
			Object:     env.cfg.ActorURL("blocked"),
		}
		require.Error(t, env.handler.processBlockActivity(context.Background(), activity, env.local))
	})
}

func TestInboxHandler_Round10_UndoBlock_MoreBranches(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("unauthorized undo block returns error", func(t *testing.T) {
		undoActivity := &activitypub.Activity{Actor: env.remoteActorID, BaseObject: activitypub.BaseObject{ID: env.cfg.BaseURL() + "/activities/undo-1"}}
		blockActivity := &activitypub.Activity{Actor: env.cfg.ActorURL("someone-else"), BaseObject: activitypub.BaseObject{ID: env.cfg.BaseURL() + "/activities/block-1"}, Object: env.cfg.ActorURL("blocked")}
		require.Error(t, env.handler.processUndoBlock(context.Background(), undoActivity, blockActivity))
	})

	t.Run("delete block error returns deleteBlockError", func(t *testing.T) {
		call := env.mockQuery.On("Delete").Return(stdliberrors.New("boom")).Once()
		env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

		actorID := env.cfg.ActorURL("blocker")
		undoActivity := &activitypub.Activity{Actor: actorID, BaseObject: activitypub.BaseObject{ID: env.cfg.BaseURL() + "/activities/undo-2"}}
		blockActivity := &activitypub.Activity{Actor: actorID, BaseObject: activitypub.BaseObject{ID: env.cfg.BaseURL() + "/activities/block-2"}, Object: env.cfg.ActorURL("blocked")}

		require.Error(t, env.handler.processUndoBlock(context.Background(), undoActivity, blockActivity))
	})

	t.Run("missing blocked actor id returns nil", func(t *testing.T) {
		undoActivity := &activitypub.Activity{Actor: env.remoteActorID, BaseObject: activitypub.BaseObject{ID: env.cfg.BaseURL() + "/activities/undo-3"}}
		blockActivity := &activitypub.Activity{Actor: env.remoteActorID, BaseObject: activitypub.BaseObject{ID: env.cfg.BaseURL() + "/activities/block-3"}, Object: map[string]any{}}
		require.NoError(t, env.handler.processUndoBlock(context.Background(), undoActivity, blockActivity))
	})
}

func TestInboxHandler_Round10_CascadeDeleteNotifications_ErrorBranch(t *testing.T) {
	env := newInboxTestEnv(t)

	call := env.mockQuery.On("All", mock.Anything).Return(stdliberrors.New("boom")).Once()
	env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

	require.Error(t, env.handler.cascadeDeleteNotifications(context.Background(), env.cfg.BaseURL()+"/objects/1"))
}

func TestInboxHandler_Round10_ValidateActivity_ErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)

	validActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   env.local.ID,
			Type: activitypub.PersonType,
		},
		PreferredUsername: env.local.PreferredUsername,
		Inbox:             env.local.Inbox,
		Outbox:            env.local.Outbox,
	}

	t.Run("validateBasicActivity fails", func(t *testing.T) {
		bad := &activitypub.Activity{Actor: env.remoteActorID}
		require.Error(t, env.handler.validateActivity(bad, validActor))
	})

	t.Run("validateActivityAddressing fails", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.FollowType,
				ID:      env.cfg.BaseURL() + "/activities/va-1",
				To:      []string{"https://remote.example/users/bob"},
				BTo:     []string{"not-a-url"},
			},
			Actor:  env.remoteActorID,
			Object: env.local.ID,
		}
		require.Error(t, env.handler.validateActivity(activity, validActor))
	})

	t.Run("validateActorUsername fails", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.FollowType,
				ID:      env.cfg.BaseURL() + "/activities/va-2",
				To:      []string{env.local.ID},
			},
			Actor:  "https://remote.example/users/bad user",
			Object: env.local.ID,
		}
		require.Error(t, env.handler.validateActivity(activity, validActor))
	})

	t.Run("validateActorPublicKey fails", func(t *testing.T) {
		actorWithBadKey := *validActor
		actorWithBadKey.PublicKey = &activitypub.PublicKey{ID: "not-a-url"}

		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.FollowType,
				ID:      env.cfg.BaseURL() + "/activities/va-3",
				To:      []string{env.local.ID},
			},
			Actor:  env.remoteActorID,
			Object: env.local.ID,
		}
		require.Error(t, env.handler.validateActivity(activity, &actorWithBadKey))
	})

	t.Run("validateCreateActivityObject fails", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.CreateType,
				ID:      env.cfg.BaseURL() + "/activities/va-4",
				To:      []string{env.local.ID},
			},
			Actor:  env.remoteActorID,
			Object: map[string]any{"type": "Note"},
		}
		require.Error(t, env.handler.validateActivity(activity, validActor))
	})

	t.Run("validateActivityTargeting fails", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.FollowType,
				ID:      env.cfg.BaseURL() + "/activities/va-5",
				To:      []string{"https://remote.example/users/other"},
			},
			Actor:  env.remoteActorID,
			Object: env.local.ID,
		}
		require.Error(t, env.handler.validateActivity(activity, validActor))
	})
}

func TestInboxHandler_Round10_ProcessMoveActivity_StoreMigrationError(t *testing.T) {
	env := newInboxTestEnv(t)

	env.local.AlsoKnownAs = []string{env.cfg.ActorURL("old")}

	call := env.mockQuery.On("Create").Return(stdliberrors.New("boom")).Once()
	env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.MoveType, ID: env.cfg.BaseURL() + "/activities/move-store-error"},
		Actor:      env.cfg.ActorURL("old"),
		Target:     env.cfg.ActorURL("alice"),
	}

	require.Error(t, env.handler.processMoveActivity(context.Background(), activity, env.local))
}

func TestInboxHandler_Round10_ProcessActivityByType_ErrorBranches(t *testing.T) {
	makeReq := func(env *inboxTestEnv, activity *activitypub.Activity) *InboxRequest {
		now := time.Now()
		return &InboxRequest{
			Username:    env.local.PreferredUsername,
			Activity:    activity,
			Actor:       env.local,
			Body:        []byte(`{}`),
			ActorDomain: "remote.example",
			StartTime:   now,
			CostParams: &federation.CostCalculationParams{
				ActivityID:    activity.ID,
				Domain:        "remote.example",
				ActivityType:  activity.Type,
				Direction:     "inbound",
				OperationType: "inbox_processing",
				Timestamp:     now,
			},
		}
	}

	cases := []struct {
		name  string
		setup func(env *inboxTestEnv)
		make  func(env *inboxTestEnv) *activitypub.Activity
	}{
		{
			name: "follow errors bubble",
			setup: func(env *inboxTestEnv) {
				call := env.mockQuery.On("Create").Return(stdliberrors.New("boom")).Once()
				env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)
			},
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.FollowType, ID: env.cfg.BaseURL() + "/activities/follow-error"},
					Actor:      env.remoteActorID,
					Object:     env.local.ID,
				}
			},
		},
		{
			name: "accept errors bubble",
			setup: func(env *inboxTestEnv) {
				call := env.mockQuery.On("Update", mock.Anything).Return(stdliberrors.New("boom")).Once()
				env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)
			},
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.AcceptType, ID: env.cfg.BaseURL() + "/activities/accept-error"},
					Actor:      env.remoteActorID,
					Object:     env.cfg.BaseURL() + "/activities/follow-lookup",
				}
			},
		},
		{
			name: "reject errors bubble",
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.RejectType, ID: env.cfg.BaseURL() + "/activities/reject-error"},
					Actor:      env.remoteActorID,
					Object: map[string]any{
						"type":      activitypub.FollowType,
						"id":        env.cfg.BaseURL() + "/activities/follow-reject-error",
						"published": "not-a-time",
					},
				}
			},
		},
		{
			name: "create errors bubble",
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.CreateType, ID: env.cfg.BaseURL() + "/activities/create-error"},
					Actor:      env.remoteActorID,
					Object: map[string]any{
						"type": activitypub.NoteType,
						"id":   env.cfg.BaseURL() + "/objects/bad-note",
					},
				}
			},
		},
		{
			name: "update errors bubble",
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-error"},
					Actor:      env.cfg.ActorURL("mallory"),
					Object: map[string]any{
						"id":   env.cfg.BaseURL() + "/objects/1",
						"type": activitypub.NoteType,
					},
				}
			},
		},
		{
			name: "delete errors bubble",
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-error"},
					Actor:      env.cfg.ActorURL("mallory"),
					Object:     env.cfg.BaseURL() + "/objects/1",
				}
			},
		},
		{
			name: "like errors bubble",
			setup: func(env *inboxTestEnv) {
				call := env.mockQuery.On("Create").Return(stdliberrors.New("boom")).Once()
				env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)
			},
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.LikeType, ID: env.cfg.BaseURL() + "/activities/like-error"},
					Actor:      env.remoteActorID,
					Object:     env.cfg.BaseURL() + "/objects/1",
				}
			},
		},
		{
			name: "announce errors bubble",
			setup: func(env *inboxTestEnv) {
				call := env.mockQuery.On("Create").Return(stdliberrors.New("boom")).Once()
				env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)
			},
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.AnnounceType, ID: env.cfg.BaseURL() + "/activities/announce-error"},
					Actor:      env.remoteActorID,
					Object:     env.cfg.BaseURL() + "/objects/1",
				}
			},
		},
		{
			name: "undo errors bubble",
			setup: func(env *inboxTestEnv) {
				call := env.mockQuery.On("Delete").Return(stdliberrors.New("boom")).Once()
				env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)
			},
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.UndoType, ID: env.cfg.BaseURL() + "/activities/undo-error"},
					Actor:      env.remoteActorID,
					Object:     env.cfg.BaseURL() + "/activities/follow-undo-error",
				}
			},
		},
		{
			name: "block errors bubble",
			setup: func(env *inboxTestEnv) {
				call := env.mockQuery.On("Create").Return(stdliberrors.New("boom")).Once()
				env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)
			},
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.BlockType, ID: env.cfg.BaseURL() + "/activities/block-error"},
					Actor:      env.remoteActorID,
					Object:     env.cfg.ActorURL("blocked"),
				}
			},
		},
		{
			name: "add errors bubble",
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-error"},
					Actor:      env.local.ID,
					Object:     env.cfg.BaseURL() + "/objects/1",
				}
			},
		},
		{
			name: "remove errors bubble",
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-error"},
					Actor:      env.local.ID,
					Object:     env.cfg.BaseURL() + "/objects/1",
				}
			},
		},
		{
			name: "flag errors bubble",
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.FlagType, ID: env.cfg.BaseURL() + "/activities/flag-error"},
					Actor:      env.remoteActorID,
					Object:     123,
				}
			},
		},
		{
			name: "move errors bubble",
			make: func(env *inboxTestEnv) *activitypub.Activity {
				return &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.MoveType, ID: env.cfg.BaseURL() + "/activities/move-error"},
					Actor:      env.remoteActorID,
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newInboxTestEnv(t)
			setRunAsyncSynchronous(t)

			if tc.setup != nil {
				tc.setup(env)
			}

			activity := tc.make(env)
			require.Error(t, env.handler.processActivityByType(context.Background(), makeReq(env, activity)))
		})
	}
}

func TestInboxHandler_Round10_ExtractHelpers_EdgeCases(t *testing.T) {
	env := newInboxTestEnv(t)

	require.Equal(t, "alice", env.handler.extractHandleFromActorID("alice"))
	require.Empty(t, env.handler.extractUsernameFromActorID("alice"))
	require.Empty(t, env.handler.extractDomainFromURL("://bad"))
}
