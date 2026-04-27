package reputation

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
)

type userRepository interface {
	GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error)
	StoreReputation(ctx context.Context, actorID string, reputation *storage.Reputation) error
}

type actorRepository interface {
	GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error)
}

type statusRepository interface {
	CountStatusesByAuthor(ctx context.Context, username string) (int, error)
	GetUserTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error)
}

type activityRepository interface {
	GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error)
}

type relationshipRepository interface {
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
}

type trustRepository interface {
	GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error)
	GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error)
}

type moderationRepository interface {
	GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error)
	GetReportsByTarget(ctx context.Context, targetAccountID string, limit int, cursor string) ([]*storage.Report, string, error)
}

type communityNoteRepository interface {
	GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error)
	GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error)
}

type reputationCalculator interface {
	Calculate(ctx context.Context, input *CalculationInput) (*Reputation, error)
}

type reputationSigner interface {
	SignReputation(rep *Reputation) error
	SignPortableReputation(pr *PortableReputation) error
	GetPublicKeyBase64() string
}

type vouchSignatureVerifier interface {
	VerifyVouchSignature(vouch *Vouch) (bool, error)
}

type reputationVerifier interface {
	VerifyPortableReputation(pr *PortableReputation) (*VerificationResult, error)
	VerifyVouchSignature(vouch *Vouch) (bool, error)
}

type reputationVouchManager interface {
	GetVouchesForActor(ctx context.Context, actorID string) ([]Vouch, error)
	GetVouchesFromActor(ctx context.Context, actorID string) ([]Vouch, error)
	ImportVouches(ctx context.Context, vouches []Vouch, verifier vouchSignatureVerifier) (int, error)
	CreateVouch(ctx context.Context, input *CreateVouchInput) (*Vouch, error)
	RevokeVouch(ctx context.Context, vouchID, actorID string) error
}

type reputationCacheDB interface {
	WithContext(ctx context.Context) reputationCacheDB
	Model(model any) reputationCacheQuery
}

type reputationCacheQuery interface {
	Where(field string, op string, value any) reputationCacheQuery
	First(dest any) error
	Create() error
}

type dynamormReputationCacheDB struct {
	db dynamormCore.DB
}

func (d dynamormReputationCacheDB) WithContext(ctx context.Context) reputationCacheDB {
	if d.db == nil {
		return d
	}
	return dynamormReputationCacheDB{db: d.db.WithContext(ctx)}
}

func (d dynamormReputationCacheDB) Model(model any) reputationCacheQuery {
	if d.db == nil {
		return &dynamormReputationCacheQuery{}
	}
	return &dynamormReputationCacheQuery{q: d.db.Model(model)}
}

type dynamormReputationCacheQuery struct {
	q dynamormCore.Query
}

func (q *dynamormReputationCacheQuery) Where(field string, op string, value any) reputationCacheQuery {
	if q.q == nil {
		return q
	}
	q.q = q.q.Where(field, op, value)
	return q
}

func (q *dynamormReputationCacheQuery) First(dest any) error {
	if q.q == nil {
		return fmt.Errorf("cache disabled")
	}
	return q.q.First(dest)
}

func (q *dynamormReputationCacheQuery) Create() error {
	if q.q == nil {
		return fmt.Errorf("cache disabled")
	}
	return q.q.Create()
}

// Service provides reputation management functionality
type Service struct {
	userRepo          userRepository
	actorRepo         actorRepository
	statusRepo        statusRepository
	activityRepo      activityRepository
	relationshipRepo  relationshipRepository
	trustRepo         trustRepository
	moderationRepo    moderationRepository
	communityNoteRepo communityNoteRepository
	cache             reputationCacheDB

	calculator   reputationCalculator
	signer       reputationSigner
	verifier     reputationVerifier
	vouchManager reputationVouchManager

	logger      *zap.Logger
	costTracker *cost.Tracker
	instanceURL string
}

// Config contains configuration for the reputation service
type Config struct {
	Storage     core.RepositoryStorage
	Logger      *zap.Logger
	CostTracker *cost.Tracker
	InstanceURL string
	PrivateKey  string
}

// NewService creates a new reputation service
func NewService(cfg *Config) (*Service, error) {
	// Validate config
	if cfg.Storage == nil {
		return nil, fmt.Errorf("storage is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	// Create signer
	signer, err := NewSigner(cfg.PrivateKey, cfg.InstanceURL, cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	// Create components using storage interface
	calculator := NewCalculator(cfg.Storage, cfg.InstanceURL, cfg.Logger)
	verifier := NewVerifier(cfg.InstanceURL, cfg.Logger, cfg.Storage.DomainBlock())
	vouchManager := NewVouchManager(cfg.Storage, signer, cfg.InstanceURL, cfg.Logger)

	return &Service{
		userRepo:          cfg.Storage.User(),
		actorRepo:         cfg.Storage.Actor(),
		statusRepo:        cfg.Storage.Status(),
		activityRepo:      cfg.Storage.Activity(),
		relationshipRepo:  cfg.Storage.Relationship(),
		trustRepo:         cfg.Storage.Trust(),
		moderationRepo:    cfg.Storage.Moderation(),
		communityNoteRepo: cfg.Storage.CommunityNote(),
		cache:             dynamormReputationCacheDB{db: cfg.Storage.GetDB()},

		calculator:   calculator,
		signer:       signer,
		verifier:     verifier,
		vouchManager: vouchManager,
		logger:       cfg.Logger,
		costTracker:  cfg.CostTracker,
		instanceURL:  cfg.InstanceURL,
	}, nil
}

// GetReputation retrieves the current reputation for an actor
func (s *Service) GetReputation(ctx context.Context, actorID string) (*Reputation, error) {
	if err := ValidateActorID(actorID); err != nil {
		return nil, fmt.Errorf("invalid actor ID: %w", err)
	}

	// Get reputation from storage
	storedRep, err := s.userRepo.GetReputation(ctx, actorID)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) {
			// No reputation history, calculate new.
			return s.calculateAndStore(ctx, actorID)
		}
		return nil, fmt.Errorf("failed to get reputation: %w", err)
	}

	if storedRep == nil {
		// No reputation history, calculate new
		return s.calculateAndStore(ctx, actorID)
	}

	// Convert storage.Reputation to reputation.Reputation
	rep := &Reputation{
		ActorID:           storedRep.ActorID,
		InstanceURL:       storedRep.InstanceURL,
		TrustScore:        int(storedRep.TrustScore),
		ActivityScore:     int(storedRep.ActivityScore),
		ModerationScore:   int(storedRep.ModerationScore),
		CommunityScore:    int(storedRep.CommunityScore),
		TotalScore:        int(storedRep.TotalScore),
		CalculatedAt:      storedRep.CalculatedAt,
		Version:           fmt.Sprintf("%d", storedRep.Version),
		TotalPosts:        storedRep.TotalPosts,
		TotalFollowers:    storedRep.TotalFollowers,
		AccountAge:        storedRep.AccountAge,
		VouchCount:        storedRep.VouchCount,
		TrustingActors:    len(storedRep.TrustingActors),
		AverageTrustScore: storedRep.AverageTrustScore,
		ReportsReceived:   storedRep.ReportsReceived,
		ReportsUpheld:     storedRep.ReportsUpheld,
		FalseReports:      storedRep.FalseReports,
		Signature:         storedRep.Signature,
		PublicKey:         storedRep.PublicKey,
	}

	// Check if reputation is stale (older than 24 hours)
	if time.Since(rep.CalculatedAt) > 24*time.Hour {
		// Recalculate
		return s.calculateAndStore(ctx, actorID)
	}

	return rep, nil
}

// calculateAndStore calculates and stores reputation for an actor
func (s *Service) calculateAndStore(ctx context.Context, actorID string) (*Reputation, error) {
	// Gather calculation input
	input, err := s.gatherCalculationInput(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("failed to gather calculation input: %w", err)
	}

	// Calculate reputation
	rep, err := s.calculator.Calculate(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate reputation: %w", err)
	}

	// Sign reputation
	if err := s.signer.SignReputation(rep); err != nil {
		return nil, fmt.Errorf("failed to sign reputation: %w", err)
	}

	// Store reputation
	if err := s.storeReputation(ctx, rep); err != nil {
		return nil, fmt.Errorf("failed to store reputation: %w", err)
	}

	return rep, nil
}

// gatherCalculationInput gathers all data needed to calculate reputation
func (s *Service) gatherCalculationInput(ctx context.Context, actorID string) (*CalculationInput, error) {
	input := &CalculationInput{
		ActorID: actorID,
	}

	// Extract username and get actor
	username := s.extractUsername(actorID)
	actor, err := s.getActorData(ctx, actorID, username)
	if err != nil {
		return nil, err
	}

	// Gather basic actor information
	s.gatherActorBasics(input, actor)

	// Gather activity metrics
	s.gatherActivityMetrics(ctx, input, actorID)

	// Gather trust relationships
	input.TrustRelationships = s.gatherTrustRelationships(ctx, actorID)

	// Gather moderation history
	input.ModerationHistory = s.gatherModerationHistory(ctx, actorID)

	// Gather vouches
	s.gatherVouches(ctx, input, actorID)

	// Gather community contributions
	s.gatherCommunityContributions(ctx, input, actorID)

	return input, nil
}

// extractUsername extracts the username from an actor ID
func (s *Service) extractUsername(actorID string) string {
	username := actorID

	// Try to extract from /users/ format
	if idx := strings.LastIndex(actorID, "/users/"); idx != -1 {
		username = actorID[idx+7:] // 7 is len("/users/")
	} else if idx := strings.LastIndex(actorID, "/@"); idx != -1 {
		username = actorID[idx+2:] // 2 is len("/@")
	} else if strings.HasPrefix(actorID, "@") {
		username = actorID[1:] // Remove leading @
	}

	// Remove any trailing slashes or query parameters
	if idx := strings.IndexAny(username, "/?#"); idx != -1 {
		username = username[:idx]
	}

	s.logger.Debug("Extracting username from actor ID",
		zap.String("actorID", actorID),
		zap.String("extracted_username", username))

	return username
}

// getActorData retrieves actor data from storage
func (s *Service) getActorData(ctx context.Context, actorID, username string) (*activitypub.Actor, error) {
	actor, err := s.actorRepo.GetActorByUsername(ctx, username)
	if err != nil {
		s.logger.Error("Failed to get actor",
			zap.String("actorID", actorID),
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}
	return actor, nil
}

// gatherActorBasics gathers basic actor information
func (s *Service) gatherActorBasics(input *CalculationInput, actor *activitypub.Actor) {
	if actor.Published != nil {
		input.AccountCreated = *actor.Published
	} else {
		input.AccountCreated = time.Now().AddDate(-1, 0, 0) // Default to 1 year
	}
}

// gatherActivityMetrics gathers activity-related metrics
func (s *Service) gatherActivityMetrics(ctx context.Context, input *CalculationInput, actorID string) {
	// Get post count from status repository
	input.PostCount = s.getPostCount(ctx, actorID)

	// Get follower count
	input.FollowerCount = s.getFollowerCount(ctx, actorID)

	// Get last activity time from recent posts and interactions
	input.LastActive = s.getLastActivityTime(ctx, actorID)
}

// getPostCount retrieves the total number of posts (statuses) by an actor
func (s *Service) getPostCount(ctx context.Context, actorID string) int {
	username := s.extractUsername(actorID)

	// Try cache first (cache for 10 minutes)
	cacheKey := fmt.Sprintf("post_count_%s", username)
	if cached := s.getCachedMetric(ctx, cacheKey, 10*time.Minute); cached >= 0 {
		return cached
	}

	count, err := s.statusRepo.CountStatusesByAuthor(ctx, username)
	if err != nil {
		s.logger.Warn("Failed to get post count",
			zap.String("actorID", actorID),
			zap.String("username", username),
			zap.Error(err))
		return 0
	}

	// Cache the result
	s.setCachedMetric(ctx, cacheKey, count)

	return count
}

// getLastActivityTime retrieves the timestamp of the most recent activity by an actor
func (s *Service) getLastActivityTime(ctx context.Context, actorID string) time.Time {
	username := s.extractUsername(actorID)

	// Try cache first (cache for 5 minutes - activity timestamps change more frequently)
	cacheKey := fmt.Sprintf("last_activity_%s", username)
	if cached := s.getCachedTimestamp(ctx, cacheKey, 5*time.Minute); !cached.IsZero() {
		return cached
	}

	// Check recent posts first (statuses are the primary activity type)
	lastActivity := s.getLastStatusTime(ctx, username)

	// Check recent outbox activities to catch other activity types like likes, follows, etc.
	lastOutboxActivity := s.getLastOutboxActivityTime(ctx, username)

	// Return the most recent timestamp
	var result time.Time
	if lastOutboxActivity.After(lastActivity) {
		result = lastOutboxActivity
	} else if !lastActivity.IsZero() {
		result = lastActivity
	} else {
		// If no activity found, default to 30 days ago
		result = time.Now().Add(-30 * 24 * time.Hour)
	}

	// Cache the result
	s.setCachedTimestamp(ctx, cacheKey, result)

	return result
}

// getLastStatusTime gets the timestamp of the most recent status by an actor
func (s *Service) getLastStatusTime(ctx context.Context, username string) time.Time {
	// Get the most recent status using GetUserTimeline (limit to 1)
	opts := interfaces.PaginationOptions{Limit: 1}
	result, err := s.statusRepo.GetUserTimeline(ctx, username, opts)
	if err != nil {
		s.logger.Warn("Failed to get recent statuses for last activity",
			zap.String("username", username),
			zap.Error(err))
		return time.Time{}
	}
	statuses := result.Items

	if err := common.ValidateSliceNotEmpty("statuses", statuses); err != nil {
		return time.Time{}
	}

	// Extract timestamp from the most recent status
	if statuses[0].Note != nil && statuses[0].Note.Published != nil {
		return *statuses[0].Note.Published
	}

	return time.Time{}
}

// getLastOutboxActivityTime gets the timestamp of the most recent outbox activity
func (s *Service) getLastOutboxActivityTime(ctx context.Context, username string) time.Time {
	// Get the most recent outbox activity (limit to 1)
	activities, _, err := s.activityRepo.GetOutboxActivities(ctx, username, 1, "")
	if err != nil {
		s.logger.Warn("Failed to get recent outbox activities for last activity",
			zap.String("username", username),
			zap.Error(err))
		return time.Time{}
	}

	if err := common.ValidateSliceNotEmpty("activities", activities); err != nil {
		return time.Time{}
	}

	// Extract timestamp from the most recent activity
	if activities[0].Published != nil {
		return *activities[0].Published
	}

	return time.Time{}
}

// getFollowerCount retrieves the follower count for an actor
func (s *Service) getFollowerCount(ctx context.Context, actorID string) int {
	followers, _, err := s.relationshipRepo.GetFollowers(ctx, actorID, 1000, "")
	if err != nil {
		s.logger.Warn("Failed to get followers", zap.Error(err))
		return 0
	}
	return len(followers)
}

// gatherTrustRelationships gathers trust relationship data
func (s *Service) gatherTrustRelationships(ctx context.Context, actorID string) []TrustRelationship {
	trustRelationships := []TrustRelationship{}

	// Get outgoing trust relationships
	s.addOutgoingTrust(ctx, actorID, &trustRelationships)

	// Get incoming trust relationships
	s.addIncomingTrust(ctx, actorID, &trustRelationships)

	return trustRelationships
}

// addOutgoingTrust adds relationships where this actor trusts others
func (s *Service) addOutgoingTrust(ctx context.Context, actorID string, relationships *[]TrustRelationship) {
	trusting, _, err := s.trustRepo.GetTrustRelationships(ctx, actorID, 100, "")
	if err != nil {
		s.logger.Warn("Failed to get trusting relationships", zap.Error(err))
		return
	}

	for _, rel := range trusting {
		*relationships = append(*relationships, s.convertTrustRelationship(rel))
	}
}

// addIncomingTrust adds relationships where others trust this actor
func (s *Service) addIncomingTrust(ctx context.Context, actorID string, relationships *[]TrustRelationship) {
	trustedBy, _, err := s.trustRepo.GetTrustedByRelationships(ctx, actorID, 100, "")
	if err != nil {
		s.logger.Warn("Failed to get trusted-by relationships", zap.Error(err))
		return
	}

	for _, rel := range trustedBy {
		if !s.isDuplicateTrust(*relationships, rel) {
			*relationships = append(*relationships, s.convertTrustRelationship(rel))
		}
	}
}

// isDuplicateTrust checks if a trust relationship already exists
func (s *Service) isDuplicateTrust(relationships []TrustRelationship, rel *storage.TrustRelationship) bool {
	for _, existing := range relationships {
		if existing.FromActor == rel.TrusterID && existing.ToActor == rel.TrusteeID {
			return true
		}
	}
	return false
}

// convertTrustRelationship converts storage trust to reputation trust
func (s *Service) convertTrustRelationship(rel *storage.TrustRelationship) TrustRelationship {
	return TrustRelationship{
		FromActor:  rel.TrusterID,
		ToActor:    rel.TrusteeID,
		TrustScore: rel.Score,
		Category:   string(rel.Category),
		UpdatedAt:  rel.Updated,
	}
}

// gatherModerationHistory gathers moderation event history
func (s *Service) gatherModerationHistory(ctx context.Context, actorID string) []ModerationEvent {
	moderationHistory := []ModerationEvent{}

	// Add moderation events
	s.addModerationEvents(ctx, actorID, &moderationHistory)

	// Add reports
	s.addReportEvents(ctx, actorID, &moderationHistory)

	return moderationHistory
}

// addModerationEvents adds moderation events to history
func (s *Service) addModerationEvents(ctx context.Context, actorID string, history *[]ModerationEvent) {
	events, _, err := s.moderationRepo.GetModerationEventsByActor(ctx, actorID, 100, "")
	if err != nil {
		s.logger.Warn("Failed to get moderation events", zap.Error(err))
		return
	}

	for _, event := range events {
		*history = append(*history, s.convertModerationEvent(event))
	}
}

// convertModerationEvent converts storage moderation event to reputation moderation event
func (s *Service) convertModerationEvent(event *storage.ModerationEvent) ModerationEvent {
	modEvent := ModerationEvent{
		ID:         event.ID,
		Type:       event.EventType,
		Outcome:    s.deriveOutcomeFromEvent(event),
		OccurredAt: event.Created,
	}

	// Parse severity
	modEvent.Severity = s.parseSeverity(event.Severity)

	return modEvent
}

// parseSeverity parses severity string to int
func (s *Service) parseSeverity(severity string) int {
	switch severity {
	case "1":
		return 1
	case "2":
		return 2
	case "3":
		return 3
	case "4":
		return 4
	default:
		return 2 // Default to medium
	}
}

// addReportEvents adds report events to moderation history
func (s *Service) addReportEvents(ctx context.Context, actorID string, history *[]ModerationEvent) {
	reports, _, err := s.moderationRepo.GetReportsByTarget(ctx, actorID, 100, "")
	if err != nil {
		s.logger.Warn("Failed to get reports", zap.Error(err))
		return
	}

	for _, report := range reports {
		*history = append(*history, ModerationEvent{
			ID:         report.ID,
			Type:       "report",
			Outcome:    report.Status,
			OccurredAt: report.CreatedAt,
			Severity:   2, // Reports are medium severity by default
		})
	}
}

// gatherVouches gathers vouch information
func (s *Service) gatherVouches(ctx context.Context, input *CalculationInput, actorID string) {
	// Get vouches received
	vouchesReceived, err := s.vouchManager.GetVouchesForActor(ctx, actorID)
	if err != nil {
		s.logger.Warn("Failed to get vouches", zap.Error(err))
		vouchesReceived = []Vouch{}
	}
	input.VouchesReceived = vouchesReceived

	// Get vouches given
	vouchesGiven, err := s.vouchManager.GetVouchesFromActor(ctx, actorID)
	if err != nil {
		s.logger.Warn("Failed to get vouches given", zap.Error(err))
		vouchesGiven = []Vouch{}
	}
	input.VouchesGiven = vouchesGiven
}

// gatherCommunityContributions gathers community contribution data
func (s *Service) gatherCommunityContributions(ctx context.Context, input *CalculationInput, actorID string) {
	// Get community notes count
	communityNotes := s.getCommunityNotes(ctx, actorID)
	input.CommunityNotes = len(communityNotes)

	// Count helpful votes
	input.HelpfulVotes = s.countHelpfulVotes(ctx, communityNotes)
}

// getCommunityNotes retrieves community notes for an actor
func (s *Service) getCommunityNotes(ctx context.Context, actorID string) []*storage.CommunityNote {
	communityNotes, _, err := s.communityNoteRepo.GetCommunityNotesByAuthor(ctx, actorID, 1000, "")
	if err != nil {
		s.logger.Warn("Failed to get community notes", zap.Error(err))
		return []*storage.CommunityNote{}
	}
	return communityNotes
}

// countHelpfulVotes counts helpful votes on community notes
func (s *Service) countHelpfulVotes(ctx context.Context, notes []*storage.CommunityNote) int {
	helpfulVotes := 0
	for _, note := range notes {
		helpfulVotes += s.countNoteHelpfulVotes(ctx, note.ID)
	}
	return helpfulVotes
}

// countNoteHelpfulVotes counts helpful votes for a single note
func (s *Service) countNoteHelpfulVotes(ctx context.Context, noteID string) int {
	votes, err := s.communityNoteRepo.GetCommunityNoteVotes(ctx, noteID)
	if err != nil {
		s.logger.Warn("Failed to get votes for note",
			zap.String("note_id", noteID),
			zap.Error(err))
		return 0
	}

	count := 0
	for _, vote := range votes {
		if vote.Helpful {
			count++
		}
	}
	return count
}

// storeReputation stores reputation using the storage layer
func (s *Service) storeReputation(ctx context.Context, rep *Reputation) error {
	// Convert reputation.Reputation to storage.Reputation
	storedRep := &storage.Reputation{
		ActorID:           rep.ActorID,
		InstanceURL:       rep.InstanceURL,
		TrustScore:        float64(rep.TrustScore),
		ActivityScore:     float64(rep.ActivityScore),
		ModerationScore:   float64(rep.ModerationScore),
		CommunityScore:    float64(rep.CommunityScore),
		TotalScore:        float64(rep.TotalScore),
		CalculatedAt:      rep.CalculatedAt,
		Version:           func() int { v, _ := strconv.Atoi(rep.Version); return v }(),
		TotalPosts:        rep.TotalPosts,
		TotalFollowers:    rep.TotalFollowers,
		AccountAge:        rep.AccountAge,
		VouchCount:        rep.VouchCount,
		TrustingActors:    []string{}, // We don't store individual actors in this conversion
		AverageTrustScore: rep.AverageTrustScore,
		ReportsReceived:   rep.ReportsReceived,
		ReportsUpheld:     rep.ReportsUpheld,
		FalseReports:      rep.FalseReports,
		Signature:         rep.Signature,
		PublicKey:         rep.PublicKey,
	}

	return s.userRepo.StoreReputation(ctx, rep.ActorID, storedRep)
}

// ExportReputation exports a portable reputation document
func (s *Service) ExportReputation(ctx context.Context, actorID string) (*PortableReputation, error) {
	// Get current reputation
	rep, err := s.GetReputation(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get reputation: %w", err)
	}

	// Get active vouches
	vouches, err := s.vouchManager.GetVouchesForActor(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get vouches: %w", err)
	}

	// Create portable document
	pr := &PortableReputation{
		Context: []string{
			"https://www.w3.org/ns/activitystreams",
			"https://lesser.social/ns/reputation/v1",
		},
		Type:       "PortableReputation",
		Actor:      actorID,
		Reputation: rep,
		Vouches:    vouches,
	}

	// Sign the document
	if err := s.signer.SignPortableReputation(pr); err != nil {
		return nil, fmt.Errorf("failed to sign portable reputation: %w", err)
	}

	s.logger.Info("Exported reputation",
		zap.String("actor", actorID),
		zap.Int("score", rep.TotalScore),
		zap.Int("vouches", len(vouches)))

	return pr, nil
}

// ImportReputation imports a portable reputation document
func (s *Service) ImportReputation(ctx context.Context, document string) (*ImportResult, error) {
	// Parse document
	var pr PortableReputation
	if err := json.Unmarshal([]byte(document), &pr); err != nil {
		return &ImportResult{
			Success: false,
			Error:   "Invalid JSON document",
		}, nil
	}

	// Verify the document
	verification, err := s.verifier.VerifyPortableReputation(&pr)
	if err != nil {
		return &ImportResult{
			Success: false,
			Error:   fmt.Sprintf("Verification failed: %v", err),
		}, nil
	}

	if !verification.Valid {
		return &ImportResult{
			Success: false,
			Error:   verification.Error,
		}, nil
	}

	// Get current reputation
	currentRep, _ := s.GetReputation(ctx, pr.Actor)
	previousScore := 0
	if currentRep != nil {
		previousScore = currentRep.TotalScore
	}

	// Import reputation data
	if pr.Reputation != nil {
		// Store imported reputation with special marker
		pr.Reputation.InstanceURL = pr.Issuer // Mark as imported
		if err := s.storeReputation(ctx, pr.Reputation); err != nil {
			return &ImportResult{
				Success: false,
				Error:   "Failed to store reputation",
			}, nil
		}
	}

	// Import vouches
	vouchesImported, err := s.vouchManager.ImportVouches(ctx, pr.Vouches, s.verifier)
	if err != nil {
		s.logger.Warn("Some vouches failed to import", zap.Error(err))
	}

	return &ImportResult{
		Success:         true,
		ActorID:         pr.Actor,
		PreviousScore:   previousScore,
		ImportedScore:   pr.Reputation.TotalScore,
		VouchesImported: vouchesImported,
		Message:         "Reputation imported successfully",
	}, nil
}

// CreateVouch creates a new vouch
func (s *Service) CreateVouch(ctx context.Context, fromActorID, toActorID string, confidence float64, context string) (*Vouch, error) {
	return s.vouchManager.CreateVouch(ctx, &CreateVouchInput{
		FromActorID: fromActorID,
		ToActorID:   toActorID,
		Confidence:  confidence,
		Context:     context,
	})
}

// RevokeVouch revokes an existing vouch
func (s *Service) RevokeVouch(ctx context.Context, vouchID, actorID string) error {
	return s.vouchManager.RevokeVouch(ctx, vouchID, actorID)
}

// GetVouches retrieves vouches for an actor
func (s *Service) GetVouches(ctx context.Context, actorID string) ([]Vouch, error) {
	return s.vouchManager.GetVouchesForActor(ctx, actorID)
}

// VerifyReputation verifies a reputation document
func (s *Service) VerifyReputation(_ context.Context, document string) (*VerificationResult, error) {
	var pr PortableReputation
	if err := json.Unmarshal([]byte(document), &pr); err != nil {
		return &VerificationResult{
			Valid: false,
			Error: "Invalid JSON document",
		}, nil
	}

	return s.verifier.VerifyPortableReputation(&pr)
}

// GetPublicKey returns the instance's public key for reputation signing
func (s *Service) GetPublicKey() string {
	return s.signer.GetPublicKeyBase64()
}

// Caching helper methods for performance optimization

// getCachedMetric retrieves a cached integer metric if not expired
func (s *Service) getCachedMetric(ctx context.Context, key string, maxAge time.Duration) int {
	// Create a simple cache key pattern
	pk := fmt.Sprintf("REPUTATION_CACHE#%s", key)
	sk := "METRIC"

	type CachedMetric struct {
		PK       string    `dynamodb:"PK"`
		SK       string    `dynamodb:"SK"`
		Value    int       `dynamodb:"Value"`
		CachedAt time.Time `dynamodb:"CachedAt"`
		TTL      int64     `dynamodb:"TTL"`
	}

	var cached CachedMetric
	if s.cache == nil {
		return -1
	}
	err := s.cache.WithContext(ctx).Model(&CachedMetric{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&cached)

	if err != nil {
		return -1 // Not found or error
	}

	// Check if cache is still fresh
	if time.Since(cached.CachedAt) > maxAge {
		return -1 // Cache expired
	}

	return cached.Value
}

// setCachedMetric stores a metric in cache with TTL
func (s *Service) setCachedMetric(ctx context.Context, key string, value int) {
	pk := fmt.Sprintf("REPUTATION_CACHE#%s", key)
	sk := "METRIC"
	now := time.Now()
	ttl := now.Add(24 * time.Hour).Unix() // Cache for 24 hours max

	type CachedMetric struct {
		PK       string    `dynamodb:"PK"`
		SK       string    `dynamodb:"SK"`
		Value    int       `dynamodb:"Value"`
		CachedAt time.Time `dynamodb:"CachedAt"`
		TTL      int64     `dynamodb:"TTL"`
	}

	cached := &CachedMetric{
		PK:       pk,
		SK:       sk,
		Value:    value,
		CachedAt: now,
		TTL:      ttl,
	}

	if s.cache == nil {
		return
	}
	err := s.cache.WithContext(ctx).Model(cached).Create()
	if err != nil {
		s.logger.Debug("Failed to cache metric",
			zap.String("key", key),
			zap.Error(err))
	}
}

// getCachedTimestamp retrieves a cached timestamp if not expired
func (s *Service) getCachedTimestamp(ctx context.Context, key string, maxAge time.Duration) time.Time {
	pk := fmt.Sprintf("REPUTATION_CACHE#%s", key)
	sk := "TIMESTAMP"

	type CachedTimestamp struct {
		PK       string    `dynamodb:"PK"`
		SK       string    `dynamodb:"SK"`
		Value    time.Time `dynamodb:"Value"`
		CachedAt time.Time `dynamodb:"CachedAt"`
		TTL      int64     `dynamodb:"TTL"`
	}

	var cached CachedTimestamp
	if s.cache == nil {
		return time.Time{}
	}
	err := s.cache.WithContext(ctx).Model(&CachedTimestamp{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&cached)

	if err != nil {
		return time.Time{} // Not found or error
	}

	// Check if cache is still fresh
	if time.Since(cached.CachedAt) > maxAge {
		return time.Time{} // Cache expired
	}

	return cached.Value
}

// setCachedTimestamp stores a timestamp in cache with TTL
func (s *Service) setCachedTimestamp(ctx context.Context, key string, value time.Time) {
	pk := fmt.Sprintf("REPUTATION_CACHE#%s", key)
	sk := "TIMESTAMP"
	now := time.Now()
	ttl := now.Add(24 * time.Hour).Unix() // Cache for 24 hours max

	type CachedTimestamp struct {
		PK       string    `dynamodb:"PK"`
		SK       string    `dynamodb:"SK"`
		Value    time.Time `dynamodb:"Value"`
		CachedAt time.Time `dynamodb:"CachedAt"`
		TTL      int64     `dynamodb:"TTL"`
	}

	cached := &CachedTimestamp{
		PK:       pk,
		SK:       sk,
		Value:    value,
		CachedAt: now,
		TTL:      ttl,
	}

	if s.cache == nil {
		return
	}
	err := s.cache.WithContext(ctx).Model(cached).Create()
	if err != nil {
		s.logger.Debug("Failed to cache timestamp",
			zap.String("key", key),
			zap.Error(err))
	}
}

// deriveOutcomeFromEvent derives the outcome of a moderation event from its properties
func (s *Service) deriveOutcomeFromEvent(event *storage.ModerationEvent) string {
	// Check the event data for explicit outcome information
	if event.Data != nil {
		if outcome, exists := event.Data["outcome"]; exists {
			if outcomeStr, ok := outcome.(string); ok && outcomeStr != "" {
				return outcomeStr
			}
		}

		// Check for status field which might indicate outcome
		if status, exists := event.Data["status"]; exists {
			if statusStr, ok := status.(string); ok && statusStr != "" {
				return statusStr
			}
		}
	}

	// Derive outcome from event type and category
	switch strings.ToLower(event.EventType) {
	case "warn", "warning":
		return OutcomeUpheld // Warnings are typically upheld when issued

	case "silence", "suspend", "ban":
		// These are enforcement actions, so they're upheld
		return OutcomeUpheld

	case "report":
		// Reports need to be investigated, default to pending
		// unless we have more specific information
		switch strings.ToLower(event.Category) {
		case "spam", "harassment", "violence":
			// Serious categories likely to be upheld if they resulted in an event
			return OutcomeUpheld
		default:
			return OutcomePending
		}

	case "appeal":
		// Appeals could go either way, check severity or confidence
		if event.ConfidenceScore > 0.8 {
			return OutcomeUpheld
		} else if event.ConfidenceScore < 0.3 {
			return OutcomeDismissed
		}
		return OutcomePending

	case "review", "investigation":
		// Reviews are typically pending until resolved
		return OutcomePending

	case "dismiss", "rejected", "false_positive":
		return OutcomeDismissed

	case "confirmed", "validated", "enforced":
		return OutcomeUpheld

	default:
		// For unknown event types, check severity to make a reasonable guess
		switch strings.ToLower(event.Severity) {
		case "low", "minor":
			return OutcomeDismissed
		case "high", "critical", "severe":
			return OutcomeUpheld
		default:
			return OutcomePending // Conservative default
		}
	}
}
