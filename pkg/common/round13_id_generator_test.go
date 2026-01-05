package common

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"strings"
)

func TestGenerateNumericID_IsStableAndNumeric(t *testing.T) {
	id1 := GenerateNumericID("alice")
	id2 := GenerateNumericID("alice")
	assert.Equal(t, id1, id2)
	assert.GreaterOrEqual(t, len(id1), 10)

	// Different input should (very likely) differ.
	id3 := GenerateNumericID("bob")
	assert.NotEqual(t, id1, id3)
}

func TestGenerateNumericIDFromActorID_ExtractsUsernamePatterns(t *testing.T) {
	assert.Equal(t, GenerateNumericID("alice"), GenerateNumericIDFromActorID("https://example.com/users/alice"))
	assert.Equal(t, GenerateNumericID("bob"), GenerateNumericIDFromActorID("https://example.com/@bob?x=y"))
}

func TestIDGenerator_ULIDAndHelpers(t *testing.T) {
	g := NewIDGenerator(zap.NewNop())

	id := g.GenerateULID()
	assert.True(t, g.ValidateULID(id))
	assert.Equal(t, 26, len(id))

	short := g.ShortID()
	assert.Equal(t, 13, len(short))
	assert.Equal(t, short, strings.ToLower(short))

	apID := g.GenerateActivityPubID("example.com", "objects")
	assert.Contains(t, apID, "https://example.com/objects/")

	statusID := g.GenerateStatusID()
	assert.Contains(t, statusID, "status_")
	assert.True(t, g.ValidateULID(statusID[len("status_"):]))
}

func TestIDGenerator_ExtractTimestamp_AndIsOlderThan(t *testing.T) {
	g := NewIDGenerator(zap.NewNop())

	old := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Millisecond)
	oldULID := ulid.MustNew(ulid.Timestamp(old), ulid.Monotonic(rand.Reader, 0))

	ts, err := g.ExtractTimestamp(oldULID.String())
	assert.NoError(t, err)
	assert.Equal(t, old, ts.UTC())

	assert.True(t, g.IsOlderThan(oldULID.String(), time.Hour))
	assert.False(t, g.IsOlderThan(oldULID.String(), 3*time.Hour))

	// Invalid ULID should return false (and log a warning).
	assert.False(t, g.IsOlderThan("not-a-ulid", time.Second))
}

func TestGlobalIDGenerator_FallbackAndInitialization(t *testing.T) {
	previous := globalIDGenerator
	t.Cleanup(func() { globalIDGenerator = previous })

	globalIDGenerator = nil
	raw := GenerateID()
	assert.NotEmpty(t, raw)
	_, err := ulid.Parse(raw)
	assert.NoError(t, err)

	InitGlobalIDGenerator(zap.NewNop())
	assert.NotNil(t, globalIDGenerator)

	assert.Contains(t, GenerateStatusID(), "status_")
	assert.Contains(t, GenerateNoteIDULID(), "note_")
	assert.Contains(t, GenerateMediaIDULID(), "media_")
	assert.Contains(t, GenerateSessionIDULID(), "session_")
	assert.Contains(t, GenerateOperationIDULID(), "op_")
	assert.Contains(t, GenerateJobIDULID(), "job_")
	assert.Contains(t, GenerateRequestIDULID(), "req_")
	assert.Contains(t, GenerateActivityPubIDULID("example.com", "objects"), "https://example.com/objects/")
}
