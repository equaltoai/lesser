package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func l17RemoteActor(username, domain string) *activitypub.Actor {
	actorID := "https://" + domain + "/users/" + username
	return &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   actorID,
			Type: activitypub.PersonType,
		},
		PreferredUsername: username,
		Name:              "Remote " + username,
		URL:               "https://" + domain + "/@" + username,
		Inbox:             actorID + "/inbox",
		Outbox:            actorID + "/outbox",
	}
}

func l17CachedRemoteActor(handle string, actor *activitypub.Actor) storagemodels.RemoteActor {
	now := time.Now().UTC()
	cached := storagemodels.RemoteActor{
		Handle:    handle,
		Actor:     actor,
		CachedAt:  now.Add(-time.Minute),
		UpdatedAt: now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
	}
	cached.UpdateKeys()
	return cached
}

func l17LocalAccount(cfgDomain, username string) *storage.Account {
	actorID := "https://" + cfgDomain + "/users/" + username
	return &storage.Account{
		User: &storage.User{
			Username:    username,
			DisplayName: "Local " + username,
			Approved:    true,
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   actorID,
				Type: activitypub.PersonType,
			},
			PreferredUsername: username,
			Name:              "Local " + username,
			URL:               "https://" + cfgDomain + "/@" + username,
			Inbox:             actorID + "/inbox",
			Outbox:            actorID + "/outbox",
		},
	}
}

func l17AccountsStub(cfgDomain string) *AccountsServiceStub {
	return &AccountsServiceStub{
		GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
			if username == "alice" {
				return l17LocalAccount(cfgDomain, username), nil
			}
			return nil, errors.New("not found")
		},
	}
}

func TestL17RemoteActorHelperBranches(t *testing.T) {
	cfg := round11TestConfig()
	remoteActor := l17RemoteActor("alice", "remote.example")
	cached := l17CachedRemoteActor("alice@remote.example", remoteActor)
	state := &round10QueryState{
		remoteActorsByPK: map[string]storagemodels.RemoteActor{
			cached.PK: cached,
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: l17AccountsStub(cfg.Domain)})
	ctx := context.Background()

	t.Run("remote detector separates remote and local identifiers", func(t *testing.T) {
		require.True(t, h.actorIdentifierLooksRemote(remoteActor.ID))
		require.True(t, h.actorIdentifierLooksRemote("@alice@remote.example"))
		require.False(t, h.actorIdentifierLooksRemote(cfg.BaseURL()+"/users/alice"))
		require.False(t, h.actorIdentifierLooksRemote("@alice@"+cfg.Domain))
		require.False(t, h.actorIdentifierLooksRemote("alice"))
		require.False(t, h.actorIdentifierLooksRemote(""))
		require.False(t, h.actorIdentifierLooksRemote("https://%"))
		require.False(t, h.actorIdentifierLooksRemote("https:///users/alice"))
		require.True(t, (*Handler)(nil).actorIdentifierLooksRemote(remoteActor.ID))
		require.True(t, (*Handler)(nil).actorIdentifierLooksRemote("@alice@remote.example"))
		require.False(t, (*Handler)(nil).actorIdentifierLooksRemote("alice@"))
	})

	t.Run("cached remote lookup tries URL and handle candidates", func(t *testing.T) {
		fromURL := h.cachedRemoteActorForIdentifier(ctx, remoteActor.ID)
		require.NotNil(t, fromURL)
		require.Equal(t, remoteActor.ID, fromURL.ID)

		fromHandle := h.cachedRemoteActorForIdentifier(ctx, "@alice@remote.example")
		require.NotNil(t, fromHandle)
		require.Equal(t, remoteActor.ID, fromHandle.ID)

		craftedActorID := remoteActor.ID + "/anything"
		require.Nil(t, h.cachedRemoteActorForIdentifier(ctx, craftedActorID))
		require.Nil(t, h.cachedRemoteNotificationActor(ctx, craftedActorID))

		craftedSynthetic := h.resolveAttributedActorForObject(ctx, craftedActorID)
		require.NotNil(t, craftedSynthetic)
		require.Equal(t, craftedActorID, craftedSynthetic.ID)
		require.NotEqual(t, remoteActor.ID, craftedSynthetic.ID)

		require.Nil(t, h.cachedRemoteActorForIdentifier(ctx, "https://missing.example/users/alice"))
		require.Nil(t, h.cachedRemoteActorForIdentifier(ctx, "missing@remote.example"))
		require.Nil(t, h.cachedRemoteActorForIdentifier(ctx, "   "))
		require.Nil(t, (*Handler)(nil).cachedRemoteActorForIdentifier(ctx, remoteActor.ID))
	})

	t.Run("synthetic placeholders preserve remote identity", func(t *testing.T) {
		require.Empty(t, remoteActorPlaceholderUsername(""))
		require.Equal(t, "alice", remoteActorPlaceholderUsername("alice@remote.example"))
		require.Equal(t, "alice", remoteActorPlaceholderUsername(remoteActor.ID))
		require.NotEmpty(t, remoteActorPlaceholderUsername("https://remote.example/@alice"))
		require.NotEmpty(t, remoteActorPlaceholderUsername("urn:remote:alice"))

		require.Nil(t, syntheticRemoteActorFromIdentifier(""))
		synthetic := syntheticRemoteActorFromIdentifier("https://uncached.example/users/bob")
		require.NotNil(t, synthetic)
		require.Equal(t, "https://uncached.example/users/bob", synthetic.ID)
		require.Equal(t, "bob", synthetic.PreferredUsername)
		require.Equal(t, "bob", synthetic.Name)
		require.Equal(t, "https://uncached.example/users/bob", synthetic.URL)
	})

	t.Run("attributed actor resolver returns cached remote, synthetic remote, and local actors", func(t *testing.T) {
		require.Nil(t, h.resolveAttributedActorForObject(ctx, ""))
		require.Nil(t, (*Handler)(nil).resolveAttributedActorForObject(ctx, cfg.BaseURL()+"/users/alice"))

		cachedActor := h.resolveAttributedActorForObject(ctx, remoteActor.ID)
		require.NotNil(t, cachedActor)
		require.Equal(t, remoteActor.ID, cachedActor.ID)
		require.Equal(t, "Remote alice", cachedActor.Name)

		synthetic := h.resolveAttributedActorForObject(ctx, "https://uncached.example/users/alice")
		require.NotNil(t, synthetic)
		require.Equal(t, "https://uncached.example/users/alice", synthetic.ID)
		require.Equal(t, "alice", synthetic.PreferredUsername)
		require.NotEqual(t, cfg.BaseURL()+"/users/alice", synthetic.ID)

		local := h.resolveAttributedActorForObject(ctx, cfg.BaseURL()+"/users/alice")
		require.NotNil(t, local)
		require.Equal(t, cfg.BaseURL()+"/users/alice", local.ID)
		require.Equal(t, "Local alice", local.Name)

		missingLocal := h.resolveAttributedActorForObject(ctx, cfg.BaseURL()+"/users/missing")
		require.Nil(t, missingLocal)
	})

	t.Run("stored status author rejects canonical mismatch before handle fallback", func(t *testing.T) {
		craftedActorID := remoteActor.ID + "/anything"
		actor := h.loadStoredStatusAuthorActor(ctx, &storagemodels.Status{
			StatusID:       "crafted-status",
			AuthorID:       craftedActorID,
			AuthorUsername: "alice@remote.example",
			Note: &activitypub.Note{
				AttributedTo: craftedActorID,
			},
		})
		require.Nil(t, actor)
	})
}

func TestL17AIAnalysisOwnerUsernameRemoteAndLocalFallbacks(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		statusByID: map[string]storagemodels.Status{
			"stored-owner": {
				StatusID:       "stored-owner",
				AuthorUsername: "alice",
				AuthorID:       cfg.BaseURL() + "/users/ignored",
			},
			"local-author-id": {
				StatusID: "local-author-id",
				AuthorID: cfg.BaseURL() + "/users/alice",
			},
			"remote-author-id": {
				StatusID: "remote-author-id",
				AuthorID: "https://remote.example/users/alice",
			},
		},
	})

	owner, err := h.aiAnalysisOwnerUsername(context.Background(), "stored-owner")
	require.NoError(t, err)
	require.Equal(t, "alice", owner)

	owner, err = h.aiAnalysisOwnerUsername(context.Background(), "local-author-id")
	require.NoError(t, err)
	require.Equal(t, "alice", owner)

	owner, err = h.aiAnalysisOwnerUsername(context.Background(), "remote-author-id")
	require.NoError(t, err)
	require.Empty(t, owner)
}

func TestL17SiblingAttributionExtractorsRemoteAndLocalBranches(t *testing.T) {
	cfg := round11TestConfig()
	remoteActor := l17RemoteActor("alice", "remote.example")
	cached := l17CachedRemoteActor("alice@remote.example", remoteActor)
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		remoteActorsByPK: map[string]storagemodels.RemoteActor{
			cached.PK: cached,
		},
	}, &RegistryStub{AccountsSvc: l17AccountsStub(cfg.Domain)})

	liftCtx, err := round10NewLiftContext(http.MethodGet, "/test", nil, nil, nil)
	require.NoError(t, err)

	localNote := &activitypub.Note{AttributedTo: cfg.BaseURL() + "/users/alice"}
	cachedRemoteNote := &activitypub.Note{AttributedTo: remoteActor.ID}
	uncachedRemoteNote := &activitypub.Note{AttributedTo: "https://uncached.example/users/alice"}

	t.Run("statuses getActorForObject keeps remote and local paths distinct", func(t *testing.T) {
		local := h.getActorForObject(context.Background(), localNote)
		require.NotNil(t, local)
		require.Equal(t, cfg.BaseURL()+"/users/alice", local.ID)

		cachedRemote := h.getActorForObject(context.Background(), cachedRemoteNote)
		require.NotNil(t, cachedRemote)
		require.Equal(t, remoteActor.ID, cachedRemote.ID)
		require.NotEqual(t, cfg.BaseURL()+"/users/alice", cachedRemote.ID)

		placeholder := h.getActorForObject(context.Background(), uncachedRemoteNote)
		require.NotNil(t, placeholder)
		require.Equal(t, uncachedRemoteNote.AttributedTo, placeholder.ID)
		require.NotEqual(t, cfg.BaseURL()+"/users/alice", placeholder.ID)
	})

	t.Run("status info getHistoryAuthorActor keeps remote and local paths distinct", func(t *testing.T) {
		local := h.getHistoryAuthorActor(liftCtx, map[string]any{"attributedTo": localNote.AttributedTo})
		require.NotNil(t, local)
		require.Equal(t, cfg.BaseURL()+"/users/alice", local.ID)

		cachedRemote := h.getHistoryAuthorActor(liftCtx, map[string]any{"attributedTo": cachedRemoteNote.AttributedTo})
		require.NotNil(t, cachedRemote)
		require.Equal(t, remoteActor.ID, cachedRemote.ID)

		placeholder := h.getHistoryAuthorActor(liftCtx, map[string]any{"attributedTo": uncachedRemoteNote.AttributedTo})
		require.NotNil(t, placeholder)
		require.Equal(t, uncachedRemoteNote.AttributedTo, placeholder.ID)
		require.NotEqual(t, cfg.BaseURL()+"/users/alice", placeholder.ID)
	})

	t.Run("oEmbed author helpers keep remote and local paths distinct", func(t *testing.T) {
		local := h.getOEmbedAuthorActor(liftCtx, localNote)
		require.NotNil(t, local)
		require.Equal(t, cfg.BaseURL()+"/users/alice", local.ID)

		cachedRemote := h.getOEmbedAuthorActor(liftCtx, cachedRemoteNote)
		require.NotNil(t, cachedRemote)
		require.Equal(t, remoteActor.ID, cachedRemote.ID)

		placeholder := h.getOEmbedAuthorActor(liftCtx, uncachedRemoteNote)
		require.NotNil(t, placeholder)
		require.Equal(t, uncachedRemoteNote.AttributedTo, placeholder.ID)
		require.NotEqual(t, cfg.BaseURL()+"/users/alice", placeholder.ID)

		localInfo := h.getEmbedAuthorInfo(liftCtx, localNote)
		require.NotNil(t, localInfo.actor)
		require.Equal(t, "alice", localInfo.username)
		require.Equal(t, "Local alice", localInfo.name)

		cachedRemoteInfo := h.getEmbedAuthorInfo(liftCtx, cachedRemoteNote)
		require.NotNil(t, cachedRemoteInfo.actor)
		require.Equal(t, remoteActor.ID, cachedRemoteInfo.actor.ID)
		require.Equal(t, "alice", cachedRemoteInfo.username)
		require.Equal(t, "Remote alice", cachedRemoteInfo.name)

		placeholderInfo := h.getEmbedAuthorInfo(liftCtx, uncachedRemoteNote)
		require.NotNil(t, placeholderInfo.actor)
		require.Equal(t, uncachedRemoteNote.AttributedTo, placeholderInfo.actor.ID)
		require.NotEqual(t, cfg.BaseURL()+"/users/alice", placeholderInfo.actor.ID)

		fallbackHandler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		fallbackActor := fallbackHandler.getOEmbedAuthorActor(liftCtx, localNote)
		require.NotNil(t, fallbackActor)
		require.Equal(t, localNote.AttributedTo, fallbackActor.ID)
		require.Equal(t, "alice", fallbackActor.PreferredUsername)

		fallbackInfo := fallbackHandler.getEmbedAuthorInfo(liftCtx, localNote)
		require.Nil(t, fallbackInfo.actor)
		require.Equal(t, "alice", fallbackInfo.username)
	})

	t.Run("notification status author extraction keeps remote and local paths distinct", func(t *testing.T) {
		local := h.extractStatusAuthor(liftCtx, localNote)
		require.NotNil(t, local)
		require.Equal(t, cfg.BaseURL()+"/users/alice", local.ID)

		cachedRemote := h.extractStatusAuthor(liftCtx, cachedRemoteNote)
		require.NotNil(t, cachedRemote)
		require.Equal(t, remoteActor.ID, cachedRemote.ID)

		placeholder := h.extractStatusAuthor(liftCtx, uncachedRemoteNote)
		require.NotNil(t, placeholder)
		require.Equal(t, uncachedRemoteNote.AttributedTo, placeholder.ID)
		require.NotEqual(t, cfg.BaseURL()+"/users/alice", placeholder.ID)
	})
}

func TestL17StatusInfoSourceGuardDoesNotTreatRemoteAuthorAsLocalOwner(t *testing.T) {
	cfg := round11TestConfig()
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"authorization": "Bearer " + token}

	localStatus := &storagemodels.Status{
		StatusID: "local-source",
		AuthorID: cfg.BaseURL() + "/users/alice",
		Note: &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: cfg.BaseURL() + "/objects/local-source", Type: activitypub.NoteType},
			AttributedTo: cfg.BaseURL() + "/users/alice",
			Content:      "local source",
		},
	}
	remoteStatus := &storagemodels.Status{
		StatusID: "remote-source",
		AuthorID: "https://remote.example/users/alice",
		Note: &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: "https://remote.example/objects/remote-source", Type: activitypub.NoteType},
			AttributedTo: "https://remote.example/users/alice",
			Content:      "remote source",
		},
	}
	notesSvc := &NotesServiceStub{
		GetNoteWithViewerFunc: func(_ context.Context, query *notes.GetNoteQuery) (*storagemodels.Status, error) {
			switch query.StatusID {
			case "local-source":
				return localStatus, nil
			case "remote-source":
				return remoteStatus, nil
			default:
				return nil, errors.New("not found")
			}
		},
	}
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesSvc})

	localCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/local-source/source", headers, nil, nil)
	require.NoError(t, err)
	localCtx.Params["id"] = "local-source"
	requireStatus(t, http.StatusOK)(h.HandleGetStatusSourceLift(localCtx))

	remoteCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/remote-source/source", headers, nil, nil)
	require.NoError(t, err)
	remoteCtx.Params["id"] = "remote-source"
	requireStatus(t, http.StatusNotFound)(h.HandleGetStatusSourceLift(remoteCtx))
}

func TestL17StatusLookupObjectIDsGuardSkipsRemoteAuthorURLProjection(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	localIDs := h.statusLookupObjectIDs(&storagemodels.Status{
		StatusID: "local-1",
		AuthorID: cfg.BaseURL() + "/users/alice",
	})
	require.Contains(t, localIDs, cfg.BaseURL()+"/users/alice/statuses/local-1")
	require.Contains(t, localIDs, cfg.BaseURL()+"/objects/local-1")
	require.Contains(t, localIDs, "local-1")

	remoteIDs := h.statusLookupObjectIDs(&storagemodels.Status{
		StatusID: "remote-1",
		AuthorID: "https://remote.example/users/alice",
	})
	require.NotContains(t, remoteIDs, cfg.BaseURL()+"/users/alice/statuses/remote-1")
	require.NotContains(t, remoteIDs, cfg.BaseURL()+"/objects/remote-1")
	require.Equal(t, []string{"remote-1"}, remoteIDs)
}
