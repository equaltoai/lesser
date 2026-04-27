package activitypub

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsPublicAddressedActivity(t *testing.T) {
	t.Run("top level to", func(t *testing.T) {
		activity := &Activity{BaseObject: BaseObject{To: []string{PublicAddress}}}
		require.True(t, IsPublicAddressedActivity(activity))
	})

	t.Run("top level cc", func(t *testing.T) {
		activity := &Activity{BaseObject: BaseObject{CC: []string{PublicAddress}}}
		require.True(t, IsPublicAddressedActivity(activity))
	})

	t.Run("embedded note", func(t *testing.T) {
		activity := &Activity{Object: &Note{BaseObject: BaseObject{To: []string{PublicAddress}}}}
		require.True(t, IsPublicAddressedActivity(activity))
	})

	t.Run("embedded map", func(t *testing.T) {
		activity := &Activity{Object: map[string]any{"cc": []any{PublicAddress}}}
		require.True(t, IsPublicAddressedActivity(activity))
	})

	t.Run("private", func(t *testing.T) {
		activity := &Activity{BaseObject: BaseObject{To: []string{"https://example.com/users/bob"}}}
		require.False(t, IsPublicAddressedActivity(activity))
	})
}
