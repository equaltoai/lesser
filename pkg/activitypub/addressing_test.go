package activitypub

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddressingValidator_ValidateRecipient(t *testing.T) {
	v := NewAddressingValidator()

	t.Run("empty recipient", func(t *testing.T) {
		err := v.validateRecipient("", "to")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEmptyRecipient)
	})

	t.Run("public address", func(t *testing.T) {
		require.NoError(t, v.validateRecipient(PublicAddress, "to"))
	})

	t.Run("invalid URL", func(t *testing.T) {
		err := v.validateRecipient("http://[::1", "to")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidRecipientURL)
	})

	t.Run("invalid scheme", func(t *testing.T) {
		// url.Parse treats many strings as URLs with custom schemes, so test both an unknown scheme
		// and a well-formed but disallowed scheme.
		err := v.validateRecipient("not-a-url", "to")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidURLScheme)

		err = v.validateRecipient("ftp://example.com/users/alice", "to")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidURLScheme)
	})

	t.Run("invalid recipient format", func(t *testing.T) {
		err := v.validateRecipient("https://example.com/not-a-user", "to")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidRecipientFormat)
	})

	t.Run("valid user URL", func(t *testing.T) {
		require.NoError(t, v.validateRecipient("https://example.com/users/alice", "to"))
	})
}

func TestAddressingValidator_ValidateAddressing_NoteObject(t *testing.T) {
	v := NewAddressingValidator()
	activity := &Activity{
		BaseObject: BaseObject{
			To: []string{"https://example.com/users/alice"},
		},
		Object: &Note{
			BaseObject: BaseObject{
				To: []string{"https://example.com/users/bob"},
			},
		},
	}

	require.NoError(t, v.ValidateAddressing(activity))

	activity.Object.(*Note).To = []string{""}
	err := v.ValidateAddressing(activity)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEmptyRecipient)
}

func TestAddressingValidator_DetermineDeliveryRecipients_GroupsByDomain(t *testing.T) {
	v := NewAddressingValidator()
	activity := &Activity{
		BaseObject: BaseObject{
			To:  []string{"https://example.com/users/alice", PublicAddress},
			CC:  []string{"https://remote.test/users/bob"},
			BTo: []string{"https://example.com/users/carol"},
			BCC: []string{"https://remote.test/users/dave"},
		},
	}

	targets := v.DetermineDeliveryRecipients(activity)
	require.NotNil(t, targets)

	assert.True(t, targets.DirectRecipients["https://example.com/users/alice"])
	assert.True(t, targets.DirectRecipients["https://example.com/users/carol"])
	assert.True(t, targets.DirectRecipients["https://remote.test/users/bob"])
	assert.True(t, targets.DirectRecipients["https://remote.test/users/dave"])
	assert.False(t, targets.DirectRecipients[PublicAddress])

	assert.ElementsMatch(t, []string{"https://example.com/users/alice", "https://remote.test/users/bob", "https://example.com/users/carol", "https://remote.test/users/dave"}, targets.GetAllRecipients())

	require.Contains(t, targets.DomainGroups, "example.com")
	require.Contains(t, targets.DomainGroups, "remote.test")
	assert.Len(t, targets.DomainGroups["example.com"], 2)
	assert.Len(t, targets.DomainGroups["remote.test"], 2)
}

func TestDeliveryTargets_GetAllRecipients_IncludesSharedInboxes(t *testing.T) {
	targets := &DeliveryTargets{
		DirectRecipients: map[string]bool{"https://example.com/users/alice": true},
		SharedInboxes:    map[string]bool{"https://example.com/inbox": true},
		DomainGroups:     map[string][]string{},
	}

	assert.ElementsMatch(t, []string{"https://example.com/users/alice", "https://example.com/inbox"}, targets.GetAllRecipients())
}

func TestAddressingValidator_MessageVisibilityHelpers(t *testing.T) {
	v := NewAddressingValidator()

	t.Run("direct message", func(t *testing.T) {
		activity := &Activity{
			BaseObject: BaseObject{
				To: []string{"https://example.com/users/bob"},
			},
		}
		assert.True(t, v.IsDirectMessage(activity))
		assert.Equal(t, "direct", v.GetVisibilityLevel(activity))
	})

	t.Run("private message (followers-only)", func(t *testing.T) {
		activity := &Activity{
			BaseObject: BaseObject{
				To: []string{"https://example.com/users/alice/followers"},
			},
		}
		assert.True(t, v.IsPrivateMessage(activity))
		assert.True(t, v.IsFollowersOnlyMessage(activity))
		assert.Equal(t, "private", v.GetVisibilityLevel(activity))
	})

	t.Run("public message", func(t *testing.T) {
		activity := &Activity{
			BaseObject: BaseObject{
				To: []string{PublicAddress},
			},
		}
		assert.True(t, v.IsPublicMessage(activity))
		assert.Equal(t, "public", v.GetVisibilityLevel(activity))
	})

	t.Run("unlisted message", func(t *testing.T) {
		activity := &Activity{
			BaseObject: BaseObject{
				To: []string{"https://example.com/users/bob"},
				CC: []string{PublicAddress},
			},
		}
		assert.True(t, v.IsUnlistedMessage(activity))
		assert.Equal(t, "unlisted", v.GetVisibilityLevel(activity))
	})
}

func TestAddressingValidator_SanitizeForDelivery(t *testing.T) {
	v := NewAddressingValidator()

	originalNote := &Note{
		BaseObject: BaseObject{
			To:  []string{"https://example.com/users/bob"},
			BTo: []string{"https://example.com/users/carol"},
			BCC: []string{"https://example.com/users/dave"},
		},
	}
	original := &Activity{
		BaseObject: BaseObject{
			To:  []string{"https://example.com/users/bob"},
			BTo: []string{"https://example.com/users/carol"},
			BCC: []string{"https://example.com/users/dave"},
		},
		Object: originalNote,
	}

	sanitized := v.SanitizeForDelivery(original, true)
	require.NotSame(t, original, sanitized)

	assert.Nil(t, sanitized.BCC)
	assert.Nil(t, sanitized.BTo)

	sanitizedNote, ok := sanitized.Object.(*Note)
	require.True(t, ok)
	assert.Nil(t, sanitizedNote.BCC)
	assert.Nil(t, sanitizedNote.BTo)

	// Original should remain unchanged
	assert.Len(t, original.BCC, 1)
	assert.Len(t, original.BTo, 1)
	require.IsType(t, &Note{}, original.Object)
	assert.Len(t, originalNote.BCC, 1)
	assert.Len(t, originalNote.BTo, 1)
}

func TestAddressingValidator_SanitizeForDelivery_ExcludeBToFalse(t *testing.T) {
	v := NewAddressingValidator()
	original := &Activity{
		BaseObject: BaseObject{
			BTo: []string{"https://example.com/users/carol"},
			BCC: []string{"https://example.com/users/dave"},
		},
	}

	sanitized := v.SanitizeForDelivery(original, false)
	assert.Nil(t, sanitized.BCC)
	assert.Len(t, sanitized.BTo, 1)
}

func TestAddressingValidator_ValidatePrivacyCompliance(t *testing.T) {
	v := NewAddressingValidator()

	t.Run("direct message without public addressing allowed", func(t *testing.T) {
		activity := &Activity{
			BaseObject: BaseObject{
				To: []string{"https://example.com/users/bob"},
			},
		}

		require.NoError(t, v.ValidatePrivacyCompliance(activity))
	})

	t.Run("bcc in visible fields rejected", func(t *testing.T) {
		activity := &Activity{
			BaseObject: BaseObject{
				To:  []string{"https://example.com/users/bob"},
				BCC: []string{"https://example.com/users/bob"},
			},
		}

		err := v.ValidatePrivacyCompliance(activity)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrBCCInVisibleFields))
	})

	t.Run("bcc in cc is rejected", func(t *testing.T) {
		activity := &Activity{
			BaseObject: BaseObject{
				CC:  []string{"https://example.com/users/bob"},
				BCC: []string{"https://example.com/users/bob"},
			},
		}

		err := v.ValidatePrivacyCompliance(activity)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrBCCInVisibleFields))
	})

	t.Run("bcc only in bcc is allowed", func(t *testing.T) {
		activity := &Activity{
			BaseObject: BaseObject{
				To:  []string{"https://example.com/users/bob"},
				BCC: []string{"https://example.com/users/carol"},
			},
		}

		require.NoError(t, v.ValidatePrivacyCompliance(activity))
	})
}

func TestAddressingValidator_isInVisibleFields_CCAndBTo(t *testing.T) {
	v := NewAddressingValidator()
	activity := &Activity{
		BaseObject: BaseObject{
			CC:  []string{"https://example.com/users/alice"},
			BTo: []string{"https://example.com/users/bob"},
		},
	}

	assert.True(t, v.isInVisibleFields(activity, "https://example.com/users/alice"))
	assert.True(t, v.isInVisibleFields(activity, "https://example.com/users/bob"))
	assert.False(t, v.isInVisibleFields(activity, "https://example.com/users/carol"))
}

func TestAddressingValidator_GetDeliveryRecipientsForPrivacy(t *testing.T) {
	v := NewAddressingValidator()
	requestingActor := "https://example.com/users/alice"

	t.Run("public recipients include To/CC/BTo", func(t *testing.T) {
		activity := &Activity{
			BaseObject: BaseObject{
				To:  []string{PublicAddress, "https://example.com/users/bob"},
				CC:  []string{"https://example.com/users/carol"},
				BTo: []string{"https://example.com/users/dave"},
				BCC: []string{"https://example.com/users/erin"},
			},
		}

		recipients := v.GetDeliveryRecipientsForPrivacy(activity, requestingActor)
		assert.ElementsMatch(t, []string{PublicAddress, "https://example.com/users/bob", "https://example.com/users/carol", "https://example.com/users/dave"}, recipients)
	})

	t.Run("private recipients include BTo only for mentioned actor", func(t *testing.T) {
		activity := &Activity{
			BaseObject: BaseObject{
				To:  []string{"https://example.com/users/alice/followers"},
				BTo: []string{"https://example.com/users/bob"},
				BCC: []string{requestingActor},
			},
		}

		recipients := v.GetDeliveryRecipientsForPrivacy(activity, requestingActor)
		assert.ElementsMatch(t, []string{"https://example.com/users/alice/followers", "https://example.com/users/bob"}, recipients)

		recipients = v.GetDeliveryRecipientsForPrivacy(activity, "https://example.com/users/not-mentioned")
		assert.ElementsMatch(t, []string{"https://example.com/users/alice/followers"}, recipients)
	})

	t.Run("direct recipients are only visible to participants", func(t *testing.T) {
		activity := &Activity{
			BaseObject: BaseObject{
				To:  []string{requestingActor},
				BTo: []string{"https://example.com/users/bob"},
			},
		}

		recipients := v.GetDeliveryRecipientsForPrivacy(activity, requestingActor)
		assert.ElementsMatch(t, []string{requestingActor, "https://example.com/users/bob"}, recipients)

		recipients = v.GetDeliveryRecipientsForPrivacy(activity, "https://example.com/users/stranger")
		assert.Empty(t, recipients)
	})
}

func TestAddressingValidator_DetermineDeliveryRecipients_ParseErrorSkipsGrouping(t *testing.T) {
	v := NewAddressingValidator()
	activity := &Activity{
		BaseObject: BaseObject{
			To: []string{"http://[::1"},
		},
	}

	targets := v.DetermineDeliveryRecipients(activity)
	require.NotNil(t, targets)
	assert.True(t, targets.DirectRecipients["http://[::1"])
	assert.Empty(t, targets.DomainGroups)
}
