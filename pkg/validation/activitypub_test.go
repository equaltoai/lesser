package validation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityPubValidator_ValidateActivity_MissingRequiredFields(t *testing.T) {
	v := NewActivityPubValidator(zap.NewNop())
	_, err := v.ValidateActivity([]byte(`{}`), &Config{
		MaxObjectSize:   1024,
		MaxStringLength: 100,
		MaxArrayLength:  10,
		AllowedTypes:    []string{"Create"},
		RequiredFields:  []string{"type", "actor"},
		AllowLocalURLs:  true,
		MaxDepth:        2,
		AllowedIRI:      []string{"https"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing required field")
}

func TestActivityPubValidator_ValidateActivity_RejectsLocalhostActor(t *testing.T) {
	t.Setenv("ENVIRONMENT", "")

	v := NewActivityPubValidator(zap.NewNop())
	payload, err := json.Marshal(Activity{
		Type:  "Create",
		Actor: "http://localhost/actor",
	})
	require.NoError(t, err)

	_, err = v.ValidateActivity(payload, &Config{
		MaxObjectSize:   1024,
		MaxStringLength: 100,
		MaxArrayLength:  10,
		AllowedTypes:    []string{"Create"},
		RequiredFields:  []string{"type", "actor"},
		AllowLocalURLs:  false,
		MaxDepth:        2,
		AllowedIRI:      []string{"https", "http"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "localhost URLs not allowed")
}

func TestActivityPubValidator_ValidateActivity_AllowsValidActorWhenLocalChecksDisabled(t *testing.T) {
	t.Setenv("ENVIRONMENT", "")

	v := NewActivityPubValidator(zap.NewNop())
	payload, err := json.Marshal(Activity{
		Type:  "Create",
		Actor: "https://example.com/actor",
	})
	require.NoError(t, err)

	activity, err := v.ValidateActivity(payload, &Config{
		MaxObjectSize:   1024,
		MaxStringLength: 100,
		MaxArrayLength:  10,
		AllowedTypes:    []string{"Create"},
		RequiredFields:  []string{"type", "actor"},
		AllowLocalURLs:  true,
		MaxDepth:        2,
		AllowedIRI:      []string{"https"},
	})
	require.NoError(t, err)
	require.Equal(t, "Create", activity.Type)
	require.Equal(t, "https://example.com/actor", activity.Actor)
}
