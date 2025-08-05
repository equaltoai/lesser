package reputation

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
)

// Service provides reputation management functionality
type Service struct {
	storage        core.RepositoryStorage
	calculator     *Calculator
	signer         *Signer
	verifier       *Verifier
	vouchManager   *VouchManager
	logger         *zap.Logger
	costTracker    *cost.Tracker
	instanceURL    string
}

// Config contains configuration for the reputation service
type Config struct {
	Storage        core.RepositoryStorage
	Logger         *zap.Logger
	CostTracker    *cost.Tracker
	InstanceURL    string
	PrivateKey     string
}

// NewService creates a new reputation service
func NewService(cfg *Config) (*Service, error) {
	// Validate config
	if cfg.Storage == nil {
		return nil, fmt.Errorf("storage is required")
	}

	// Create signer
	signer, err := NewSigner(cfg.PrivateKey, cfg.InstanceURL, cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	// Create components using storage interface
	calculator := NewCalculator(cfg.Storage, cfg.InstanceURL, cfg.Logger)
	verifier := NewVerifier(cfg.InstanceURL, cfg.Logger, cfg.Storage)
	vouchManager := NewVouchManager(cfg.Storage, signer, cfg.InstanceURL, cfg.Logger)

	return &Service{
		storage:        cfg.Storage,
		calculator:     calculator,
		signer:         signer,
		verifier:       verifier,
		vouchManager:   vouchManager,
		logger:         cfg.Logger,
		costTracker:    cfg.CostTracker,
		instanceURL:    cfg.InstanceURL,
	}, nil
}

// GetReputation retrieves the current reputation for an actor
func (s *Service) GetReputation(ctx context.Context, actorID string) (*Reputation, error) {
	// Get reputation from storage
	storedRep, err := s.storage.User().GetReputation(ctx, actorID)
	if err != nil {
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

	// Extract username from actor ID for storage calls
	// Actor ID format: https://domain/users/username or https://domain/@username
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

	// Log the extraction for debugging
	s.logger.Debug("Extracting username from actor ID",
		zap.String("actorID", actorID),
		zap.String("extracted_username", username))

	// Get actor data
	actor, err := s.storage.Actor().GetActorByUsername(ctx, username)
	if err != nil {
		s.logger.Error("Failed to get actor",
			zap.String("actorID", actorID),
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	// Get account age
	if actor.Published != nil {
		input.AccountCreated = *actor.Published
	} else {
		input.AccountCreated = time.Now().AddDate(-1, 0, 0) // Default to 1 year
	}

	// Get activity metrics
	// Get post count - GetStatusCount doesn't exist, so we'll use a placeholder
	// TODO: Implement proper status counting
	postCount := 0
	input.PostCount = postCount

	// Get follower count - need to get followers list and count
	followers, _, err := s.storage.Relationship().GetFollowers(ctx, actorID, 1000, "")
	if err != nil {
		s.logger.Warn("Failed to get followers", zap.Error(err))
		input.FollowerCount = 0
	} else {
		input.FollowerCount = len(followers)
	}

	// Get last activity time - GetLatestStatus doesn't exist
	// TODO: Implement proper latest status retrieval
	input.LastActive = time.Now().Add(-30 * 24 * time.Hour) // Default to 30 days ago

	// Get trust relationships
	trustRelationships := []TrustRelationship{}

	// Get relationships where this actor trusts others
	trusting, _, err := s.storage.Trust().GetTrustRelationships(ctx, actorID, 100, "")
	if err != nil {
		s.logger.Warn("Failed to get trusting relationships", zap.Error(err))
	} else {
		for _, rel := range trusting {
			trustRelationships = append(trustRelationships, TrustRelationship{
				FromActor:  rel.TrusterID,
				ToActor:    rel.TrusteeID,
				TrustScore: rel.Score,
				Category:   string(rel.Category),
				UpdatedAt:  rel.Updated,
			})
		}
	}

	// Get relationships where others trust this actor
	trustedBy, _, err := s.storage.Trust().GetTrustedByRelationships(ctx, actorID, 100, "")
	if err != nil {
		s.logger.Warn("Failed to get trusted-by relationships", zap.Error(err))
	} else {
		for _, rel := range trustedBy {
			// Avoid duplicates if already added
			found := false
			for _, existing := range trustRelationships {
				if existing.FromActor == rel.TrusterID && existing.ToActor == rel.TrusteeID {
					found = true
					break
				}
			}
			if !found {
				trustRelationships = append(trustRelationships, TrustRelationship{
					FromActor:  rel.TrusterID,
					ToActor:    rel.TrusteeID,
					TrustScore: rel.Score,
					Category:   string(rel.Category),
				})
			}
		}
	}

	input.TrustRelationships = trustRelationships

	// Get moderation history
	moderationHistory := []ModerationEvent{}

	// Get moderation events where this actor is the subject
	events, _, err := s.storage.Moderation().GetModerationEventsByActor(ctx, actorID, 100, "")
	if err != nil {
		s.logger.Warn("Failed to get moderation events", zap.Error(err))
	} else {
		for _, event := range events {
			// Convert storage moderation event to reputation moderation event
			modEvent := ModerationEvent{
				ID:         event.ID,
				Type:       string(event.EventType),
				Outcome:    string(event.EventType), // Use event type as outcome for now
				OccurredAt: event.Created,
			}

			// Add severity based on event severity
			// Parse severity string to int
			switch event.Severity {
			case "1":
				modEvent.Severity = 1
			case "2":
				modEvent.Severity = 2
			case "3":
				modEvent.Severity = 3
			case "4":
				modEvent.Severity = 4
			default:
				modEvent.Severity = 2 // Default to medium
			}

			moderationHistory = append(moderationHistory, modEvent)
		}
	}

	// Get reports filed against this actor
	// GetReportsByTarget already exists in storage interface
	reports, _, err := s.storage.Moderation().GetReportsByTarget(ctx, actorID, 100, "")
	if err != nil {
		s.logger.Warn("Failed to get reports", zap.Error(err))
	} else {
		for _, report := range reports {
			// Convert report to moderation event
			modEvent := ModerationEvent{
				ID:         report.ID,
				Type:       "report",
				Outcome:    string(report.Status),
				OccurredAt: report.CreatedAt,
				Severity:   2, // Reports are medium severity by default
			}
			moderationHistory = append(moderationHistory, modEvent)
		}
	}

	input.ModerationHistory = moderationHistory

	// Get vouches
	vouchesReceived, err := s.vouchManager.GetVouchesForActor(ctx, actorID)
	if err != nil {
		s.logger.Warn("Failed to get vouches", zap.Error(err))
		vouchesReceived = []Vouch{}
	}
	input.VouchesReceived = vouchesReceived

	vouchesGiven, err := s.vouchManager.GetVouchesFromActor(ctx, actorID)
	if err != nil {
		s.logger.Warn("Failed to get vouches given", zap.Error(err))
		vouchesGiven = []Vouch{}
	}
	input.VouchesGiven = vouchesGiven

	// Community contributions
	// Query community notes authored by this actor
	communityNotes, _, err := s.storage.CommunityNote().GetCommunityNotesByAuthor(ctx, actorID, 1000, "")
	if err != nil {
		s.logger.Warn("Failed to get community notes", zap.Error(err))
		input.CommunityNotes = 0
	} else {
		input.CommunityNotes = len(communityNotes)
	}

	// Query helpful votes on this actor's community notes
	helpfulVotes := 0
	for _, note := range communityNotes {
		votes, err := s.storage.CommunityNote().GetCommunityNoteVotes(ctx, note.ID)
		if err != nil {
			s.logger.Warn("Failed to get votes for note",
				zap.String("note_id", note.ID),
				zap.Error(err))
			continue
		}

		// Count helpful votes
		for _, vote := range votes {
			if vote.Helpful {
				helpfulVotes++
			}
		}
	}
	input.HelpfulVotes = helpfulVotes

	return input, nil
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

	return s.storage.User().StoreReputation(ctx, rep.ActorID, storedRep)
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
func (s *Service) VerifyReputation(ctx context.Context, document string) (*VerificationResult, error) {
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
