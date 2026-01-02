package common

import (
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestIDGenerator_MethodsAndGlobalFallbacks(t *testing.T) {
	t.Run("generator methods add expected prefixes", func(t *testing.T) {
		g := NewIDGenerator(zap.NewNop())

		assert.Equal(t, "https://example.com/users/alice", g.GenerateActorID("example.com", "alice"))
		assert.Equal(t, "http://example.com/users/alice", g.GenerateActorID("http://example.com", "alice"))

		objID := g.GenerateObjectID("example.com")
		assert.Contains(t, objID, "https://example.com/objects/")
		actID := g.GenerateActivityID("example.com")
		assert.Contains(t, actID, "https://example.com/activities/")

		reportID := g.GenerateReportID()
		assert.Contains(t, reportID, "report_")
		assert.True(t, g.ValidateULID(reportID[len("report_"):]))

		auditID := g.GenerateAuditLogID()
		assert.Contains(t, auditID, "audit_")
		assert.True(t, g.ValidateULID(auditID[len("audit_"):]))

		assert.Contains(t, g.GenerateExportID(), "export_")
		assert.Contains(t, g.GenerateImportID(), "import_")
		assert.Contains(t, g.GenerateConversationID(), "conv_")
		assert.Contains(t, g.GenerateMessageID(), "msg_")
		assert.Contains(t, g.GenerateStreamingSessionID(), "stream_")
		assert.Contains(t, g.GenerateVouchID(), "vouch_")
		assert.Contains(t, g.GenerateCommunityNoteID(), "cn_")
	})

	t.Run("global generator fallbacks cover nil branches", func(t *testing.T) {
		previous := globalIDGenerator
		t.Cleanup(func() { globalIDGenerator = previous })

		globalIDGenerator = nil

		raw := GenerateID()
		_, err := ulid.Parse(raw)
		assert.NoError(t, err)

		assert.Contains(t, GenerateStatusID(), "status_")
		assert.Contains(t, GenerateNoteIDULID(), "note_")
		assert.Contains(t, GenerateMediaIDULID(), "media_")
		assert.Contains(t, GenerateSessionIDULID(), "session_")
		assert.Contains(t, GenerateOperationIDULID(), "op_")
		assert.Contains(t, GenerateJobIDULID(), "job_")
		assert.Contains(t, GenerateRequestIDULID(), "req_")
		assert.Contains(t, GenerateActivityPubIDULID("example.com", "objects"), "https://example.com/objects/")
		assert.Contains(t, GenerateActivityPubIDULID("https://example.com", "objects"), "https://example.com/objects/")
	})
}
