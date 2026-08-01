package notes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	testinginmemory "github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestResolveConversationID(t *testing.T) {
	ctx := context.Background()

	t.Run("nil status", func(t *testing.T) {
		assert.Equal(t, "", resolveConversationID(ctx, nil, nil))
	})

	t.Run("existing conversation id", func(t *testing.T) {
		status := &models.Status{
			StatusID:       "status-1",
			ConversationID: "conversation-1",
		}
		assert.Equal(t, "conversation-1", resolveConversationID(ctx, status, nil))
	})

	t.Run("new top level post", func(t *testing.T) {
		status := &models.Status{
			StatusID: "status-2",
		}
		assert.Equal(t, "status-2", resolveConversationID(ctx, status, nil))
	})

	t.Run("reply inherits parent conversation", func(t *testing.T) {
		status := &models.Status{
			StatusID:    "child-status",
			InReplyToID: "parent-status",
		}
		fetcher := func(_ context.Context, _ string) (*models.Status, error) {
			return &models.Status{ConversationID: "parent-conversation"}, nil
		}
		assert.Equal(t, "parent-conversation", resolveConversationID(ctx, status, fetcher))
	})

	t.Run("reply falls back to reply target", func(t *testing.T) {
		status := &models.Status{
			StatusID:    "child-status",
			InReplyToID: "parent-status",
		}
		fetcher := func(_ context.Context, _ string) (*models.Status, error) {
			return nil, errors.New("not found")
		}
		assert.Equal(t, "parent-status", resolveConversationID(ctx, status, fetcher))
	})
}

func TestCanViewDirectMessageMatchesMentionURLs(t *testing.T) {
	service := &Service{domainName: "example.com"}

	status := &models.Status{
		Mentions: []string{"https://example.com/users/bob"},
	}

	assert.True(t, service.canViewDirectMessage(status, "bob"))
}

func TestExtractMentionHandlesSupportsRemoteMentions(t *testing.T) {
	mentions := extractMentionHandles("hi @alice and @bob@remote.example, but not bob@example.com")
	assert.Equal(t, []string{"alice", "bob@remote.example"}, mentions)
}

func TestBuildMentionTagsUsesActorFallbackAndSkipsUnresolved(t *testing.T) {
	service := &Service{
		domainName: "example.com",
		accountRepo: &stubAccountRepo{
			domain:    "example.com",
			missing:   map[string]bool{"missing": true},
			omitActor: map[string]bool{"bob": true},
		},
		logger: zap.NewNop(),
	}

	tags, usernames := service.buildMentionTags(context.Background(), "hi @bob @missing @alice", &storage.Account{
		User: &storage.User{Username: "alice"},
	})

	require.Len(t, tags, 2)
	assert.Equal(t, "https://example.com/users/bob", tags[0].Href)
	assert.Equal(t, "@bob", tags[0].Name)
	assert.Equal(t, "https://example.com/users/alice", tags[1].Href)
	assert.Equal(t, "@alice", tags[1].Name)
	assert.Equal(t, []string{"bob"}, usernames)
}

func TestBuildMentionTagsResolvesCompleteExactUsernameWhenPrefixAccountExists(t *testing.T) {
	accounts := map[string]*storage.Account{
		"prototype": {
			User: &storage.User{Username: "prototype"},
			Actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{ID: "https://example.com/users/prototype"},
			},
		},
		"prototype-11": {
			User: &storage.User{Username: "prototype-11"},
			Actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{ID: "https://example.com/users/prototype-11"},
			},
		},
	}
	service := &Service{
		domainName: "example.com",
		accountRepo: &stubAccountRepo{
			domain:   "example.com",
			accounts: accounts,
		},
		logger: zap.NewNop(),
	}
	author := &storage.Account{User: &storage.User{Username: "prototype-13"}}

	for _, username := range []string{"prototype-11", "prototype"} {
		username := username
		t.Run(username, func(t *testing.T) {
			tags, usernames := service.buildMentionTags(
				context.Background(),
				"public mention @"+username,
				author,
			)

			require.Len(t, tags, 1)
			assert.Equal(t, "https://example.com/users/"+username, tags[0].Href)
			assert.Equal(t, "@"+username, tags[0].Name)
			assert.Equal(t, []string{username}, usernames)
		})
	}
}

func TestNotifyMentionsDeduplicatesRecipientsAndSkipsAuthor(t *testing.T) {
	notifier := &stubNotificationService{}
	service := &Service{
		domainName:    "example.com",
		notifications: notifier,
		logger:        zap.NewNop(),
	}

	service.notifyMentions(context.Background(), &models.Status{
		StatusID:       "status-1",
		AuthorUsername: "alice",
	}, []string{"bob", "Bob", "alice", ""})

	require.Len(t, notifier.cmds, 1)
	assert.Equal(t, "bob", notifier.cmds[0].UserID)
	assert.Equal(t, "alice", notifier.cmds[0].ActorID)
	assert.Equal(t, "mention", notifier.cmds[0].Type)
}

func TestBuildMentionTagsSkipsWhenActorIDCannotBeResolved(t *testing.T) {
	service := &Service{
		accountRepo: &stubAccountRepo{
			omitActor: map[string]bool{"bob": true},
		},
		logger: zap.NewNop(),
	}

	tags, usernames := service.buildMentionTags(context.Background(), "hi @bob", nil)

	assert.Nil(t, tags)
	assert.Nil(t, usernames)
}

func TestBuildMentionTagsResolvesRemoteMentionsViaFederation(t *testing.T) {
	federation := &stubFederation{
		resolved: map[string]*activitypub.Actor{
			"carol@remote.example": {
				BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/carol"},
				PreferredUsername: "carol",
			},
		},
	}
	service := &Service{
		domainName: "example.com",
		accountRepo: &stubAccountRepo{
			domain:  "example.com",
			missing: map[string]bool{"carol@remote.example": true},
		},
		federation: federation,
		logger:     zap.NewNop(),
	}

	tags, usernames := service.buildMentionTags(context.Background(), "hi @carol@remote.example", &storage.Account{
		User: &storage.User{Username: "alice"},
	})

	require.Len(t, tags, 1)
	assert.Equal(t, "https://remote.example/users/carol", tags[0].Href)
	assert.Equal(t, "@carol@remote.example", tags[0].Name)
	assert.Empty(t, usernames)
	assert.Equal(t, []string{"carol@remote.example"}, federation.calls)
}

func TestBuildMentionTagsCapsRemoteMentionResolution(t *testing.T) {
	missing := make(map[string]bool)
	resolved := make(map[string]*activitypub.Actor)
	mentions := make([]string, 0, maxRemoteMentionResolutions+3)
	for i := 0; i < maxRemoteMentionResolutions+3; i++ {
		handle := fmt.Sprintf("user%d@remote.example", i)
		mentions = append(mentions, "@"+handle)
		missing[handle] = true
		resolved[handle] = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: fmt.Sprintf("https://remote.example/users/user%d", i)},
			PreferredUsername: fmt.Sprintf("user%d", i),
		}
	}

	federation := &stubFederation{resolved: resolved}
	service := &Service{
		domainName: "example.com",
		accountRepo: &stubAccountRepo{
			domain:  "example.com",
			missing: missing,
		},
		federation: federation,
		logger:     zap.NewNop(),
	}

	tags, usernames := service.buildMentionTags(context.Background(), strings.Join(mentions, " "), &storage.Account{
		User: &storage.User{Username: "alice"},
	})

	require.Len(t, tags, maxRemoteMentionResolutions)
	assert.Empty(t, usernames)
	require.Len(t, federation.calls, maxRemoteMentionResolutions)
	assert.Equal(t, "user0@remote.example", federation.calls[0])
	assert.Equal(t, fmt.Sprintf("user%d@remote.example", maxRemoteMentionResolutions-1), federation.calls[len(federation.calls)-1])
}

func TestBuildMentionTagsUsesCanonicalRemoteHandleFromCachedAccount(t *testing.T) {
	service := &Service{
		domainName: "example.com",
		accountRepo: &stubAccountRepo{
			domain: "example.com",
			accounts: map[string]*storage.Account{
				"carol@remote.example": {
					User: &storage.User{Username: "carol@remote.example"},
					Actor: &activitypub.Actor{
						BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/carol"},
						PreferredUsername: "carol",
					},
				},
			},
		},
		logger: zap.NewNop(),
	}

	tags, usernames := service.buildMentionTags(context.Background(), "hi @carol@remote.example", &storage.Account{
		User: &storage.User{Username: "alice"},
	})

	require.Len(t, tags, 1)
	assert.Equal(t, "https://remote.example/users/carol", tags[0].Href)
	assert.Equal(t, "@carol@remote.example", tags[0].Name)
	assert.Empty(t, usernames)
}

func TestAddMentionAudienceRespectsVisibility(t *testing.T) {
	mentionTags := []activitypub.Tag{
		{Type: "Mention", Href: "https://remote.example/users/bob", Name: "@bob"},
		{Type: "Mention", Href: "https://remote.example/users/bob", Name: "@bob"},
	}

	t.Run("public adds mentions to cc", func(t *testing.T) {
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				To: []string{activitypub.PublicAddress},
				CC: []string{"https://example.com/users/alice/followers"},
			},
			Visibility: models.VisibilityPublic,
		}

		(&Service{}).addMentionAudience(note, mentionTags)

		assert.Equal(t, []string{activitypub.PublicAddress}, note.To)
		assert.Equal(t, []string{"https://example.com/users/alice/followers", "https://remote.example/users/bob"}, note.CC)
	})

	t.Run("unlisted adds mentions to cc", func(t *testing.T) {
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				To: []string{"https://example.com/users/alice/followers"},
				CC: []string{activitypub.PublicAddress},
			},
			Visibility: models.VisibilityUnlisted,
		}

		(&Service{}).addMentionAudience(note, mentionTags)

		assert.Equal(t, []string{activitypub.PublicAddress, "https://remote.example/users/bob"}, note.CC)
	})

	t.Run("private leaves mentions out of audience", func(t *testing.T) {
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				To: []string{"https://example.com/users/alice/followers"},
			},
			Visibility: models.VisibilityPrivate,
		}

		(&Service{}).addMentionAudience(note, mentionTags)

		assert.Equal(t, []string{"https://example.com/users/alice/followers"}, note.To)
		assert.Empty(t, note.CC)
	})

	t.Run("direct adds mentions to to", func(t *testing.T) {
		note := &activitypub.Note{
			Visibility: models.VisibilityDirect,
		}

		(&Service{}).addMentionAudience(note, mentionTags)

		assert.Equal(t, []string{"https://remote.example/users/bob"}, note.To)
		assert.Empty(t, note.CC)
	})
}

func TestMentionActorIDsAndAudienceHelpers(t *testing.T) {
	t.Run("mention actor ids skip non-mentions blanks and duplicates", func(t *testing.T) {
		recipients := mentionActorIDs([]activitypub.Tag{
			{Type: "Hashtag", Href: "https://example.com/tags/go", Name: "#go"},
			{Type: "Mention", Href: " https://remote.example/users/bob ", Name: "@bob"},
			{Type: "Mention", Href: "", Name: "@missing"},
			{Type: "Mention", Href: "https://remote.example/users/bob", Name: "@bob"},
		})

		assert.Equal(t, []string{"https://remote.example/users/bob"}, recipients)
	})

	t.Run("append unique audience trims and keeps first case-insensitive match", func(t *testing.T) {
		audience := appendUniqueAudience(
			[]string{"https://remote.example/users/alice"},
			" https://remote.example/users/bob ",
			"https://remote.example/users/BOB",
			"",
		)

		assert.Equal(t, []string{
			"https://remote.example/users/alice",
			"https://remote.example/users/bob",
		}, audience)
	})

	t.Run("add mention audience ignores nil note and empty mentions", func(t *testing.T) {
		service := &Service{}
		service.addMentionAudience(nil, []activitypub.Tag{{Type: "Mention", Href: "https://remote.example/users/bob"}})

		note := &activitypub.Note{Visibility: models.VisibilityPublic}
		service.addMentionAudience(note, nil)

		assert.Empty(t, note.To)
		assert.Empty(t, note.CC)
	})
}

func TestMentionHelpersNormalizeURLsAndUsernames(t *testing.T) {
	// Exact actor ID match still works.
	assert.True(t, mentionMatchesViewer("https://example.com/users/bob", "bob", "https://example.com/users/bob"))
	// Bare @username mention matches local viewer username.
	assert.True(t, mentionMatchesViewer("@carol", "carol", "https://example.com/users/carol"))
	// URL mention from a different domain must NOT match a local username.
	assert.False(t, mentionMatchesViewer("https://evil.example/users/alice", "alice", "https://local.example/users/alice"))
	// URL mention from matching domain with matching actor ID must match.
	assert.True(t, mentionMatchesViewer("https://local.example/users/alice", "alice", "https://local.example/users/alice"))
}

type stubPublisher struct {
	userEvents         []streamingEventRecord
	streamEvents       []streamingEventRecord
	conversationEvents []streamingEventRecord
}

type streamingEventRecord struct {
	target string
	event  *streaming.Event
}

func (s *stubPublisher) PublishToUser(_ context.Context, userID string, event *streaming.Event) error {
	s.userEvents = append(s.userEvents, streamingEventRecord{target: userID, event: event})
	return nil
}

func (s *stubPublisher) PublishToStream(_ context.Context, streamName string, event *streaming.Event) error {
	s.streamEvents = append(s.streamEvents, streamingEventRecord{target: streamName, event: event})
	return nil
}

func (s *stubPublisher) PublishToConversation(_ context.Context, conversationID string, event *streaming.Event) error {
	s.conversationEvents = append(s.conversationEvents, streamingEventRecord{target: conversationID, event: event})
	return nil
}

func (s *stubPublisher) Close() error { return nil }

func TestStatusStreamingSharedEventsSanitizeBlindRecipients(t *testing.T) {
	ctx := context.Background()
	pub := &stubPublisher{}
	service := &Service{publisher: pub, logger: zap.NewNop()}

	status := &models.Status{
		StatusID:       "status-1",
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Visibility:     models.VisibilityPublic,
		Hashtags:       []string{"security"},
		BtoRecipients:  []string{"https://example.com/users/hidden-to"},
		BccRecipients:  []string{"https://example.com/users/hidden-bcc"},
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:  "https://example.com/objects/status-1",
				BTo: []string{"https://example.com/users/hidden-to"},
				BCC: []string{"https://example.com/users/hidden-bcc"},
			},
			AttributedTo: "https://example.com/users/alice",
			Content:      "hello",
		},
	}

	service.emitStatusCreatedEvents(ctx, status)

	require.NotEmpty(t, pub.userEvents)
	userStatus := pub.userEvents[0].event.Payload["entity"].(*models.Status)
	require.NotEmpty(t, userStatus.BtoRecipients)
	require.NotEmpty(t, userStatus.BccRecipients)

	require.NotEmpty(t, pub.streamEvents)
	for _, record := range pub.streamEvents {
		sharedStatus := record.event.Payload["entity"].(*models.Status)
		require.Empty(t, sharedStatus.BtoRecipients, "stream %s leaked bto recipients", record.target)
		require.Empty(t, sharedStatus.BccRecipients, "stream %s leaked bcc recipients", record.target)
		require.NotNil(t, sharedStatus.Note)
		require.Empty(t, sharedStatus.Note.BTo, "stream %s leaked note bto", record.target)
		require.Empty(t, sharedStatus.Note.BCC, "stream %s leaked note bcc", record.target)
	}
}

func TestStatusStreamingPrivateHashtagsStayOutOfSharedStreams(t *testing.T) {
	ctx := context.Background()
	pub := &stubPublisher{}
	service := &Service{publisher: pub, logger: zap.NewNop()}

	status := &models.Status{
		StatusID:       "status-private",
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Visibility:     models.VisibilityPrivate,
		Hashtags:       []string{"private"},
	}

	service.emitStatusCreatedEvents(ctx, status)

	require.NotEmpty(t, pub.userEvents)
	require.Empty(t, pub.streamEvents)
}

func TestStatusStreamingConversationEventsSanitizeBlindRecipients(t *testing.T) {
	ctx := context.Background()
	pub := &stubPublisher{}
	service := &Service{publisher: pub, logger: zap.NewNop()}

	status := &models.Status{
		StatusID:       "status-direct",
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Visibility:     models.VisibilityDirect,
		ConversationID: "conversation-1",
		BtoRecipients:  []string{"https://example.com/users/hidden-to"},
		BccRecipients:  []string{"https://example.com/users/hidden-bcc"},
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:  "https://example.com/objects/status-direct",
				BTo: []string{"https://example.com/users/hidden-to"},
				BCC: []string{"https://example.com/users/hidden-bcc"},
			},
			AttributedTo: "https://example.com/users/alice",
		},
	}

	service.emitStatusCreatedEvents(ctx, status)

	require.Len(t, pub.conversationEvents, 2)
	for _, record := range pub.conversationEvents {
		sharedStatus := record.event.Payload["entity"].(*models.Status)
		require.Empty(t, sharedStatus.BtoRecipients)
		require.Empty(t, sharedStatus.BccRecipients)
		require.NotNil(t, sharedStatus.Note)
		require.Empty(t, sharedStatus.Note.BTo)
		require.Empty(t, sharedStatus.Note.BCC)
	}
}

type stubNotificationService struct {
	cmds []*notifications.CreateNotificationCommand
}

func (s *stubNotificationService) CreateNotification(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
	s.cmds = append(s.cmds, cmd)
	return &notifications.NotificationResult{}, nil
}

type stubFederation struct {
	activities []*activitypub.Activity
	resolved   map[string]*activitypub.Actor
	resolveErr map[string]error
	calls      []string
}

func (s *stubFederation) QueueActivity(_ context.Context, activity *activitypub.Activity) error {
	s.activities = append(s.activities, activity)
	return nil
}

func (s *stubFederation) ResolveActor(_ context.Context, handle string) (*activitypub.Actor, error) {
	s.calls = append(s.calls, handle)
	if s.resolveErr != nil && s.resolveErr[handle] != nil {
		return nil, s.resolveErr[handle]
	}
	if s.resolved != nil && s.resolved[handle] != nil {
		return s.resolved[handle], nil
	}
	return nil, errors.New("not found")
}

type replyParentResolutionCall struct {
	raw        string
	visibility string
	authorID   string
	username   string
}

type stubReplyParentResolver struct {
	resolved map[string]*ResolvedReplyParent
	err      error
	calls    []replyParentResolutionCall
}

func (s *stubReplyParentResolver) ResolveQuoteTarget(_ context.Context, author *storage.Account, rawQuoteTarget string) (*ResolvedReplyParent, error) {
	call := replyParentResolutionCall{raw: rawQuoteTarget}
	if author != nil {
		if author.User != nil {
			call.username = author.User.Username
		}
		if author.Actor != nil {
			call.authorID = author.Actor.ID
		}
	}
	s.calls = append(s.calls, call)

	if s.err != nil {
		return nil, s.err
	}
	if s.resolved != nil {
		return s.resolved[rawQuoteTarget], nil
	}
	return nil, nil
}

func TestResolveQuoteTargetAppliesViewerAccessAfterResolution(t *testing.T) {
	ctx := context.Background()
	statusRepo := testinginmemory.NewStatusRepository()
	now := time.Now().UTC()
	target := &models.Status{
		StatusID:       "remote-direct",
		AuthorID:       "https://remote.example/users/bob",
		AuthorUsername: "bob@remote.example",
		Visibility:     models.VisibilityDirect,
		ToRecipients:   []string{"https://example.com/users/carol"},
		PublishedAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
		ModifiedAt:     now,
		Note: &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: "https://remote.example/users/bob/statuses/direct", Type: activitypub.NoteType},
			AttributedTo: "https://remote.example/users/bob",
			Visibility:   models.VisibilityDirect,
		},
	}
	require.NoError(t, statusRepo.CreateStatus(ctx, target))

	resolver := &stubReplyParentResolver{resolved: map[string]*ResolvedReplyParent{
		target.Note.ID: {
			Status:             target,
			CanonicalStatusID:  target.StatusID,
			CanonicalObjectURL: target.Note.ID,
			Visibility:         target.Visibility,
			Fetched:            true,
			Remote:             true,
		},
	}}
	service := &Service{
		noteRepo:     statusRepo,
		accountRepo:  &stubAccountRepo{domain: "example.com"},
		replyParents: resolver,
		domainName:   "example.com",
		logger:       zap.NewNop(),
	}

	_, err := service.ResolveQuoteTarget(ctx, "alice", target.Note.ID)
	appErr, ok := apperrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, apperrors.CodeNotFound, appErr.Code)
	require.Len(t, resolver.calls, 1)
	require.Equal(t, "alice", resolver.calls[0].username)

	target.ToRecipients = []string{"https://example.com/users/alice"}
	require.NoError(t, statusRepo.UpdateStatus(ctx, target))
	visible, err := service.ResolveQuoteTarget(ctx, "alice", target.Note.ID)
	require.NoError(t, err)
	require.Equal(t, target.StatusID, visible.StatusID)
}

func (s *stubReplyParentResolver) ResolveReplyParent(_ context.Context, author *storage.Account, rawInReplyTo string, requestedVisibility string) (*ResolvedReplyParent, error) {
	call := replyParentResolutionCall{
		raw:        rawInReplyTo,
		visibility: requestedVisibility,
	}
	if author != nil {
		if author.User != nil {
			call.username = author.User.Username
		}
		if author.Actor != nil {
			call.authorID = author.Actor.ID
		}
	}
	s.calls = append(s.calls, call)

	if s.err != nil {
		return nil, s.err
	}
	if s.resolved != nil {
		return s.resolved[rawInReplyTo], nil
	}
	return nil, nil
}

func TestEmitReblogEventsPublishesBoostedEvents(t *testing.T) {
	publisher := &stubPublisher{}
	service := &Service{
		publisher: publisher,
		logger:    zap.NewNop(),
	}

	status := &models.Status{
		StatusID:       "status-boost",
		AuthorUsername: "author",
		Visibility:     VisibilityPublic,
		ReblogCount:    3,
	}

	events := service.emitReblogEvents(context.Background(), status, "booster")
	require.Len(t, events, 2)
	require.Len(t, publisher.userEvents, 1)
	require.Len(t, publisher.streamEvents, 1)

	assert.Equal(t, streaming.StatusBoosted, publisher.userEvents[0].event.Type)
	assert.Equal(t, streaming.StatusBoosted, publisher.streamEvents[0].event.Type)

	payloadStatus, ok := publisher.userEvents[0].event.Payload["status"].(*models.Status)
	require.True(t, ok)
	assert.Equal(t, status, payloadStatus)
	assert.Equal(t, 3, payloadStatus.ReblogCount)
}

func TestNotifyBoostCreatesNotification(t *testing.T) {
	notifier := &stubNotificationService{}
	service := &Service{
		logger:        zap.NewNop(),
		notifications: notifier,
		domainName:    "example.com",
	}

	status := &models.Status{
		StatusID:       "status-123",
		AuthorUsername: "author",
	}

	service.notifyBoost(context.Background(), status, "booster")

	require.Len(t, notifier.cmds, 1)
	cmd := notifier.cmds[0]
	assert.Equal(t, "author", cmd.UserID)
	assert.Equal(t, "booster", cmd.ActorID)
	assert.Equal(t, "status-123", cmd.TargetID)
	assert.Equal(t, "reblog", cmd.Type)
}

func TestBoostStatusIDFromAnnounceID(t *testing.T) {
	first := boostStatusIDFromAnnounceID("https://example.com/activities/123")
	second := boostStatusIDFromAnnounceID("https://example.com/activities/123")
	third := boostStatusIDFromAnnounceID("https://example.com/activities/456")

	require.NotEmpty(t, first)
	require.Equal(t, first, second)
	require.NotEqual(t, first, third)
}

func TestDeriveBoostStatusIDUsesActorFallback(t *testing.T) {
	original := &models.Status{StatusID: "orig-42"}
	booster := &storage.Account{
		User: &storage.User{Username: "bob"},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"},
		},
	}

	announce := &storage.Announce{ID: ""}
	id := deriveBoostStatusID(original, booster, announce)

	require.NotEmpty(t, id)
	assert.Equal(t, boostStatusIDFromActors(booster.Actor.ID, original.StatusID), id)
}

func TestBuildBoostStatusCopiesMetadata(t *testing.T) {
	service := &Service{}
	original := &models.Status{
		StatusID:       "orig-1",
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Visibility:     VisibilityPublic,
		Sensitive:      true,
		Language:       "en",
		ConversationID: "conv-1",
		ToRecipients:   []string{"https://www.w3.org/ns/activitystreams#Public"},
		CcRecipients:   []string{"https://example.com/users/alice/followers"},
	}

	booster := &storage.Account{
		User: &storage.User{
			Username: "bob",
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"},
		},
	}

	announce := &storage.Announce{
		ID:        "https://example.com/activities/boost-1",
		Published: time.Unix(100, 0),
	}

	boost := service.buildBoostStatus(original, booster, announce)
	require.NotNil(t, boost)
	assert.Equal(t, "bob", boost.AuthorUsername)
	assert.Equal(t, booster.Actor.ID, boost.AuthorID)
	assert.Equal(t, original.Visibility, boost.Visibility)
	assert.Equal(t, original.Sensitive, boost.Sensitive)
	assert.Equal(t, original.Language, boost.Language)
	assert.Equal(t, original.ConversationID, boost.ConversationID)
	assert.Equal(t, original.ReblogCount, boost.ReblogCount)
	assert.Equal(t, original.ToRecipients, boost.ToRecipients)
	assert.Equal(t, original.CcRecipients, boost.CcRecipients)
	assert.Equal(t, original.StatusID, boost.ReblogOfID)
	assert.Equal(t, original.StatusID, boost.BoostOfStatusID)
	assert.Equal(t, original.AuthorID, boost.BoostOfAuthorID)
	assert.Equal(t, announce.ID, boost.BoostAnnounceID)
	assert.Equal(t, announce.Published, boost.PublishedAt)
}

func TestQueueAnnounceActivityEnqueuesFederation(t *testing.T) {
	fed := &stubFederation{}
	service := &Service{
		logger:     zap.NewNop(),
		federation: fed,
	}

	now := time.Now()
	announce := &storage.Announce{
		ID:        "announce-1",
		Actor:     "https://example.com/users/booster",
		Object:    "https://example.com/users/author/statuses/status-1",
		Published: now,
	}
	status := &models.Status{
		StatusID: "status-1",
		ToRecipients: []string{
			"https://example.com/users/author/followers",
		},
	}

	service.queueAnnounceActivity(context.Background(), status, announce)

	require.Len(t, fed.activities, 1)
	assert.Equal(t, activitypub.AnnounceType, fed.activities[0].Type)
	assert.Equal(t, announce.Actor, fed.activities[0].Actor)
	assert.Equal(t, announce.Object, fed.activities[0].Object)
}
