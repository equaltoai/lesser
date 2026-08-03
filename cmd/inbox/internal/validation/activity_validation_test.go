package validation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

func TestValidateRequestBody(t *testing.T) {
	logger := zap.NewNop()

	err := ValidateRequestBody(logger, nil)
	require.Error(t, err)
	appErr, ok := pkgErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, pkgErrors.CodeRequiredFieldMissing, appErr.Code)
	require.Equal(t, 400, appErr.HTTPStatusCode)

	err = ValidateRequestBody(logger, make([]byte, common.MaxActivitySize+1))
	require.Error(t, err)
	require.IsType(t, &apptheory.AppTheoryError{}, err)
	require.Equal(t, "app.too_large", err.(*apptheory.AppTheoryError).Code)
}

func TestValidateRequestBody_Round24_AcceptsValidBody(t *testing.T) {
	require.NoError(t, ValidateRequestBody(zap.NewNop(), []byte("ok")))
}

func TestParseActivity_InvalidTimestamp(t *testing.T) {
	logger := zap.NewNop()

	raw := map[string]any{
		"@context":  activitypub.Context,
		"type":      activitypub.CreateType,
		"id":        "https://example.com/activities/1",
		"actor":     "https://remote.example/users/bob",
		"to":        []string{"https://example.com/users/alice"},
		"published": "not-a-time",
		"object":    "https://example.com/objects/1",
	}
	body, err := json.Marshal(raw)
	require.NoError(t, err)

	_, err = ParseActivity(logger, body)
	require.Error(t, err)
	appErr, ok := pkgErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, pkgErrors.CodeValidationFailed, appErr.Code)
	require.Equal(t, 400, appErr.HTTPStatusCode)
}

func TestParseActivity_InvalidJSON(t *testing.T) {
	logger := zap.NewNop()

	_, err := ParseActivity(logger, []byte("not-json"))
	require.Error(t, err)
	appErr, ok := pkgErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, pkgErrors.CodeValidationFailed, appErr.Code)
	require.Equal(t, 400, appErr.HTTPStatusCode)
}

func TestParseActivity_ValidActivity(t *testing.T) {
	logger := zap.NewNop()

	raw := map[string]any{
		"@context":  activitypub.Context,
		"type":      activitypub.CreateType,
		"id":        "https://example.com/activities/1",
		"actor":     "https://remote.example/users/bob",
		"to":        []string{"https://example.com/users/alice"},
		"published": "2026-01-01T00:00:00Z",
		"object": map[string]any{
			"type":    "Note",
			"id":      "https://example.com/objects/1",
			"content": "hello",
		},
	}
	body, err := json.Marshal(raw)
	require.NoError(t, err)

	activity, err := ParseActivity(logger, body)
	require.NoError(t, err)
	require.NotNil(t, activity)
	require.Equal(t, "https://example.com/activities/1", activity.ID)
	require.Equal(t, "https://remote.example/users/bob", activity.Actor)
}

func TestValidateActorUsername(t *testing.T) {
	require.Error(t, ValidateActorUsername("https://example.com/"))
	require.NoError(t, ValidateActorUsername("https://example.com/users/alice"))
}

func TestValidateActorUsername_InvalidURL(t *testing.T) {
	err := ValidateActorUsername("http://[::1")
	require.Error(t, err)
	appErr, ok := pkgErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, pkgErrors.CodeValidationFailed, appErr.Code)
	require.Equal(t, 400, appErr.HTTPStatusCode)
}

func TestIsAddressedTo(t *testing.T) {
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		Inbox: "https://example.com/users/alice/inbox",
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Type: activitypub.CreateType,
			ID:   "https://example.com/activities/1",
			To:   []string{actor.ID},
		},
		Actor:  "https://remote.example/users/bob",
		Object: "https://example.com/objects/1",
	}

	require.True(t, IsAddressedTo(activity, actor))
}

func TestIsAddressedTo_AcceptsCCBToBCCAndPublicAddress(t *testing.T) {
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		Inbox: "https://example.com/users/alice/inbox",
	}

	base := activitypub.BaseObject{
		Type: activitypub.CreateType,
		ID:   "https://example.com/activities/1",
	}

	t.Run("cc inbox", func(t *testing.T) {
		bo := base
		bo.CC = []string{actor.Inbox}
		activity := &activitypub.Activity{
			BaseObject: bo,
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.True(t, IsAddressedTo(activity, actor))
	})

	t.Run("bto actor id", func(t *testing.T) {
		bo := base
		bo.BTo = []string{actor.ID}
		activity := &activitypub.Activity{
			BaseObject: bo,
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.True(t, IsAddressedTo(activity, actor))
	})

	t.Run("bcc actor id", func(t *testing.T) {
		bo := base
		bo.BCC = []string{actor.ID}
		activity := &activitypub.Activity{
			BaseObject: bo,
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.True(t, IsAddressedTo(activity, actor))
	})

	t.Run("public address", func(t *testing.T) {
		bo := base
		bo.To = []string{activitypub.PublicAddress}
		activity := &activitypub.Activity{
			BaseObject: bo,
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.True(t, IsAddressedTo(activity, actor))
	})

	t.Run("to inbox", func(t *testing.T) {
		bo := base
		bo.To = []string{actor.Inbox}
		activity := &activitypub.Activity{
			BaseObject: bo,
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.True(t, IsAddressedTo(activity, actor))
	})

	t.Run("not addressed", func(t *testing.T) {
		bo := base
		bo.To = []string{"https://elsewhere.example/users/other"}
		bo.CC = []string{"https://elsewhere.example/users/other"}
		bo.BTo = []string{"https://elsewhere.example/users/other"}
		bo.BCC = []string{"https://elsewhere.example/users/other"}
		activity := &activitypub.Activity{
			BaseObject: bo,
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.False(t, IsAddressedTo(activity, actor))
	})
}

func TestValidateBasicActivityAndAddressing(t *testing.T) {
	t.Run("basic activity uses custom context", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.ContextValue{"https://www.w3.org/ns/activitystreams"},
				ID:      "https://example.com/activities/1",
				Type:    activitypub.CreateType,
				To:      []string{"https://example.com/users/alice"},
			},
			Actor:  "https://remote.example/users/bob",
			Object: "https://example.com/objects/1",
		}

		require.NoError(t, ValidateBasicActivity(activity))
		require.NoError(t, ValidateActivityAddressing(activity))
	})

	t.Run("invalid activity is rejected", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "",
				Type: activitypub.CreateType,
			},
			Actor:  "https://remote.example/users/bob",
			Object: "https://example.com/objects/1",
		}

		err := ValidateBasicActivity(activity)
		require.Error(t, err)
		appErr, ok := pkgErrors.AsAppError(err)
		require.True(t, ok)
		require.Equal(t, pkgErrors.CodeValidationFailed, appErr.Code)
	})

	t.Run("invalid addressing is rejected", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/activities/1",
				Type: activitypub.CreateType,
				BTo:  []string{"not-a-url"},
			},
			Actor:  "https://remote.example/users/bob",
			Object: "https://example.com/objects/1",
		}

		err := ValidateActivityAddressing(activity)
		require.Error(t, err)
	})
}

func TestValidateBasicActorAndPublicKey(t *testing.T) {
	t.Run("basic actor is accepted", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice",
				Type: activitypub.PersonType,
			},
			PreferredUsername: "alice",
			Inbox:             "https://example.com/users/alice/inbox",
			Outbox:            "https://example.com/users/alice/outbox",
		}

		require.NoError(t, ValidateBasicActor(actor))
		require.NoError(t, ValidateActorPublicKey(actor))
	})

	t.Run("invalid actor is rejected", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "not-a-url",
				Type: activitypub.PersonType,
			},
			PreferredUsername: "alice",
			Inbox:             "not-a-url",
			Outbox:            "not-a-url",
		}

		err := ValidateBasicActor(actor)
		require.Error(t, err)
	})

	t.Run("invalid public key is rejected", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice",
				Type: activitypub.PersonType,
			},
			PreferredUsername: "alice",
			Inbox:             "https://example.com/users/alice/inbox",
			Outbox:            "https://example.com/users/alice/outbox",
			PublicKey: &activitypub.PublicKey{
				ID:           "not-a-url",
				Owner:        "not-a-url",
				PublicKeyPem: "not-a-pem",
			},
		}

		err := ValidateActorPublicKey(actor)
		require.Error(t, err)
		appErr, ok := pkgErrors.AsAppError(err)
		require.True(t, ok)
		require.Equal(t, pkgErrors.CodeValidationFailed, appErr.Code)
	})
}

func TestValidateCreateActivityObject_Branches(t *testing.T) {
	t.Run("non-Create activities are ignored", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/activities/1",
				Type: activitypub.UpdateType,
			},
			Actor:  "https://remote.example/users/bob",
			Object: map[string]any{"type": "Note"},
		}

		require.NoError(t, ValidateCreateActivityObject(activity))
	})

	t.Run("Create activities with non-map objects are ignored", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/activities/1",
				Type: activitypub.CreateType,
			},
			Actor:  "https://remote.example/users/bob",
			Object: "https://example.com/objects/1",
		}

		require.NoError(t, ValidateCreateActivityObject(activity))
	})

	t.Run("invalid attachments are rejected", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/activities/1",
				Type: activitypub.CreateType,
			},
			Actor: "https://remote.example/users/bob",
			Object: map[string]any{
				"type":       "Note",
				"attachment": "bad",
			},
		}

		require.Error(t, ValidateCreateActivityObject(activity))
	})

	t.Run("invalid tags are rejected", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/activities/1",
				Type: activitypub.CreateType,
			},
			Actor: "https://remote.example/users/bob",
			Object: map[string]any{
				"type": "Note",
				"tag":  123,
			},
		}

		require.Error(t, ValidateCreateActivityObject(activity))
	})

	t.Run("invalid Note objects are rejected", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/activities/1",
				Type: activitypub.CreateType,
			},
			Actor: "https://remote.example/users/bob",
			Object: map[string]any{
				"type": "Note",
			},
		}

		require.Error(t, ValidateCreateActivityObject(activity))
	})

	t.Run("valid Note objects are accepted", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/activities/1",
				Type: activitypub.CreateType,
			},
			Actor: "https://remote.example/users/bob",
			Object: map[string]any{
				"@context":     []interface{}(activitypub.Context),
				"type":         "Note",
				"id":           "https://example.com/objects/1",
				"content":      "hello",
				"attributedTo": "https://remote.example/users/bob",
				"published":    "2026-01-01T00:00:00Z",
				"to":           []interface{}{activitypub.PublicAddress},
				"cc":           []interface{}{"https://example.com/users/alice"},
				"inReplyTo":    "https://example.com/objects/0",
				"summary":      "cw",
			},
		}

		require.NoError(t, ValidateCreateActivityObject(activity))
	})
}

func TestValidateComprehensiveAddressingAndTargeting(t *testing.T) {
	logger := zap.NewNop()

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
	}

	t.Run("invalid addressing fails comprehensive validation", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/activities/1",
				Type: activitypub.CreateType,
				To:   []string{"not-a-url"},
			},
			Actor:  "https://remote.example/users/bob",
			Object: "https://example.com/objects/1",
		}

		require.Error(t, ValidateComprehensiveAddressing(logger, activity))
	})

	t.Run("targeting requires activity to be addressed to actor", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/activities/1",
				Type: activitypub.CreateType,
				To:   []string{"https://elsewhere.example/users/other"},
			},
			Actor:  "https://remote.example/users/bob",
			Object: "https://example.com/objects/1",
		}

		require.Error(t, ValidateActivityTargeting(logger, activity, actor))
	})

	t.Run("full validation succeeds for minimal valid payload", func(t *testing.T) {
		published := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:        "https://example.com/activities/1",
				Type:      activitypub.CreateType,
				To:        []string{actor.ID},
				Published: &published,
			},
			Actor:  "https://remote.example/users/bob",
			Object: "https://example.com/objects/1",
		}

		require.NoError(t, ValidateActivity(logger, activity, actor))
	})
}

func TestParseActivity_InvalidIDURL_Round24(t *testing.T) {
	logger := zap.NewNop()

	raw := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.CreateType,
		"id":       "not-a-url",
		"actor":    "https://remote.example/users/bob",
		"to":       []string{"https://example.com/users/alice"},
		"object":   "https://example.com/objects/1",
	}
	body, err := json.Marshal(raw)
	require.NoError(t, err)

	_, err = ParseActivity(logger, body)
	require.Error(t, err)
	appErr, ok := pkgErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, pkgErrors.CodeValidationFailed, appErr.Code)
	require.Equal(t, 400, appErr.HTTPStatusCode)
}

func TestParseActivity_UnmarshalError_Round24(t *testing.T) {
	logger := zap.NewNop()

	raw := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.CreateType,
		"id":       123,
		"actor":    "https://remote.example/users/bob",
		"to":       []string{"https://example.com/users/alice"},
		"object":   "https://example.com/objects/1",
	}
	body, err := json.Marshal(raw)
	require.NoError(t, err)

	_, err = ParseActivity(logger, body)
	require.Error(t, err)
	appErr, ok := pkgErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, pkgErrors.CodeValidationFailed, appErr.Code)
	require.Equal(t, 400, appErr.HTTPStatusCode)
}

func TestValidateActivity_Round24_InvalidActorUsername(t *testing.T) {
	logger := zap.NewNop()

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
	}

	published := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/activities/1",
			Type:      activitypub.CreateType,
			To:        []string{actor.ID},
			Published: &published,
		},
		Actor:  "https://remote.example/",
		Object: "https://example.com/objects/1",
	}

	err := ValidateActivity(logger, activity, actor)
	require.Error(t, err)
	appErr, ok := pkgErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, pkgErrors.CodeValidationFailed, appErr.Code)
	require.Equal(t, 400, appErr.HTTPStatusCode)
}

func TestValidateActivity_Round24_TargetingFailure(t *testing.T) {
	logger := zap.NewNop()

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
	}

	published := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/activities/1",
			Type:      activitypub.CreateType,
			To:        []string{"https://elsewhere.example/users/other"},
			Published: &published,
		},
		Actor:  "https://remote.example/users/bob",
		Object: "https://example.com/objects/1",
	}

	err := ValidateActivity(logger, activity, actor)
	require.Error(t, err)
	appErr, ok := pkgErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, pkgErrors.CodeValidationFailed, appErr.Code)
	require.Equal(t, 400, appErr.HTTPStatusCode)
}
