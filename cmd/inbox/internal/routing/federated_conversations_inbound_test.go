package routing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

type inboundDirectCapturePublisher struct {
	events []streaming.Event
}

func (p *inboundDirectCapturePublisher) PublishToUser(_ context.Context, userID string, event *streaming.Event) error {
	captured := *event
	captured.Stream = fmt.Sprintf("user:%s:direct", userID)
	p.events = append(p.events, captured)
	return nil
}

func (p *inboundDirectCapturePublisher) PublishToStream(_ context.Context, streamName string, event *streaming.Event) error {
	captured := *event
	captured.Stream = streamName
	p.events = append(p.events, captured)
	return nil
}

func (p *inboundDirectCapturePublisher) PublishToConversation(_ context.Context, conversationID string, event *streaming.Event) error {
	captured := *event
	captured.Stream = fmt.Sprintf("conversation:%s", conversationID)
	p.events = append(p.events, captured)
	return nil
}

func (p *inboundDirectCapturePublisher) Close() error {
	return nil
}

func TestInboxHandler_FederatedConversation_RemoteDirectMaterializesStateNotificationAndStream(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()
	notifications := inmemory.NewNotificationRepository()
	conversations := inmemory.NewConversationRepository()
	statuses := inmemory.NewStatusRepository()
	objects := inmemory.NewObjectRepository()
	publisher := &inboundDirectCapturePublisher{}
	env.handler.notificationRepository = notifications
	env.handler.conversationRepository = conversations
	env.handler.statusRepository = statuses
	env.handler.objectRepository = objects
	env.handler.publisher = publisher

	published := time.Date(2026, 4, 27, 14, 0, 0, 0, time.UTC)
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        "https://remote.example/users/bob/statuses/direct-1",
			Type:      activitypub.NoteType,
			Published: &published,
			To:        []string{env.local.ID},
		},
		AttributedTo: env.remoteActorID,
		Content:      "<p>direct hello @alice</p>",
		Tag: []activitypub.Tag{{
			Type: "Mention",
			Href: env.local.ID,
			Name: "@alice@localhost",
		}},
	}
	activity := remoteCreateActivityForNote(t, env.remoteActorID, note, "https://remote.example/activities/direct-1")

	require.NoError(t, env.handler.processRemoteCreateActivity(ctx, activity, env.local))

	statusID := models.CanonicalStatusIDForDomain(note.ID, env.handler.localDomain())
	status, err := statuses.GetStatus(ctx, statusID)
	require.NoError(t, err)
	require.Equal(t, models.VisibilityDirect, status.Visibility)
	require.NotEmpty(t, status.ConversationID)
	require.Equal(t, []string{env.local.ID}, status.ToRecipients)

	stateResult, err := conversations.ListUserConversationStatesByFolder(ctx, "alice", interfaces.UserConversationFolder(models.UserConversationFolderInbox), interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, stateResult.Items, 1)
	state := stateResult.Items[0]
	require.Equal(t, status.ConversationID, state.ConversationID)
	require.Equal(t, env.remoteActorID, state.CounterpartID)
	require.Equal(t, models.ConversationParticipantTypeRemoteActor, state.CounterpartType)
	require.Equal(t, "bob@remote.example", state.CounterpartAcct)
	require.True(t, state.Unread)
	require.Equal(t, statusID, state.PreviewStatusID)

	conversation, err := conversations.GetConversation(ctx, status.ConversationID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"alice", env.remoteActorID}, conversation.Participants)
	require.Len(t, conversation.ParticipantRefs, 2)

	result, err := notifications.GetUserNotifications(ctx, "alice", interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	notification := result.Items[0]
	require.Equal(t, common.NotificationTypeMention, notification.Type)
	require.Equal(t, env.remoteActorID, notification.ActorID)
	require.Equal(t, "remote_actor", notification.ActorType)
	require.Equal(t, statusID, notification.TargetID)
	require.Equal(t, status.ConversationID, notification.Data["conversationID"])
	require.Equal(t, models.VisibilityDirect, notification.Data["visibility"])

	require.Len(t, publisher.events, 2)
	require.Contains(t, []string{publisher.events[0].Stream, publisher.events[1].Stream}, "conversation:"+status.ConversationID)
	require.Contains(t, []string{publisher.events[0].Stream, publisher.events[1].Stream}, "user:alice:direct")
}

func TestInboxHandler_FederatedConversation_UnsupportedRemoteDirectGroupIsNotMaterialized(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()
	notifications := inmemory.NewNotificationRepository()
	conversations := inmemory.NewConversationRepository()
	statuses := inmemory.NewStatusRepository()
	objects := inmemory.NewObjectRepository()
	env.handler.notificationRepository = notifications
	env.handler.conversationRepository = conversations
	env.handler.statusRepository = statuses
	env.handler.objectRepository = objects

	published := time.Date(2026, 4, 27, 14, 30, 0, 0, time.UTC)
	otherRecipient := "https://remote.example/users/carol"
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        "https://remote.example/users/bob/statuses/direct-group-1",
			Type:      activitypub.NoteType,
			Published: &published,
			To:        []string{env.local.ID, otherRecipient},
		},
		AttributedTo: env.remoteActorID,
		Content:      "<p>group direct hello @alice @carol</p>",
		Tag: []activitypub.Tag{{
			Type: "Mention",
			Href: env.local.ID,
			Name: "@alice@localhost",
		}},
	}
	activity := remoteCreateActivityForNote(t, env.remoteActorID, note, "https://remote.example/activities/direct-group-1")

	require.NoError(t, env.handler.processRemoteCreateActivity(ctx, activity, env.local))

	statusID := models.CanonicalStatusIDForDomain(note.ID, env.handler.localDomain())
	_, err := statuses.GetStatus(ctx, statusID)
	require.ErrorIs(t, err, storage.ErrNotFound)

	stateResult, err := conversations.ListUserConversationStatesByFolder(ctx, "alice", interfaces.UserConversationFolder(models.UserConversationFolderInbox), interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, stateResult.Items)

	result, err := notifications.GetUserNotifications(ctx, "alice", interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, result.Items)
}

func TestInboxHandler_FederatedConversation_ClassifyInboundDirectBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	published := time.Date(2026, 4, 27, 15, 0, 0, 0, time.UTC)
	baseNote := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        "https://remote.example/users/bob/statuses/direct-classify",
			Type:      activitypub.NoteType,
			Published: &published,
			To:        []string{env.local.ID},
		},
		AttributedTo: env.remoteActorID,
		Content:      "<p>direct hello</p>",
	}
	baseActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			ID:      "https://remote.example/activities/direct-classify",
			Type:    activitypub.CreateType,
			To:      []string{env.local.ID},
		},
		Object: baseNote,
	}

	info := env.handler.classifyInboundDirectCreate(baseActivity, baseNote, env.local)
	require.True(t, info.isDirect)
	require.False(t, info.unsupportedGroup)
	require.Equal(t, "alice", info.localParticipantID)
	require.Equal(t, env.remoteActorID, info.remoteActorID)
	require.Equal(t, "bob@remote.example", info.remoteAcct)
	require.Len(t, info.participantRefs, 2)

	publicNote := *baseNote
	publicNote.To = []string{activitypub.PublicAddress}
	publicActivity := *baseActivity
	publicActivity.Actor = env.remoteActorID
	publicActivity.To = []string{activitypub.PublicAddress}
	require.False(t, env.handler.classifyInboundDirectCreate(&publicActivity, &publicNote, env.local).isDirect)

	followersNote := *baseNote
	followersNote.To = []string{env.local.ID, env.remoteActorID + "/followers"}
	followersActivity := *baseActivity
	followersActivity.Actor = env.remoteActorID
	followersActivity.To = []string{env.local.ID, env.remoteActorID + "/followers"}
	require.False(t, env.handler.classifyInboundDirectCreate(&followersActivity, &followersNote, env.local).isDirect)

	groupNote := *baseNote
	groupNote.To = []string{env.local.ID, "https://remote.example/users/carol"}
	groupActivity := *baseActivity
	groupActivity.Actor = env.remoteActorID
	groupActivity.To = []string{env.local.ID, "https://remote.example/users/carol"}
	groupInfo := env.handler.classifyInboundDirectCreate(&groupActivity, &groupNote, env.local)
	require.False(t, groupInfo.isDirect)
	require.True(t, groupInfo.unsupportedGroup)
	require.Len(t, groupInfo.specificRecipients, 2)

	require.False(t, env.handler.classifyInboundDirectCreate(nil, baseNote, env.local).isDirect)
	require.False(t, env.handler.classifyInboundDirectCreate(baseActivity, nil, env.local).isDirect)
	require.False(t, env.handler.classifyInboundDirectCreate(baseActivity, baseNote, nil).isDirect)
}

func TestInboxHandler_FederatedConversation_PrepareAndPersistUpdateBranch(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()
	conversations := inmemory.NewConversationRepository()
	statuses := inmemory.NewStatusRepository()
	env.handler.conversationRepository = conversations
	env.handler.statusRepository = statuses

	published := time.Date(2026, 4, 27, 16, 0, 0, 0, time.UTC)
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        "https://remote.example/users/bob/statuses/direct-existing",
			Type:      activitypub.NoteType,
			Published: &published,
			To:        []string{env.local.ID},
		},
		AttributedTo: env.remoteActorID,
		Content:      "<p>existing direct</p>",
	}
	activity := remoteCreateActivityForNote(t, env.remoteActorID, note, "https://remote.example/activities/direct-existing")
	info := env.handler.classifyInboundDirectCreate(activity, note, env.local)
	require.True(t, info.isDirect)

	statusID := models.CanonicalStatusIDForDomain(note.ID, env.handler.localDomain())
	require.NoError(t, statuses.CreateStatus(ctx, &models.Status{
		StatusID:       statusID,
		AuthorID:       env.remoteActorID,
		Content:        note.Content,
		Visibility:     models.VisibilityDirect,
		ConversationID: "conv-from-status",
		PublishedAt:    published,
		CreatedAt:      published,
		UpdatedAt:      published,
	}))

	conversation, createConversation, err := env.handler.prepareInboundDirectConversation(ctx, activity, note, env.local, info)
	require.NoError(t, err)
	require.True(t, createConversation)
	require.Equal(t, "conv-from-status", conversation.ID)

	existing := &models.Conversation{
		ID:              conversation.ID,
		Participants:    models.ConversationParticipantIDsFromRefs(info.participantRefs),
		ParticipantRefs: info.participantRefs,
		CreatedAt:       published.Add(-time.Minute),
		UpdatedAt:       published.Add(-time.Minute),
	}
	require.NoError(t, conversations.CreateConversationWithParticipantStates(ctx, existing, existing.Participants, nil))

	status := &models.Status{
		StatusID:       "remote-direct-update-status",
		ConversationID: conversation.ID,
		PublishedAt:    published.Add(time.Minute),
	}
	require.NoError(t, env.handler.persistInboundDirectConversation(ctx, existing, false, status, info))

	stored, err := conversations.GetConversation(ctx, conversation.ID)
	require.NoError(t, err)
	require.Equal(t, status.StatusID, stored.LastStatusID)
	require.Equal(t, int64(1), stored.TotalMessageCount)

	stateResult, err := conversations.ListUserConversationStatesByFolder(ctx, "alice", interfaces.UserConversationFolder(models.UserConversationFolderInbox), interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, stateResult.Items, 1)
	require.Equal(t, status.StatusID, stateResult.Items[0].PreviewStatusID)
	require.Equal(t, env.remoteActorID, stateResult.Items[0].CounterpartID)
}

func TestInboxHandler_M14_ReplayedRemoteDirectDoesNotOverwriteConversationMetadata(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()
	notifications := inmemory.NewNotificationRepository()
	conversations := inmemory.NewConversationRepository()
	statuses := inmemory.NewStatusRepository()
	objects := inmemory.NewObjectRepository()
	publisher := &inboundDirectCapturePublisher{}
	env.handler.notificationRepository = notifications
	env.handler.conversationRepository = conversations
	env.handler.statusRepository = statuses
	env.handler.objectRepository = objects
	env.handler.publisher = publisher

	originalPublished := time.Date(2026, 4, 27, 18, 0, 0, 0, time.UTC)
	replayPublished := originalPublished.Add(-2 * time.Hour)
	noteID := "https://remote.example/users/bob/statuses/direct-replay"
	statusID := models.CanonicalStatusIDForDomain(noteID, env.handler.localDomain())
	conversationID := "conv-replay-protected"
	participantRefs := models.NormalizeConversationParticipantRefs([]models.ConversationParticipantRef{
		{
			ParticipantType: models.ConversationParticipantTypeLocalUser,
			ParticipantID:   "alice",
		},
		{
			ParticipantType: models.ConversationParticipantTypeRemoteActor,
			ParticipantID:   env.remoteActorID,
			Acct:            "bob@remote.example",
			Domain:          "remote.example",
			ResolvedAt:      &originalPublished,
		},
	})
	participants := models.ConversationParticipantIDsFromRefs(participantRefs)
	existingConversation := &models.Conversation{
		ID:                conversationID,
		Participants:      participants,
		ParticipantRefs:   participantRefs,
		LastStatusID:      "newer-direct-status",
		LastMessageTime:   originalPublished.Add(time.Hour),
		TotalMessageCount: 7,
		CreatedAt:         originalPublished,
		UpdatedAt:         originalPublished.Add(time.Hour),
	}
	require.NoError(t, conversations.CreateConversationWithParticipantStates(ctx, existingConversation, participants, nil))
	require.NoError(t, statuses.CreateStatus(ctx, &models.Status{
		StatusID:       statusID,
		AuthorID:       env.remoteActorID,
		Content:        "<p>original direct</p>",
		Visibility:     models.VisibilityDirect,
		ConversationID: conversationID,
		PublishedAt:    originalPublished,
		CreatedAt:      originalPublished,
		UpdatedAt:      originalPublished,
	}))

	forgedActor := "https://evil.example/users/eve"
	replayedNote := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        noteID,
			Type:      activitypub.NoteType,
			Published: &replayPublished,
			To:        []string{env.local.ID},
		},
		AttributedTo: forgedActor,
		Content:      "<p>replayed direct attempting metadata overwrite</p>",
	}
	activity := remoteCreateActivityForNote(t, forgedActor, replayedNote, "https://evil.example/activities/replay-direct")

	require.NoError(t, env.handler.processRemoteCreateActivity(ctx, activity, env.local))

	stored, err := conversations.GetConversation(ctx, conversationID)
	require.NoError(t, err)
	require.Equal(t, "newer-direct-status", stored.LastStatusID)
	require.Equal(t, int64(7), stored.TotalMessageCount)
	require.Equal(t, originalPublished.Add(time.Hour), stored.LastMessageTime)
	require.ElementsMatch(t, participants, stored.Participants)
	require.Equal(t, participantRefs, stored.ParticipantRefs)

	result, err := notifications.GetUserNotifications(ctx, "alice", interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, result.Items)
	require.Empty(t, publisher.events)
}

func TestInboxHandler_L14_EqualTimestampUniqueDirectMessageAdvancesMetadata(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()
	notifications := inmemory.NewNotificationRepository()
	conversations := inmemory.NewConversationRepository()
	statuses := inmemory.NewStatusRepository()
	objects := inmemory.NewObjectRepository()
	publisher := &inboundDirectCapturePublisher{}
	env.handler.notificationRepository = notifications
	env.handler.conversationRepository = conversations
	env.handler.statusRepository = statuses
	env.handler.objectRepository = objects
	env.handler.publisher = publisher

	published := time.Date(2026, 4, 28, 13, 0, 0, 0, time.UTC)
	conversationID := "conv-equal-unique"
	participantRefs := models.NormalizeConversationParticipantRefs([]models.ConversationParticipantRef{
		{
			ParticipantType: models.ConversationParticipantTypeLocalUser,
			ParticipantID:   "alice",
		},
		{
			ParticipantType: models.ConversationParticipantTypeRemoteActor,
			ParticipantID:   env.remoteActorID,
			Acct:            "bob@remote.example",
			Domain:          "remote.example",
			ResolvedAt:      &published,
		},
	})
	participants := models.ConversationParticipantIDsFromRefs(participantRefs)
	existingConversation := &models.Conversation{
		ID:                conversationID,
		Participants:      participants,
		ParticipantRefs:   participantRefs,
		LastStatusID:      "first-equal-timestamp-status",
		LastMessageTime:   published,
		TotalMessageCount: 7,
		CreatedAt:         published.Add(-24 * time.Hour),
		UpdatedAt:         published.Add(-time.Minute),
	}
	require.NoError(t, conversations.CreateConversationWithParticipantStates(ctx, existingConversation, participants, nil))

	equalNoteID := "https://remote.example/users/bob/statuses/direct-equal-unique"
	equalStatusID := models.CanonicalStatusIDForDomain(equalNoteID, env.handler.localDomain())
	equalNote := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        equalNoteID,
			Type:      activitypub.NoteType,
			Published: &published,
			To:        []string{env.local.ID},
		},
		AttributedTo: env.remoteActorID,
		Content:      "<p>equal timestamp unique direct message @alice</p>",
		Tag: []activitypub.Tag{{
			Type: "Mention",
			Href: env.local.ID,
			Name: "@alice@localhost",
		}},
	}
	equalActivity := remoteCreateActivityForNote(t, env.remoteActorID, equalNote, "https://remote.example/activities/direct-equal-unique")

	require.NoError(t, env.handler.processRemoteCreateActivity(ctx, equalActivity, env.local))

	stored, err := conversations.GetConversation(ctx, conversationID)
	require.NoError(t, err)

	require.Equal(t, int64(8), stored.TotalMessageCount)
	require.Equal(t, equalStatusID, stored.LastStatusID)
	require.Equal(t, published, stored.LastMessageTime)
	require.Equal(t, published, stored.UpdatedAt)
	require.ElementsMatch(t, participants, stored.Participants)
	require.Equal(t, participantRefs, stored.ParticipantRefs)

	stateResult, err := conversations.ListUserConversationStatesByFolder(ctx, "alice", interfaces.UserConversationFolder(models.UserConversationFolderInbox), interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, stateResult.Items, 1)
	require.Equal(t, equalStatusID, stateResult.Items[0].PreviewStatusID)
	require.Equal(t, published, stateResult.Items[0].PreviewStatusPublishedAt)
	require.Equal(t, published, stateResult.Items[0].SortAt)
}

// TestInboxHandler_M14_OlderUniqueDirectMessageIncrementsCountPreservesMetadata verifies
// CSR-042: a new unique inbound direct message with an older publishedAt still increments
// TotalMessageCount while preserving the conversation's LastStatusID / LastMessageTime /
// UpdatedAt. Exact-replay idempotency (same activity / status) is already covered by the
// status-level skip in prepareDirectCreateState and the separate replay test above.
func TestInboxHandler_M14_OlderUniqueDirectMessageIncrementsCountPreservesMetadata(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()
	notifications := inmemory.NewNotificationRepository()
	conversations := inmemory.NewConversationRepository()
	statuses := inmemory.NewStatusRepository()
	objects := inmemory.NewObjectRepository()
	publisher := &inboundDirectCapturePublisher{}
	env.handler.notificationRepository = notifications
	env.handler.conversationRepository = conversations
	env.handler.statusRepository = statuses
	env.handler.objectRepository = objects
	env.handler.publisher = publisher

	newerPublished := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	conversationID := "conv-older-unique"
	participantRefs := models.NormalizeConversationParticipantRefs([]models.ConversationParticipantRef{
		{
			ParticipantType: models.ConversationParticipantTypeLocalUser,
			ParticipantID:   "alice",
		},
		{
			ParticipantType: models.ConversationParticipantTypeRemoteActor,
			ParticipantID:   env.remoteActorID,
			Acct:            "bob@remote.example",
			Domain:          "remote.example",
			ResolvedAt:      &newerPublished,
		},
	})
	participants := models.ConversationParticipantIDsFromRefs(participantRefs)
	existingConversation := &models.Conversation{
		ID:                conversationID,
		Participants:      participants,
		ParticipantRefs:   participantRefs,
		LastStatusID:      "newer-direct-status",
		LastMessageTime:   newerPublished,
		TotalMessageCount: 7,
		CreatedAt:         newerPublished.Add(-24 * time.Hour),
		UpdatedAt:         newerPublished,
	}
	require.NoError(t, conversations.CreateConversationWithParticipantStates(ctx, existingConversation, participants, nil))

	// Send a unique message with an older publishedAt.
	olderPublished := newerPublished.Add(-2 * time.Hour)
	olderNoteID := "https://remote.example/users/bob/statuses/direct-older-unique"
	olderNote := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        olderNoteID,
			Type:      activitypub.NoteType,
			Published: &olderPublished,
			To:        []string{env.local.ID},
		},
		AttributedTo: env.remoteActorID,
		Content:      "<p>older unique direct message @alice</p>",
		Tag: []activitypub.Tag{{
			Type: "Mention",
			Href: env.local.ID,
			Name: "@alice@localhost",
		}},
	}
	olderActivity := remoteCreateActivityForNote(t, env.remoteActorID, olderNote, "https://remote.example/activities/direct-older-unique")

	require.NoError(t, env.handler.processRemoteCreateActivity(ctx, olderActivity, env.local))

	stored, err := conversations.GetConversation(ctx, conversationID)
	require.NoError(t, err)

	// CSR-042: TotalMessageCount must increment for a unique older message.
	require.Equal(t, int64(8), stored.TotalMessageCount)

	// Last-message metadata must be preserved when the incoming publishedAt is older.
	require.Equal(t, "newer-direct-status", stored.LastStatusID)
	require.Equal(t, newerPublished, stored.LastMessageTime)
	require.Equal(t, newerPublished, stored.UpdatedAt)

	// Participant integrity must be preserved.
	require.ElementsMatch(t, participants, stored.Participants)
	require.Equal(t, participantRefs, stored.ParticipantRefs)
}

func TestInboxHandler_FederatedConversation_HelperFallbackBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()
	require.Empty(t, env.handler.remoteNotificationActorLabel(""))

	fallbackRef := inboundDirectRemoteParticipantRef(inboundDirectCreateInfo{
		remoteActorID: "https://remote.example/users/fallback",
		remoteAcct:    "fallback@remote.example",
		remoteDomain:  "remote.example",
	})
	require.Equal(t, models.ConversationParticipantTypeRemoteActor, fallbackRef.ParticipantType)
	require.Equal(t, "https://remote.example/users/fallback", fallbackRef.ParticipantID)

	status := &models.Status{StatusID: "status-early", PublishedAt: time.Now().UTC()}
	conversation := &models.Conversation{ID: "conv-early"}
	env.handler.createRemoteDirectNotification(ctx, nil, env.local, nil, status, nil, inboundDirectCreateInfo{})
	env.handler.createRemoteDirectNotification(ctx, nil, env.local, nil, status, conversation, inboundDirectCreateInfo{})
	env.handler.createRemoteDirectNotification(ctx, nil, &activitypub.Actor{}, nil, status, conversation, inboundDirectCreateInfo{
		localParticipantID: "",
		remoteActorID:      "",
	})
	env.handler.emitInboundDirectConversationEvent(ctx, conversation, status, "alice")
}
