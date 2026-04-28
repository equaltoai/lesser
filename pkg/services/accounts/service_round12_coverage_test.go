package accounts

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type federationRecorder struct {
	calls int
	err   error
}

func (f *federationRecorder) QueueActivity(ctx context.Context, activity *activitypub.Activity) error {
	_ = ctx
	_ = activity
	f.calls++
	return f.err
}

func TestService_normalizeUsername(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty", input: "   ", expected: ""},
		{name: "plain username", input: " Alice ", expected: "alice"},
		{name: "acct prefix", input: "acct:alice", expected: "alice"},
		{name: "leading at", input: "@alice", expected: "alice"},
		{name: "acct + leading at + local domain", input: "acct:@alice@example.com", expected: "alice"},
		{name: "local handle", input: "alice@example.com", expected: "alice"},
		{name: "remote handle", input: "Alice@Remote.Social", expected: "alice@remote.social"},
		{name: "users url", input: "https://example.com/users/Alice", expected: "alice"},
		{name: "at url", input: "https://example.com/@Alice", expected: "alice"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, svc.normalizeUsername(tt.input))
		})
	}
}

func TestService_normalizeBaseURL(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	assert.Equal(t, "", svc.normalizeBaseURL(""))
	assert.Equal(t, "https://example.com", svc.normalizeBaseURL("example.com/"))
	assert.Equal(t, "https://example.com", svc.normalizeBaseURL("  example.com/// "))
	assert.Equal(t, "http://example.com", svc.normalizeBaseURL("http://example.com///"))
	assert.Equal(t, "https://example.com", svc.normalizeBaseURL("https://example.com/"))
}

func TestService_validateRegisterAccountCommand_DisallowsEmail(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	err := svc.validateRegisterAccountCommand(context.Background(), &RegisterAccountCommand{
		Username:   "alice",
		Email:      "somevalue",
		Agreement:  true,
		InviteCode: "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "email is not supported")
}

func TestService_validateRegisterAccountCommand_RequiresAgreement(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	err := svc.validateRegisterAccountCommand(context.Background(), &RegisterAccountCommand{
		Username:  "alice",
		Email:     "",
		Agreement: false,
	})
	assert.ErrorIs(t, err, ErrMustAgreeToTerms)
}

func TestService_initialPostingVisibility(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	assert.Equal(t, "private", svc.initialPostingVisibility("PRIVATE"))
	assert.Equal(t, "public", svc.initialPostingVisibility(""))
}

func TestService_validateUpdatePreferencesCommand_InvalidValues(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	err := svc.validateUpdatePreferencesCommand(context.Background(), &UpdatePreferencesCommand{
		Username:                 "alice",
		UpdaterID:                "alice",
		DefaultPostingVisibility: "invalid",
	})
	assert.Error(t, err)

	err = svc.validateUpdatePreferencesCommand(context.Background(), &UpdatePreferencesCommand{
		Username:    "alice",
		UpdaterID:   "alice",
		ExpandMedia: "nope",
	})
	assert.ErrorIs(t, err, ErrInvalidExpandMediaSetting)

	err = svc.validateUpdatePreferencesCommand(context.Background(), &UpdatePreferencesCommand{
		Username:               "alice",
		UpdaterID:              "alice",
		PreferredTimelineOrder: "nope",
	})
	assert.ErrorIs(t, err, ErrInvalidTimelineOrder)
}

func TestService_updateAccountProfile_InitializesActorAndSetsFields(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	account := &storage.Account{
		User: &storage.User{
			Username: "alice",
		},
		Actor: nil,
	}

	cmd := &UpdateProfileCommand{
		Username:    "alice",
		UpdaterID:   "alice",
		DisplayName: "Alice",
		Bio:         "bio",
		Avatar:      "https://cdn.example.com/a.png",
		Header:      "https://cdn.example.com/h.png",
		Locked:      true,
		Bot:         true,
		Fields: []ProfileField{
			{Name: "Website", Value: "https://example.com"},
		},
		Discoverable: true,
	}

	err := svc.updateAccountProfile(account, cmd)
	assert.NoError(t, err)
	assert.Equal(t, "Alice", account.User.DisplayName)
	assert.Equal(t, "bio", account.User.Note)
	assert.Equal(t, "https://cdn.example.com/a.png", account.User.Avatar)
	assert.Equal(t, "https://cdn.example.com/h.png", account.User.Header)
	assert.True(t, account.User.Locked)
	assert.True(t, account.User.Discoverable)

	if assert.NotNil(t, account.Actor) {
		assert.Equal(t, "Service", account.Actor.Type) // Bot account
		assert.Equal(t, "Alice", account.Actor.Name)
		assert.Equal(t, "bio", account.Actor.Summary)
		assert.True(t, account.Actor.ManuallyApprovesFollowers)
		assert.True(t, account.Actor.Discoverable)
		if assert.NotNil(t, account.Actor.Icon) {
			assert.Equal(t, "https://cdn.example.com/a.png", account.Actor.Icon.URL)
		}
		if assert.NotNil(t, account.Actor.Image) {
			assert.Equal(t, "https://cdn.example.com/h.png", account.Actor.Image.URL)
		}
		assert.Len(t, account.Actor.Attachment, 1)
	}
}

func TestService_updateAccountProfile_SanitizesBioAndFieldValues(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	account := &storage.Account{
		User: &storage.User{
			Username: "alice",
		},
	}

	cmd := &UpdateProfileCommand{
		Username:    "alice",
		UpdaterID:   "alice",
		DisplayName: "Alice",
		Bio:         `<script>alert(1)</script><img src=x onerror=alert(1)>bio<a href="javascript:alert(1)" onclick="alert(1)">x</a>`,
		Fields: []ProfileField{
			{Name: "Website", Value: `<a href="javascript:alert(1)" onclick="alert(1)">click</a>`},
		},
	}

	err := svc.updateAccountProfile(account, cmd)
	assert.NoError(t, err)

	assert.NotContains(t, account.User.Note, "<script")
	assert.NotContains(t, account.User.Note, "onerror=")
	assert.NotContains(t, account.User.Note, "onclick=")
	assert.NotContains(t, account.User.Note, "javascript:")

	assert.NotContains(t, account.Actor.Summary, "<script")
	assert.NotContains(t, account.Actor.Summary, "onerror=")
	assert.NotContains(t, account.Actor.Summary, "onclick=")
	assert.NotContains(t, account.Actor.Summary, "javascript:")

	if assert.Len(t, account.User.Fields, 1) {
		assert.NotContains(t, account.User.Fields[0]["value"], "javascript:")
		assert.NotContains(t, account.User.Fields[0]["value"], "onclick=")
	}
	if assert.Len(t, account.Actor.Attachment, 1) {
		assert.NotContains(t, account.Actor.Attachment[0].Value, "javascript:")
		assert.NotContains(t, account.Actor.Attachment[0].Value, "onclick=")
	}
}

func TestService_sanitizeAccountForViewer(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	account := &storage.Account{
		User: &storage.User{
			Username:     "alice",
			Email:        "somevalue",
			PasswordHash: "hash",
			Silenced:     true,
		},
		Actor: &activitypub.Actor{
			Summary: "original",
		},
	}

	// Other viewer: hide internal fields and redact silenced content.
	sanitized := svc.sanitizeAccountForViewer(account, "bob")
	assert.NotNil(t, sanitized)
	assert.Equal(t, "alice", sanitized.User.Username)
	assert.Empty(t, sanitized.User.Email)
	assert.Empty(t, sanitized.User.PasswordHash)
	assert.Equal(t, "[Content hidden]", sanitized.Actor.Summary)

	// Owner viewer: no redaction of internal fields.
	ownerView := svc.sanitizeAccountForViewer(account, "alice")
	assert.NotNil(t, ownerView)
	assert.Equal(t, "somevalue", ownerView.User.Email)
	assert.Equal(t, "hash", ownerView.User.PasswordHash)
}

func TestService_emitAccountUpdatedEvents_PublishesToUserAndFollowers(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")
	ctx := context.Background()

	account := &storage.Account{
		User: &storage.User{
			Username: "alice",
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice",
				Type: "Person",
			},
		},
	}

	events := svc.emitAccountUpdatedEvents(ctx, account)
	assert.NotEmpty(t, events)

	var sawUserStream bool
	var sawFollowersStream bool
	for _, event := range events {
		if event == nil {
			continue
		}
		if event.Stream == "user:alice" {
			sawUserStream = true
		}
		if event.Stream == "followers:alice" {
			sawFollowersStream = true
		}
	}
	assert.True(t, sawUserStream)
	assert.True(t, sawFollowersStream)
}

func TestService_emitAccountUpdatedEvents_SanitizesFollowersPayload(t *testing.T) {
	ctx := context.Background()
	publisher := new(MockPublisher)
	svc := NewService(nil, publisher, nil, nil, nil, zap.NewNop(), "example.com")

	account := &storage.Account{
		User: &storage.User{
			ID:                 "u1",
			Username:           "alice",
			Email:              "alice@example.com",
			PasswordHash:       "hash",
			RecoveryMethods:    []string{"wallet"},
			Metadata:           map[string]interface{}{"auth_hint": "private"},
			Locale:             "en",
			Role:               "admin",
			AllowNSFW:          true,
			RequireNSFWWarning: true,
			AgentOwner:         "owner",
			AgentCreatedBy:     "creator",
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice",
				Type: "Person",
			},
			PreferredUsername: "alice",
		},
		PrivateKey: "private-key",
	}

	publisher.On("PublishToUser", ctx, "alice", mock.AnythingOfType("*streaming.Event")).Return(nil).Once()
	publisher.On("PublishToStream", ctx, "followers:alice", mock.AnythingOfType("*streaming.Event")).Return(nil).Once()

	events := svc.emitAccountUpdatedEvents(ctx, account)
	assert.Len(t, events, 2)

	var userAccount *storage.Account
	var followersAccount *storage.Account
	for _, event := range events {
		if event.Stream == "user:alice" {
			userAccount, _ = event.Payload["entity"].(*storage.Account)
		}
		if event.Stream == "followers:alice" {
			followersAccount, _ = event.Payload["entity"].(*storage.Account)
		}
	}

	require.NotNil(t, userAccount)
	require.NotNil(t, followersAccount)
	assert.Equal(t, "alice@example.com", userAccount.User.Email)
	assert.Equal(t, "hash", userAccount.User.PasswordHash)
	assert.Empty(t, followersAccount.PrivateKey)
	assert.Empty(t, followersAccount.User.Email)
	assert.Empty(t, followersAccount.User.PasswordHash)
	assert.Empty(t, followersAccount.User.RecoveryMethods)
	assert.Empty(t, followersAccount.User.Metadata)
	assert.Empty(t, followersAccount.User.Locale)
	assert.Empty(t, followersAccount.User.Role)
	assert.False(t, followersAccount.User.AllowNSFW)
	assert.False(t, followersAccount.User.RequireNSFWWarning)
	assert.Empty(t, followersAccount.User.AgentOwner)
	assert.Empty(t, followersAccount.User.AgentCreatedBy)
	assert.Equal(t, "alice", followersAccount.Actor.PreferredUsername)
	publisher.AssertExpectations(t)
}

func TestService_emitAccountUpdatedEvents_ToleratesPublishFailures(t *testing.T) {
	ctx := context.Background()
	account := &storage.Account{
		User: &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice",
				Type: "Person",
			},
		},
	}

	t.Run("user publish failure still attempts followers stream", func(t *testing.T) {
		publisher := new(MockPublisher)
		svc := NewService(nil, publisher, nil, nil, nil, zap.NewNop(), "example.com")

		publisher.On("PublishToUser", ctx, "alice", mock.AnythingOfType("*streaming.Event")).Return(errors.New("boom")).Once()
		publisher.On("PublishToStream", ctx, "followers:alice", mock.AnythingOfType("*streaming.Event")).Return(nil).Once()

		events := svc.emitAccountUpdatedEvents(ctx, account)
		assert.Len(t, events, 1)
		assert.Equal(t, "followers:alice", events[0].Stream)
		publisher.AssertExpectations(t)
	})

	t.Run("followers publish failure still returns user event", func(t *testing.T) {
		publisher := new(MockPublisher)
		svc := NewService(nil, publisher, nil, nil, nil, zap.NewNop(), "example.com")

		publisher.On("PublishToUser", ctx, "alice", mock.AnythingOfType("*streaming.Event")).Return(nil).Once()
		publisher.On("PublishToStream", ctx, "followers:alice", mock.AnythingOfType("*streaming.Event")).Return(errors.New("boom")).Once()

		events := svc.emitAccountUpdatedEvents(ctx, account)
		assert.Len(t, events, 1)
		assert.Equal(t, "user:alice", events[0].Stream)
		publisher.AssertExpectations(t)
	})
}

func TestService_emitPreferencesUpdatedEvents_PublishesToUserOnly(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")
	ctx := context.Background()

	events := svc.emitPreferencesUpdatedEvents(ctx, "alice", map[string]interface{}{"language": "en"})
	if assert.Len(t, events, 1) {
		assert.Equal(t, "user:alice", events[0].Stream)
	}
}

func TestService_emitPreferencesUpdatedEvents_PublishFailureReturnsNoEvents(t *testing.T) {
	ctx := context.Background()
	publisher := new(MockPublisher)
	svc := NewService(nil, publisher, nil, nil, nil, zap.NewNop(), "example.com")

	publisher.On("PublishToUser", ctx, "alice", mock.AnythingOfType("*streaming.Event")).Return(errors.New("boom")).Once()

	events := svc.emitPreferencesUpdatedEvents(ctx, "alice", map[string]interface{}{"theme": "dark"})
	assert.Empty(t, events)
	publisher.AssertExpectations(t)
}

func TestService_emitAccountCreatedEvents_SkipsWhenPublisherNil(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, zap.NewNop(), "example.com")
	events := svc.emitAccountCreatedEvents(context.Background(), &storage.Account{User: &storage.User{Username: "alice"}})
	assert.Empty(t, events)
}

func TestService_queueFederationUpdate(t *testing.T) {
	ctx := context.Background()
	account := &storage.Account{
		User: &storage.User{
			Username: "alice",
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice",
				Type: "Person",
			},
		},
	}

	t.Run("no federation configured", func(t *testing.T) {
		svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")
		assert.NotPanics(t, func() {
			svc.queueFederationUpdate(ctx, account)
		})
	})

	t.Run("queues update activity", func(t *testing.T) {
		fed := &federationRecorder{}
		svc := NewService(nil, streaming.NewMockPublisher(), fed, nil, nil, zap.NewNop(), "example.com")
		svc.queueFederationUpdate(ctx, account)
		assert.Equal(t, 1, fed.calls)
	})

	t.Run("queue error is swallowed", func(t *testing.T) {
		fed := &federationRecorder{err: errors.New("boom")}
		svc := NewService(nil, streaming.NewMockPublisher(), fed, nil, nil, zap.NewNop(), "example.com")
		assert.NotPanics(t, func() {
			svc.queueFederationUpdate(ctx, account)
		})
		assert.Equal(t, 1, fed.calls)
	})
}

func TestService_isValidPostingVisibility(t *testing.T) {
	assert.True(t, isValidPostingVisibility(models.VisibilityPublic))
	assert.True(t, isValidPostingVisibility(models.VisibilityUnlisted))
	assert.True(t, isValidPostingVisibility(models.VisibilityPrivate))
	assert.True(t, isValidPostingVisibility(models.VisibilityDirect))
	assert.False(t, isValidPostingVisibility("nope"))
}
