package repositories

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

func TestApplyUserUpdates_PrimitiveFields(t *testing.T) {
	repo := &AccountRepository{}
	original := time.Now().Add(-1 * time.Hour)
	user := &models.User{
		Email:              "old@example.com",
		Approved:           false,
		Suspended:          true,
		Role:               "user",
		RequireNSFWWarning: true,
		UpdatedAt:          original,
	}

	updates := map[string]interface{}{
		"email":                "new@example.com",
		"approved":             true,
		AccountStatusSuspended: false,
		"role":                 "admin",
		"require_nsfw_warning": false,
	}

	require.NoError(t, repo.applyUserUpdates(user, updates))
	require.Equal(t, "new@example.com", user.Email)
	require.True(t, user.Approved)
	require.False(t, user.Suspended)
	require.Equal(t, "admin", user.Role)
	require.False(t, user.RequireNSFWWarning)
	require.True(t, user.UpdatedAt.After(original))
}

func TestApplyUserUpdates_ComplexCollections(t *testing.T) {
	repo := &AccountRepository{}
	user := &models.User{}

	updates := map[string]interface{}{
		"fields": []interface{}{
			map[string]interface{}{
				"name":  "Website",
				"value": "https://example.com",
			},
			map[string]string{
				"name":  "Patreon",
				"value": "patreon.com/example",
			},
		},
		"recovery_methods": []interface{}{
			"email",
			"webauthn",
		},
		"metadata": map[string]interface{}{
			"theme": "dark",
		},
	}

	require.NoError(t, repo.applyUserUpdates(user, updates))
	require.Len(t, user.Fields, 2)
	require.Equal(t, map[string]string{"name": "Website", "value": "https://example.com"}, user.Fields[0])
	require.Equal(t, map[string]string{"name": "Patreon", "value": "patreon.com/example"}, user.Fields[1])
	require.Equal(t, []string{"email", "webauthn"}, user.RecoveryMethods)
	require.Equal(t, map[string]interface{}{
		"theme": "dark",
	}, user.Metadata)

	// ensure original maps can mutate without affecting stored values
	fieldsInput := updates["fields"].([]interface{})
	fieldsInput[0].(map[string]interface{})["value"] = "changed"
	user.Fields[0]["value"] = "https://example.com"
	updates["metadata"].(map[string]interface{})["theme"] = "light"
	require.Equal(t, "https://example.com", user.Fields[0]["value"])
	require.Equal(t, map[string]interface{}{"theme": "dark"}, user.Metadata)
}

func TestApplyUserUpdates_InvalidTypes(t *testing.T) {
	repo := &AccountRepository{}
	user := &models.User{Approved: false}

	err := repo.applyUserUpdates(user, map[string]interface{}{
		"approved": "true",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "approved")
	require.False(t, user.Approved)
}
