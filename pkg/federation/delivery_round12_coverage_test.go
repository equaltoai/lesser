package federation

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestDeliveryHelpers_RecipientsAndDomains(t *testing.T) {
	d := &DeliveryService{logger: zap.NewNop()}

	actorID := "https://example.com/users/alice"
	followers := "https://example.com/users/alice/followers"

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "act-1",
			Type: "Create",
			To:   []string{followers},
		},
	}
	assert.True(t, d.isExplicitRecipient(activity, actorID))
	assert.False(t, d.isExplicitRecipient(activity, "https://example.com/users/bob"))

	assert.True(t, isLocalActor("https://example.com/users/bob", "https://example.com/users/alice"))
	assert.False(t, isLocalActor("https://remote.example/users/bob", "https://example.com/users/alice"))

	assert.Equal(t, "example.com", extractDomain(actorID))
	assert.Equal(t, "not-a-url", extractDomain("not-a-url"))

	assert.Equal(t, "@alice@example.com", extractHandleFromActorID(actorID, "alice"))
	assert.Equal(t, "", extractHandleFromActorID("https://example.com/users/alice", ""))

	assert.Equal(t, "example.com", extractDomainFromURL("https://example.com/inbox"))
	assert.Equal(t, "unknown", extractDomainFromURL("://bad"))

	assert.True(t, d.isLocalRecipient("https://example.com/users/bob", "https://example.com/users/alice"))
	assert.False(t, d.isLocalRecipient("https://remote.example/users/bob", "https://example.com/users/alice"))

	assert.Equal(t, 2, maxInt(1, 2))
	assert.Equal(t, 2, maxInt(2, 1))
}

func TestDeliverActivityWithPrivacy_SkipsNonRecipientsForDirectMessages(t *testing.T) {
	d := &DeliveryService{logger: zap.NewNop()}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "act-1",
			Type: "Create",
			To:   []string{"https://remote.example/users/bob"},
		},
	}

	// Direct message (no Public, no followers collection) should skip if target isn't explicit recipient.
	err := d.DeliverActivityWithPrivacy(
		context.Background(),
		activity,
		"https://remote.example/inbox",
		&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}},
		"https://remote.example/users/charlie",
	)
	assert.NoError(t, err)
}
