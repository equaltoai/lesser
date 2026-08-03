package routing

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/stretchr/testify/require"
)

func TestInboxHandler_Round10_ProcessActivityByType_AllCases(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	env.local.AlsoKnownAs = []string{env.cfg.ActorURL("old")}

	makeReq := func(activity *activitypub.Activity) *InboxRequest {
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

	objectID := env.cfg.BaseURL() + "/objects/1"
	targetCollection := env.local.ID + "/featured"

	cases := []struct {
		name     string
		activity *activitypub.Activity
	}{
		{
			name: "follow",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.FollowType, ID: env.cfg.BaseURL() + "/activities/follow-bytype", To: []string{env.local.ID}},
				Actor:      env.remoteActorID,
				Object:     env.local.ID,
			},
		},
		{
			name: "accept",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.AcceptType, ID: env.cfg.BaseURL() + "/activities/accept-bytype", To: []string{env.local.ID}},
				Actor:      env.remoteActorID,
				Object:     env.cfg.BaseURL() + "/activities/follow-lookup",
			},
		},
		{
			name: "reject",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.RejectType, ID: env.cfg.BaseURL() + "/activities/reject-bytype"},
				Actor:      env.remoteActorID,
				Object:     env.cfg.BaseURL() + "/activities/follow-lookup",
			},
		},
		{
			name: "create",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.CreateType, ID: env.cfg.BaseURL() + "/activities/create-bytype"},
				Actor:      env.remoteActorID,
				Object: map[string]any{
					"@context":     "https://www.w3.org/ns/activitystreams",
					"id":           env.cfg.BaseURL() + "/objects/note-bytype",
					"type":         activitypub.NoteType,
					"attributedTo": env.remoteActorID,
					"content":      "bytype",
				},
			},
		},
		{
			name: "update",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-bytype"},
				Actor:      env.remoteActorID,
				Object: map[string]any{
					"@context":     "https://www.w3.org/ns/activitystreams",
					"id":           objectID,
					"type":         activitypub.NoteType,
					"attributedTo": env.remoteActorID,
					"content":      "updated bytype",
					"to":           []any{activitypub.PublicAddress},
				},
			},
		},
		{
			name: "delete",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-bytype"},
				Actor:      env.remoteActorID,
				Object:     objectID,
			},
		},
		{
			name: "like",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.LikeType, ID: env.cfg.BaseURL() + "/activities/like-bytype"},
				Actor:      env.remoteActorID,
				Object:     objectID,
			},
		},
		{
			name: "announce",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.AnnounceType, ID: env.cfg.BaseURL() + "/activities/announce-bytype"},
				Actor:      env.remoteActorID,
				Object:     objectID,
			},
		},
		{
			name: "undo",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.UndoType, ID: env.cfg.BaseURL() + "/activities/undo-bytype"},
				Actor:      env.remoteActorID,
				Object: map[string]any{
					"type":   activitypub.LikeType,
					"id":     env.cfg.BaseURL() + "/activities/like-undo-target",
					"actor":  env.remoteActorID,
					"object": objectID,
				},
			},
		},
		{
			name: "block",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.BlockType, ID: env.cfg.BaseURL() + "/activities/block-bytype"},
				Actor:      env.remoteActorID,
				Object:     "https://remote.example/users/spammer",
			},
		},
		{
			name: "add",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-bytype"},
				Actor:      env.local.ID,
				Object:     objectID,
				Target:     targetCollection,
			},
		},
		{
			name: "remove",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-bytype"},
				Actor:      env.local.ID,
				Object:     objectID,
				Target:     targetCollection,
			},
		},
		{
			name: "flag",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.FlagType, ID: env.cfg.BaseURL() + "/activities/flag-bytype", Summary: "spam"},
				Actor:      env.remoteActorID,
				Object:     objectID,
			},
		},
		{
			name: "move",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: activitypub.MoveType, ID: env.cfg.BaseURL() + "/activities/move-bytype"},
				Actor:      env.cfg.ActorURL("old"),
				Target:     env.cfg.ActorURL("alice"),
			},
		},
		{
			name: "unsupported type",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{Context: activitypub.Context, Type: "Ping", ID: env.cfg.BaseURL() + "/activities/unsupported-bytype"},
				Actor:      env.remoteActorID,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := makeReq(tc.activity)
			require.NoError(t, env.handler.processActivityByType(context.Background(), req))
		})
	}
}
