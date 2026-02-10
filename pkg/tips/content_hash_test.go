package tips

import "testing"

func TestContentHashFromObjectID(t *testing.T) {
	t.Run("returns keccak256 hex for id", func(t *testing.T) {
		const want = "0x1c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac8"
		if got := ContentHashFromObjectID("hello"); got != want {
			t.Fatalf("unexpected content hash: got %q want %q", got, want)
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		const want = "0x1c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac8"
		if got := ContentHashFromObjectID("  hello \n"); got != want {
			t.Fatalf("unexpected content hash: got %q want %q", got, want)
		}
	})

	t.Run("empty string returns empty", func(t *testing.T) {
		if got := ContentHashFromObjectID(""); got != "" {
			t.Fatalf("expected empty content hash, got %q", got)
		}
	})
}
