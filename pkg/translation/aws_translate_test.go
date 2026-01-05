package translation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestService_generateCacheKey(t *testing.T) {
	s := &Service{}
	got := s.generateCacheKey("hello", "en", "es")

	sum := sha256.Sum256([]byte("hello"))
	expectedHash := hex.EncodeToString(sum[:])
	require.Equal(t, fmt.Sprintf("translation:%s:en:es", expectedHash), got)
}

func TestStripHTMLTags(t *testing.T) {
	got := stripHTMLTags("<p>Hello<br/>world&nbsp;&amp; all</p>")
	require.Equal(t, "Hello world & all", got)
}
