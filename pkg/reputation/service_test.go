package reputation

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"testing"
	"testing/quick"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	dynamormCore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	"go.uber.org/zap"
)

// serviceTestStorage implements core.RepositoryStorage for testing NewService
type serviceTestStorage struct {
	userRepo          interfaces.UserRepository
	actorRepo         interfaces.ActorRepository
	statusRepo        *repositories.StatusRepository
	activityRepo      interfaces.ActivityRepository
	relationshipRepo  interfaces.ConcreteRelationshipRepository
	trustRepo         interfaces.TrustRepository
	moderationRepo    interfaces.ModerationRepository
	communityNoteRepo *repositories.CommunityNoteRepository
	domainBlockRepo   *repositories.DomainBlockRepository
}

func newServiceTestStorage() *serviceTestStorage {
	return &serviceTestStorage{}
}

func (s *serviceTestStorage) Account() *repositories.AccountRepository                   { return nil }
func (s *serviceTestStorage) Bookmark() *repositories.BookmarkRepository                 { return nil }
func (s *serviceTestStorage) Actor() interfaces.ActorRepository                          { return s.actorRepo }
func (s *serviceTestStorage) Object() interfaces.ObjectRepository                        { return nil }
func (s *serviceTestStorage) Activity() interfaces.ActivityRepository                    { return s.activityRepo }
func (s *serviceTestStorage) Timeline() interfaces.TimelineRepository                    { return nil }
func (s *serviceTestStorage) Notification() interfaces.NotificationRepository            { return nil }
func (s *serviceTestStorage) Like() *repositories.LikeRepository                         { return nil }
func (s *serviceTestStorage) Moderation() interfaces.ModerationRepository                { return s.moderationRepo }
func (s *serviceTestStorage) List() *repositories.ListRepository                         { return nil }
func (s *serviceTestStorage) Media() *repositories.MediaRepository                       { return nil }
func (s *serviceTestStorage) MediaMetadata() *repositories.MediaMetadataRepository       { return nil }
func (s *serviceTestStorage) Poll() *repositories.PollRepository                         { return nil }
func (s *serviceTestStorage) PushSubscription() *repositories.PushSubscriptionRepository { return nil }
func (s *serviceTestStorage) Hashtag() *repositories.HashtagRepository                   { return nil }
func (s *serviceTestStorage) ScheduledStatus() *repositories.ScheduledStatusRepository   { return nil }
func (s *serviceTestStorage) Announcement() *repositories.AnnouncementRepository         { return nil }
func (s *serviceTestStorage) DomainBlock() *repositories.DomainBlockRepository {
	return s.domainBlockRepo
}
func (s *serviceTestStorage) Relationship() interfaces.ConcreteRelationshipRepository {
	return s.relationshipRepo
}
func (s *serviceTestStorage) Instance() *repositories.InstanceRepository           { return nil }
func (s *serviceTestStorage) Federation() *repositories.FederationRepository       { return nil }
func (s *serviceTestStorage) Recovery() *repositories.RecoveryRepository           { return nil }
func (s *serviceTestStorage) Analytics() *repositories.TrendingRepository          { return nil }
func (s *serviceTestStorage) Social() *repositories.SocialRepository               { return nil }
func (s *serviceTestStorage) User() interfaces.UserRepository                      { return s.userRepo }
func (s *serviceTestStorage) Status() interfaces.StatusRepository                  { return s.statusRepo }
func (s *serviceTestStorage) Cost() *repositories.TrackingRepository               { return nil }
func (s *serviceTestStorage) WebSocketCost() *repositories.WebSocketCostRepository { return nil }
func (s *serviceTestStorage) Trust() interfaces.TrustRepository                    { return s.trustRepo }
func (s *serviceTestStorage) Search() *repositories.SearchRepository               { return nil }
func (s *serviceTestStorage) Relay() *repositories.RelayRepository                 { return nil }
func (s *serviceTestStorage) CommunityNote() *repositories.CommunityNoteRepository {
	return s.communityNoteRepo
}
func (s *serviceTestStorage) Emoji() *repositories.EmojiRepository               { return nil }
func (s *serviceTestStorage) RateLimit() *repositories.RateLimitRepository       { return nil }
func (s *serviceTestStorage) Conversation() *repositories.ConversationRepository { return nil }
func (s *serviceTestStorage) Marker() *repositories.MarkerRepository             { return nil }
func (s *serviceTestStorage) FeaturedTag() *repositories.FeaturedTagRepository   { return nil }
func (s *serviceTestStorage) AI() *repositories.AIRepository                     { return nil }
func (s *serviceTestStorage) Export() *repositories.ExportRepository             { return nil }
func (s *serviceTestStorage) Import() *repositories.ImportRepository             { return nil }
func (s *serviceTestStorage) DLQ() *repositories.DLQRepository                   { return nil }
func (s *serviceTestStorage) MetricRecord() *repositories.MetricRecordRepository { return nil }
func (s *serviceTestStorage) CloudWatchMetrics() *repositories.CloudWatchMetricsRepository {
	return nil
}
func (s *serviceTestStorage) StreamingCloudWatch() *repositories.StreamingCloudWatchRepository {
	return nil
}
func (s *serviceTestStorage) Audit() *repositories.AuditRepository                  { return nil }
func (s *serviceTestStorage) OAuth() *repositories.OAuthRepository                  { return nil }
func (s *serviceTestStorage) Skill() interfaces.SkillRepository                     { return nil }
func (s *serviceTestStorage) DNSCache() *repositories.DNSCacheRepository            { return nil }
func (s *serviceTestStorage) Filter() *repositories.FilterRepository                { return nil }
func (s *serviceTestStorage) Thread() *repositories.ThreadRepository                { return nil }
func (s *serviceTestStorage) Severance() *repositories.SeveranceRepository          { return nil }
func (s *serviceTestStorage) ModerationML() *repositories.ModerationMLRepository    { return nil }
func (s *serviceTestStorage) Quote() *repositories.QuoteRepository                  { return nil }
func (s *serviceTestStorage) MediaAnalytics() interfaces.MediaAnalyticsRepository   { return nil }
func (s *serviceTestStorage) MediaPopularity() interfaces.MediaPopularityRepository { return nil }
func (s *serviceTestStorage) MediaSession() interfaces.MediaSessionRepository       { return nil }
func (s *serviceTestStorage) StreamingConnection() interfaces.StreamingConnectionRepository {
	return nil
}
func (s *serviceTestStorage) Article() interfaces.ArticleRepository                     { return nil }
func (s *serviceTestStorage) Draft() interfaces.DraftRepository                         { return nil }
func (s *serviceTestStorage) Revision() interfaces.RevisionRepository                   { return nil }
func (s *serviceTestStorage) Series() interfaces.SeriesRepository                       { return nil }
func (s *serviceTestStorage) Category() interfaces.CategoryRepository                   { return nil }
func (s *serviceTestStorage) Publication() interfaces.PublicationRepository             { return nil }
func (s *serviceTestStorage) PublicationMember() interfaces.PublicationMemberRepository { return nil }
func (s *serviceTestStorage) GetDB() dynamormCore.DB                                    { return nil }
func (s *serviceTestStorage) GetTableName() string                                      { return "test-table" }
func (s *serviceTestStorage) GetLogger() *zap.Logger                                    { return zap.NewNop() }

// Ensure serviceTestStorage implements core.RepositoryStorage
var _ core.RepositoryStorage = (*serviceTestStorage)(nil)

// =============================================================================
// Tests for NewService
// Requirements: 4.1, 4.3, 4.4
// =============================================================================

// TestNewService_ValidConfig tests that NewService creates a service with all components
// Requirements: 4.4 - WHEN NewService is called with valid config THEN the Service SHALL create all components
func TestNewService_ValidConfig(t *testing.T) {
	logger := zap.NewNop()
	instanceURL := "https://test.example.com"

	// Create a mock storage that implements core.RepositoryStorage
	mockStorage := newServiceTestStorage()

	cfg := &Config{
		Storage:     mockStorage,
		Logger:      logger,
		InstanceURL: instanceURL,
		PrivateKey:  "", // Empty key will generate a new one
	}

	service, err := NewService(cfg)
	require.NoError(t, err)
	require.NotNil(t, service)

	// Verify core components are initialized (these are set from storage which may return nil repos)
	// The important thing is that the service itself is created successfully
	require.NotNil(t, service.calculator, "calculator should be initialized")
	require.NotNil(t, service.signer, "signer should be initialized")
	require.NotNil(t, service.verifier, "verifier should be initialized")
	require.NotNil(t, service.vouchManager, "vouchManager should be initialized")
	require.NotNil(t, service.logger, "logger should be initialized")
	require.Equal(t, instanceURL, service.instanceURL)
}

// TestNewService_NilStorage tests that NewService returns error with nil storage
// Requirements: 4.1 - WHEN NewService is called with nil storage THEN the Service SHALL return an error
func TestNewService_NilStorage(t *testing.T) {
	cfg := &Config{
		Storage:     nil,
		Logger:      zap.NewNop(),
		InstanceURL: "https://test.example.com",
	}

	service, err := NewService(cfg)
	require.Error(t, err)
	require.Nil(t, service)
	require.Contains(t, err.Error(), "storage is required")
}

// TestNewService_NilLogger tests that NewService uses no-op logger when nil
// Requirements: 4.2 - WHEN NewService is called with nil logger THEN the Service SHALL use a no-op logger
func TestNewService_NilLogger(t *testing.T) {
	mockStorage := newServiceTestStorage()

	cfg := &Config{
		Storage:     mockStorage,
		Logger:      nil, // Nil logger
		InstanceURL: "https://test.example.com",
		PrivateKey:  "",
	}

	service, err := NewService(cfg)
	require.NoError(t, err)
	require.NotNil(t, service)
	require.NotNil(t, service.logger, "logger should be set to no-op logger")
}

// TestNewService_InvalidPrivateKey tests that NewService returns error with invalid private key
// Requirements: 4.3 - WHEN NewService is called with invalid private key THEN the Service SHALL return an error
func TestNewService_InvalidPrivateKey(t *testing.T) {
	mockStorage := newServiceTestStorage()

	cfg := &Config{
		Storage:     mockStorage,
		Logger:      zap.NewNop(),
		InstanceURL: "https://test.example.com",
		PrivateKey:  "invalid-pem-key-data", // Invalid PEM
	}

	service, err := NewService(cfg)
	require.Error(t, err)
	require.Nil(t, service)
	require.Contains(t, err.Error(), "failed to create signer")
}

// =============================================================================
// Tests for parseSeverity
// Requirements: 5.1
// =============================================================================

// TestParseSeverity tests the parseSeverity function with all values
// Requirements: 5.1 - WHEN parseSeverity is called with severity "3" THEN the Service SHALL return 3
func TestParseSeverity(t *testing.T) {
	svc := &Service{logger: zap.NewNop()}

	testCases := []struct {
		input    string
		expected int
	}{
		{"1", 1},
		{"2", 2},
		{"3", 3},
		{"4", 4},
		{"0", 2},       // Invalid - defaults to 2
		{"5", 2},       // Invalid - defaults to 2
		{"", 2},        // Empty - defaults to 2
		{"invalid", 2}, // Non-numeric - defaults to 2
		{"-1", 2},      // Negative - defaults to 2
	}

	for _, tc := range testCases {
		t.Run("severity_"+tc.input, func(t *testing.T) {
			result := svc.parseSeverity(tc.input)
			require.Equal(t, tc.expected, result, "parseSeverity(%q) should return %d", tc.input, tc.expected)
		})
	}
}

// =============================================================================
// Tests for addModerationEvents error path
// Requirements: 5.2
// =============================================================================

// TestAddModerationEvents_ErrorPath tests that addModerationEvents logs and continues on error
// Requirements: 5.2 - WHEN addModerationEvents encounters an error THEN the Service SHALL log and continue with empty events
func TestAddModerationEvents_ErrorPath(t *testing.T) {
	ctx := context.Background()

	// Create a mock moderation repo that returns an error
	mockModRepo := &round20ModerationRepo{
		eventsErr: errors.New("database error"),
	}

	svc := &Service{
		moderationRepo: mockModRepo,
		logger:         zap.NewNop(),
	}

	var history []ModerationEvent
	svc.addModerationEvents(ctx, "actor1", &history)

	// Should have empty history due to error
	require.Empty(t, history, "history should be empty when error occurs")
}

// TestAddReportEvents_ErrorPath tests that addReportEvents logs and continues on error
// Requirements: 5.3 - WHEN addReportEvents encounters an error THEN the Service SHALL log and continue with empty reports
func TestAddReportEvents_ErrorPath(t *testing.T) {
	ctx := context.Background()

	// Create a mock moderation repo that returns an error for reports
	mockModRepo := &round20ModerationRepo{
		reportsErr: errors.New("database error"),
	}

	svc := &Service{
		moderationRepo: mockModRepo,
		logger:         zap.NewNop(),
	}

	var history []ModerationEvent
	svc.addReportEvents(ctx, "actor1", &history)

	// Should have empty history due to error
	require.Empty(t, history, "history should be empty when error occurs")
}

// =============================================================================
// Tests for ImportReputation store failure
// Requirements: 5.4
// =============================================================================

// TestImportReputation_StoreFailure tests that ImportReputation returns failure when store fails
// Requirements: 5.4 - WHEN ImportReputation fails to store reputation THEN the Service SHALL return failure result
func TestImportReputation_StoreFailure(t *testing.T) {
	ctx := context.Background()

	// Create a user repo that returns existing reputation for GetReputation
	// but fails on store
	userRepo := &round20UserRepo{
		stored: map[string]*storage.Reputation{
			"https://test.example.com/users/alice": {
				ActorID:      "https://test.example.com/users/alice",
				TotalScore:   100,
				CalculatedAt: time.Now(),
				Version:      1,
			},
		},
		storeFn: func(_ context.Context, _ string, _ *storage.Reputation) error {
			return errors.New("storage failure")
		},
	}

	// Create a verifier that returns valid
	verifier := &round20Verifier{
		verifyResult: &VerificationResult{Valid: true},
	}

	// Create a vouch manager
	vouchManager := &round20VouchManager{
		imported: 0,
	}

	svc := &Service{
		userRepo:     userRepo,
		verifier:     verifier,
		vouchManager: vouchManager,
		logger:       zap.NewNop(),
	}

	// Create a valid portable reputation document
	pr := PortableReputation{
		Actor:  "https://test.example.com/users/alice",
		Issuer: "https://issuer.example.com",
		Reputation: &Reputation{
			ActorID:      "https://test.example.com/users/alice",
			InstanceURL:  "https://test.example.com",
			TotalScore:   500,
			CalculatedAt: time.Now(),
			Version:      "1",
		},
	}

	docBytes, err := json.Marshal(pr)
	require.NoError(t, err)

	result, err := svc.ImportReputation(ctx, string(docBytes))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success, "Success should be false when store fails")
	require.Equal(t, "Failed to store reputation", result.Error)
}

// =============================================================================
// Property Test for Moderation Score Monotonicity
// Requirements: 1.7
// =============================================================================

// TestProperty_ModerationScoreMonotonicity verifies that adding an upheld report
// results in a lower or equal ModerationScore
// **Property 6: Moderation Score Monotonicity**
// **Validates: Requirements 1.7**
func TestProperty_ModerationScoreMonotonicity(t *testing.T) {
	calc := newTestCalculator()
	now := time.Now()

	property := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))

		// Generate a random base input
		accountAgeDays := r.Intn(365*5) + 30 // At least 30 days old
		accountCreated := now.AddDate(0, 0, -accountAgeDays)

		// Generate random existing moderation events (0-5)
		numExistingEvents := r.Intn(6)
		existingEvents := make([]ModerationEvent, numExistingEvents)
		for i := 0; i < numExistingEvents; i++ {
			eventType := "report"
			if r.Intn(5) == 0 {
				eventType = "suspension"
			}
			outcome := OutcomePending
			switch r.Intn(3) {
			case 0:
				outcome = OutcomeUpheld
			case 1:
				outcome = OutcomeDismissed
			}
			existingEvents[i] = ModerationEvent{
				Type:     eventType,
				Outcome:  outcome,
				Severity: r.Intn(5) + 1,
			}
		}

		// Calculate score without the additional upheld report
		inputWithout := &CalculationInput{
			AccountCreated:    accountCreated,
			ModerationHistory: existingEvents,
		}
		scoreWithout := calc.calculateModerationScore(inputWithout)

		// Add an upheld report with random severity
		severity := r.Intn(5) + 1
		upheldReport := ModerationEvent{
			Type:     "report",
			Outcome:  OutcomeUpheld,
			Severity: severity,
		}

		eventsWithUpheld := make([]ModerationEvent, len(existingEvents)+1)
		copy(eventsWithUpheld, existingEvents)
		eventsWithUpheld[len(existingEvents)] = upheldReport

		// Calculate score with the additional upheld report
		inputWith := &CalculationInput{
			AccountCreated:    accountCreated,
			ModerationHistory: eventsWithUpheld,
		}
		scoreWith := calc.calculateModerationScore(inputWith)

		// Property: Adding an upheld report should result in lower or equal score
		if scoreWith > scoreWithout {
			t.Logf("Monotonicity violated: scoreWith=%d > scoreWithout=%d (severity=%d, existingEvents=%d)",
				scoreWith, scoreWithout, severity, numExistingEvents)
			return false
		}

		return true
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property failed: %v", err)
	}
}
