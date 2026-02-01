package models

import (
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFederationSeveranceModels(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()

	sev := &FederationSeverance{UserID: "alice", Domain: "example.com"}
	sev.UpdateKeys()
	assert.Equal(t, "USER#alice", sev.PK)
	assert.Equal(t, "SEVERANCE#example.com", sev.SK)
	assert.Equal(t, "SEVERANCE#example.com", sev.GSI1PK)
	assert.Equal(t, "USER#alice", sev.GSI1SK)
	assert.Equal(t, MainTableName, sev.TableName())

	issue := &FederationIssue{Domain: "example.com", Timestamp: ts}
	issue.UpdateKeys()
	assert.Equal(t, "FEDERATION_ISSUE#example.com", issue.PK)
	assert.Equal(t, "TIMESTAMP#"+fmt.Sprintf("%d", ts.Unix()), issue.SK)
	assert.Equal(t, ts.Add(90*24*time.Hour).Unix(), issue.TTL)
	assert.Equal(t, MainTableName, issue.TableName())

	attempt := &ReconnectionAttempt{UserID: "alice", Domain: "example.com", AttemptedAt: ts}
	attempt.UpdateKeys()
	assert.Equal(t, "RECONNECTION#alice#example.com", attempt.PK)
	assert.Equal(t, "ATTEMPT#"+fmt.Sprintf("%d", ts.Unix()), attempt.SK)
	assert.Equal(t, MainTableName, attempt.TableName())

	series := &FederationTimeSeries{Domain: "example.com", Period: PeriodHourly, Timestamp: ts}
	series.UpdateKeys()
	assert.Equal(t, "TIMESERIES#example.com#hourly", series.PK)
	assert.Equal(t, ts.Format(time.RFC3339), series.SK)
	assert.Equal(t, "TIMESERIES#hourly", series.GSI1PK)
	assert.Contains(t, series.GSI1SK, "example.com")
	assert.Equal(t, ts.Add(7*24*time.Hour).Unix(), series.TTL)
	assert.Equal(t, MainTableName, series.TableName())
}

func TestThreadContext(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()
	ctx := &ThreadContext{
		RootStatusID: "root",
		StatusID:     "s1",
		UpdatedAt:    ts,
	}
	ctx.UpdateKeys()
	assert.Equal(t, "THREAD#root", ctx.PK)
	assert.Equal(t, "CONTEXT#s1", ctx.SK)
	assert.Equal(t, "STATUS#s1", ctx.GSI1PK)
	assert.Equal(t, "THREAD", ctx.GSI1SK)
	assert.Equal(t, ts.Add(7*24*time.Hour).Unix(), ctx.TTL)
	assert.Equal(t, MainTableName, ctx.TableName())

	ctx.AddParticipant("alice")
	ctx.AddParticipant("alice")
	assert.Equal(t, []string{"alice"}, ctx.Participants)
	assert.True(t, ctx.IsRoot())

	ctx.IncrementReplyCount()
	assert.Equal(t, 1, ctx.ReplyCount)
	assert.Equal(t, 1, ctx.TotalReplies)
	assert.NotNil(t, ctx.LastReplyAt)

	ctx.Path = ""
	assert.Empty(t, ctx.GetPathElements())
	ctx.Path = "root/s1"
	assert.Empty(t, ctx.GetPathElements())
}

func TestCircuitBreakerModels(t *testing.T) {
	t.Run("state keys and backoff duration helpers", func(t *testing.T) {
		changed := time.Unix(1700000000, 0).UTC()
		s := &CircuitBreakerState{InstanceID: "example.com", LastStateChange: changed}
		require.NoError(t, s.UpdateKeys())
		assert.Equal(t, "CIRCUIT#example.com", s.PK)
		assert.Equal(t, SKState, s.SK)
		assert.Equal(t, changed.Add(30*24*time.Hour).Unix(), s.TTL)
		assert.Equal(t, MainTableName, s.TableName())
		assert.Equal(t, s.PK, s.GetPK())
		assert.Equal(t, s.SK, s.GetSK())

		s.SetBackoffDuration(5 * time.Second)
		assert.Equal(t, 5*time.Second, s.GetBackoffDuration())

		// No LastStateChange falls back to "now" TTL.
		s2 := &CircuitBreakerState{InstanceID: "example.com"}
		require.NoError(t, s2.UpdateKeys())
		assert.InDelta(t, time.Now().Add(30*24*time.Hour).Unix(), s2.TTL, 2)
	})

	t.Run("event keys and TTL", func(t *testing.T) {
		e := &CircuitBreakerEvent{InstanceID: "example.com"}
		require.NoError(t, e.UpdateKeys())
		assert.Equal(t, "CIRCUIT#example.com", e.PK)
		assert.Contains(t, e.SK, "EVENT#")
		assert.InDelta(t, time.Now().Add(7*24*time.Hour).Unix(), e.TTL, 2)
		assert.Equal(t, MainTableName, e.TableName())
		assert.Equal(t, e.PK, e.GetPK())
		assert.Equal(t, e.SK, e.GetSK())
	})

	t.Run("config defaults", func(t *testing.T) {
		cfg := DefaultCircuitBreakerConfig()
		assert.Equal(t, 5, cfg.FailureThreshold)
		assert.Equal(t, 3, cfg.SuccessThreshold)
		assert.Equal(t, 30*time.Second, cfg.OpenTimeout)
		assert.Equal(t, MainTableName, (CircuitBreakerConfig{}).TableName())
	})
}

func TestInstanceState(t *testing.T) {
	t.Run("defaults and lifecycle hooks", func(t *testing.T) {
		s := &InstanceState{}
		require.NoError(t, s.BeforeCreate())
		assert.Equal(t, DefaultBootstrapUsername, s.BootstrapUsername)
		assert.False(t, s.CreatedAt.IsZero())
		assert.False(t, s.UpdatedAt.IsZero())
		assert.Equal(t, MainTableName, s.TableName())
		assert.Equal(t, s.PK, s.GetPK())
		assert.Equal(t, s.SK, s.GetSK())

		before := s.UpdatedAt
		require.NoError(t, s.BeforeUpdate())
		assert.True(t, s.UpdatedAt.After(before) || s.UpdatedAt.Equal(before))

		def := NewDefaultInstanceState()
		assert.True(t, def.Locked)
		assert.Equal(t, DefaultBootstrapUsername, def.BootstrapUsername)
	})
}

func TestMove(t *testing.T) {
	t.Run("keys and extract helpers", func(t *testing.T) {
		m := NewMove("id1", "alice", "bob")
		assert.Equal(t, "MOVE#ACTOR#alice", m.PK)
		assert.Equal(t, "TARGET#bob", m.SK)
		assert.Equal(t, "MOVE#TARGET#bob", m.GSI1PK)
		assert.Equal(t, "ACTOR#alice", m.GSI1SK)
		assert.Equal(t, MainTableName, m.TableName())
		assert.Equal(t, "alice", m.ExtractActor())
		assert.Equal(t, "bob", m.ExtractTarget())

		ttl := time.Unix(1700001000, 0).UTC()
		m.SetTTL(ttl)
		require.NotNil(t, m.TTL)
		assert.Equal(t, ttl.Unix(), *m.TTL)

		m2 := &Move{Actor: "alice", Target: "bob"}
		require.NoError(t, m2.BeforeCreate())
		assert.False(t, m2.CreatedAt.IsZero())
	})
}

func TestAnnounce(t *testing.T) {
	a := &Announce{Actor: "alice", Object: "o1"}
	require.NoError(t, a.BeforeCreate())
	assert.NotEmpty(t, a.ID)
	assert.False(t, a.Published.IsZero())
	assert.False(t, a.CreatedAt.IsZero())
	assert.Equal(t, "OBJECT#o1#ANNOUNCES", a.PK)
	assert.Equal(t, "ACTOR#alice", a.SK)
	assert.Equal(t, "ACTOR#alice#ANNOUNCES", a.GSI4PK)
	assert.Contains(t, a.GSI4SK, "OBJECT#o1")
	assert.Equal(t, MainTableName, a.TableName())
	assert.Equal(t, a.PK, a.GetPK())
	assert.Equal(t, a.SK, a.GetSK())

	id := generateRandomID(8)
	assert.Len(t, id, 8)
}

func TestQuoteRelationship(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()
	q := &QuoteRelationship{
		QuoterNoteID: "s1",
		TargetNoteID: "s2",
		QuoterID:     "alice",
		Timestamp:    ts,
	}
	require.NoError(t, q.UpdateKeys())
	assert.Equal(t, "QUOTE#s1", q.PK)
	assert.Equal(t, "QUOTED#s2", q.SK)
	assert.Equal(t, "QUOTED#s2", q.GSI1PK)
	assert.Contains(t, q.GSI1SK, "s1")
	assert.Equal(t, "QUOTER#alice", q.GSI2PK)
	assert.True(t, q.IsActive())

	q.GenerateID()
	assert.Equal(t, "s1:s2", q.ID)

	q.Withdraw()
	assert.True(t, q.Withdrawn)
	assert.False(t, q.IsActive())
	assert.Empty(t, q.GSI1PK)
	assert.NotNil(t, q.WithdrawnAt)
	assert.Equal(t, MainTableName, q.TableName())
	assert.Equal(t, q.PK, q.GetPK())
	assert.Equal(t, q.SK, q.GetSK())
}

func TestObjectModel(t *testing.T) {
	t.Run("UpdateKeys validates ID and updates GSIs", func(t *testing.T) {
		o := &Object{}
		assert.ErrorContains(t, o.UpdateKeys(), "ID is required")

		now := time.Unix(1700000000, 0).UTC()
		o = &Object{ID: "o1", Type: "Note", AttributedTo: "alice", Published: now}
		require.NoError(t, o.UpdateKeys())
		assert.Equal(t, "object#o1", o.PK)
		assert.Equal(t, "object#o1", o.SK)
		assert.Equal(t, "actor#alice", o.GSI1PK)
		assert.Contains(t, o.GSI1SK, "object#")
		assert.Equal(t, "object#type#Note", o.GSI2PK)
		assert.Equal(t, MainTableName, o.TableName())

		parent := "parent"
		o.InReplyTo = &parent
		o.UpdateGSIKeys()
		assert.Equal(t, "REPLIES#parent", o.GSI6PK)
		assert.Contains(t, o.GSI6SK, "o1")
		assert.Equal(t, o.PK, o.GetPK())
		assert.Equal(t, o.SK, o.GetSK())
	})

	t.Run("constructor initializes expected keys", func(t *testing.T) {
		o := NewObject("o1", "Note", "alice")
		assert.Equal(t, "object#o1", o.PK)
		assert.Equal(t, "actor#alice", o.GSI1PK)
		assert.Equal(t, "object#type#Note", o.GSI2PK)
	})
}

func TestSearchResultsAndHistory(t *testing.T) {
	t.Run("SearchResults basic operations", func(t *testing.T) {
		sr := NewSearchResults()
		assert.True(t, sr.IsEmpty())
		assert.Equal(t, 0, sr.Count())

		sr.AddAccount(&activitypub.Actor{})
		sr.AddStatus(NewStatusSearchResult("s1", "c", "u", "a", "alice", time.Now()))
		sr.AddHashtag(NewHashtagSearchResult("golang", "u"))
		assert.False(t, sr.IsEmpty())
		assert.Equal(t, 3, sr.Count())
		assert.Equal(t, MainTableName, sr.TableName())

		other := NewSearchResults()
		other.AddAccount(&activitypub.Actor{})
		sr.Merge(other)
		assert.Equal(t, 4, sr.Count())

		sr.TruncateToLimit(1)
		assert.Len(t, sr.Accounts, 1)
		assert.Len(t, sr.Statuses, 1)
		assert.Len(t, sr.Hashtags, 1)
	})

	t.Run("StatusSearchResult helpers", func(t *testing.T) {
		published := time.Now().Add(-1 * time.Hour)
		r := NewStatusSearchResult("s1", "c", "u", "a", "alice", published)
		r.SetScore(0.5)
		assert.InDelta(t, 0.5, r.Score, 0.0001)
		r.AddHighlight("content", "hit")
		assert.Equal(t, "hit", r.Highlights["content"])
		assert.True(t, r.IsRecent())
		assert.Equal(t, MainTableName, r.TableName())
	})

	t.Run("SearchHistoryEntry keys and computed fields", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		e := &SearchHistoryEntry{UserID: "alice", Query: "golang", SearchedAt: ts, ResultCount: 10}
		e.UpdateKeys()
		assert.Equal(t, "USER#alice", e.PK)
		assert.Contains(t, e.SK, "SEARCH_HISTORY#")
		assert.Equal(t, MainTableName, e.TableName())
		assert.Equal(t, hashQuery("golang"), hashQuery("golang"))

		e.AddClickedID("x")
		e.AddClickedID("x")
		assert.Len(t, e.ClickedIDs, 1)
		assert.InDelta(t, 0.1, e.GetClickRate(), 0.0001)
		e.SearchedAt = time.Now().Add(-1 * time.Hour)
		assert.True(t, e.IsRecent(24*time.Hour))

		e.ResultCount = 0
		assert.Equal(t, 0.0, e.GetClickRate())

		entry := NewSearchHistoryEntry("alice", "golang", 2)
		assert.Equal(t, "USER#alice", entry.PK)
		assert.NotEmpty(t, entry.SK)
	})
}

func TestAIAnalysisModels(t *testing.T) {
	t.Run("AIAnalysis keys and lifecycle hooks", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		a := &AIAnalysis{ID: "a1", ObjectID: "o1", AnalyzedAt: ts}
		require.NoError(t, a.BeforeCreate())
		assert.Equal(t, "AI#o1", a.PK)
		assert.Equal(t, "ANALYSIS#a1", a.SK)
		assert.Equal(t, "AI#ANALYSIS#"+ts.Format("2006-01-02"), a.GSI4PK)
		assert.Equal(t, "AIAnalysis", a.Type)
		assert.False(t, a.CreatedAt.IsZero())
		assert.False(t, a.UpdatedAt.IsZero())
		assert.Equal(t, MainTableName, a.TableName())
		assert.Equal(t, a.PK, a.GetPK())
		assert.Equal(t, a.SK, a.GetSK())

		before := a.UpdatedAt
		require.NoError(t, a.BeforeUpdate())
		assert.True(t, a.UpdatedAt.After(before) || a.UpdatedAt.Equal(before))
	})

	t.Run("AIAnalysisQueue lifecycle hook updates UpdatedAt", func(t *testing.T) {
		q := &AIAnalysisQueue{PK: "OBJECT#o1", SK: "OBJECT#o1"}
		before := q.UpdatedAt
		require.NoError(t, q.BeforeUpdate())
		assert.True(t, q.UpdatedAt.After(before) || q.UpdatedAt.Equal(before))
		assert.Equal(t, MainTableName, q.TableName())
		assert.Equal(t, q.PK, q.GetPK())
		assert.Equal(t, q.SK, q.GetSK())
	})
}
