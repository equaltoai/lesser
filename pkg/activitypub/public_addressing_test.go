package activitypub

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsPublicAddressedActivity(t *testing.T) {
	t.Run("nil activity", func(t *testing.T) {
		require.False(t, IsPublicAddressedActivity(nil))
	})

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

	t.Run("embedded note value", func(t *testing.T) {
		activity := &Activity{Object: Note{BaseObject: BaseObject{CC: []string{PublicAddress}}}}
		require.True(t, IsPublicAddressedActivity(activity))
	})

	t.Run("embedded map", func(t *testing.T) {
		activity := &Activity{Object: map[string]any{"cc": []any{PublicAddress}}}
		require.True(t, IsPublicAddressedActivity(activity))
	})

	t.Run("embedded map string", func(t *testing.T) {
		activity := &Activity{Object: map[string]any{"to": PublicAddress}}
		require.True(t, IsPublicAddressedActivity(activity))
	})

	t.Run("embedded map string slice", func(t *testing.T) {
		activity := &Activity{Object: map[string]any{"to": []string{PublicAddress}}}
		require.True(t, IsPublicAddressedActivity(activity))
	})

	t.Run("unsupported embedded object", func(t *testing.T) {
		activity := &Activity{Object: struct{}{}}
		require.False(t, IsPublicAddressedActivity(activity))
	})

	t.Run("private", func(t *testing.T) {
		activity := &Activity{BaseObject: BaseObject{To: []string{"https://example.com/users/bob"}}}
		require.False(t, IsPublicAddressedActivity(activity))
	})
}

func TestHasPublicAddressValue(t *testing.T) {
	t.Run("non public string", func(t *testing.T) {
		require.False(t, hasPublicAddressValue("https://example.com/users/alice"))
	})

	t.Run("mixed any slice without public", func(t *testing.T) {
		require.False(t, hasPublicAddressValue([]any{"https://example.com/users/alice", 7}))
	})
}
