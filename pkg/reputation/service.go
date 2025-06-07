package reputation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"

	"github.com/aron23/lesser/pkg/cost"
	"github.com/aron23/lesser/pkg/storage"
)

// Service provides reputation management functionality
type Service struct {
	db             *dynamodb.Client
	storage        storage.Storage
	calculator     *Calculator
	signer         *Signer
	verifier       *Verifier
	vouchManager   *VouchManager
	logger         *zap.Logger
	costTracker    *cost.Tracker
	instanceURL    string
	repTableName   string
	vouchTableName string
}

// Config contains configuration for the reputation service
type Config struct {
	DynamoClient   *dynamodb.Client
	Storage        storage.Storage
	Logger         *zap.Logger
	CostTracker    *cost.Tracker
	InstanceURL    string
	PrivateKey     string
	RepTableName   string
	VouchTableName string
}

// NewService creates a new reputation service
func NewService(cfg *Config) (*Service, error) {
	// Create signer
	signer, err := NewSigner(cfg.PrivateKey, cfg.InstanceURL, cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	// Create components
	calculator := NewCalculator(cfg.DynamoClient, cfg.InstanceURL, cfg.Logger)
	verifier := NewVerifier(cfg.InstanceURL, cfg.Logger, cfg.Storage)
	vouchManager := NewVouchManager(cfg.DynamoClient, cfg.VouchTableName, signer, cfg.Logger)

	return &Service{
		db:             cfg.DynamoClient,
		storage:        cfg.Storage,
		calculator:     calculator,
		signer:         signer,
		verifier:       verifier,
		vouchManager:   vouchManager,
		logger:         cfg.Logger,
		costTracker:    cfg.CostTracker,
		instanceURL:    cfg.InstanceURL,
		repTableName:   cfg.RepTableName,
		vouchTableName: cfg.VouchTableName,
	}, nil
}

// GetReputation retrieves the current reputation for an actor
func (s *Service) GetReputation(ctx context.Context, actorID string) (*Reputation, error) {
	s.costTracker.TrackDynamoRead(1)

	// Query for latest reputation
	result, err := s.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.repTableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", actorID)},
		},
		ScanIndexForward: aws.Bool(false), // Sort descending
		Limit:            aws.Int32(1),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query reputation: %w", err)
	}

	if len(result.Items) == 0 {
		// No reputation history, calculate new
		return s.calculateAndStore(ctx, actorID)
	}

	// Unmarshal reputation
	var storedRep struct {
		ReputationData string `dynamodbav:"ReputationData"`
	}
	if err := attributevalue.UnmarshalMap(result.Items[0], &storedRep); err != nil {
		return nil, fmt.Errorf("failed to unmarshal reputation: %w", err)
	}

	var rep Reputation
	if err := json.Unmarshal([]byte(storedRep.ReputationData), &rep); err != nil {
		return nil, fmt.Errorf("failed to unmarshal reputation data: %w", err)
	}

	// Check if reputation is stale (older than 24 hours)
	if time.Since(rep.CalculatedAt) > 24*time.Hour {
		// Recalculate
		return s.calculateAndStore(ctx, actorID)
	}

	return &rep, nil
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

	// Get actor data
	actor, err := s.storage.GetActor(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	// Get account age
	if actor.Published != nil {
		input.AccountCreated = *actor.Published
	} else {
		input.AccountCreated = time.Now().AddDate(-1, 0, 0) // Default to 1 year
	}

	// Get activity metrics
	// Get post count
	postCount, err := s.storage.GetStatusCount(ctx, actorID)
	if err != nil {
		s.logger.Warn("Failed to get post count", zap.Error(err))
		postCount = 0
	}
	input.PostCount = postCount

	// Get follower count
	followerCount, err := s.storage.GetFollowerCount(ctx, actorID)
	if err != nil {
		s.logger.Warn("Failed to get follower count", zap.Error(err))
		input.FollowerCount = 0
	} else {
		input.FollowerCount = followerCount
	}

	// Get last activity time
	lastStatus, err := s.storage.GetLatestStatus(ctx, actorID)
	if err != nil {
		s.logger.Warn("Failed to get last activity", zap.Error(err))
		input.LastActive = time.Now()
	} else if lastStatus != nil && !lastStatus.Published.IsZero() {
		input.LastActive = lastStatus.Published
	} else {
		input.LastActive = time.Now()
	}

	// Get trust relationships
	trustRelationships := []TrustRelationship{}

	// Get relationships where this actor trusts others
	trusting, _, err := s.storage.GetTrustRelationships(ctx, actorID, 100, "")
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
	trustedBy, _, err := s.storage.GetTrustedByRelationships(ctx, actorID, 100, "")
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
	events, _, err := s.storage.GetModerationEventsByActor(ctx, actorID, 100, "")
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
			modEvent.Severity = int(event.Severity)

			moderationHistory = append(moderationHistory, modEvent)
		}
	}

	// Get reports filed against this actor
	// GetReportsByTarget already exists in storage interface
	reports, _, err := s.storage.GetReportsByTarget(ctx, actorID, 100, "")
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
	communityNotes, _, err := s.storage.GetCommunityNotesByAuthor(ctx, actorID, 1000, "")
	if err != nil {
		s.logger.Warn("Failed to get community notes", zap.Error(err))
		input.CommunityNotes = 0
	} else {
		input.CommunityNotes = len(communityNotes)
	}

	// Query helpful votes on this actor's community notes
	helpfulVotes := 0
	for _, note := range communityNotes {
		votes, err := s.storage.GetCommunityNoteVotes(ctx, note.ID)
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

// storeReputation stores reputation in DynamoDB
func (s *Service) storeReputation(ctx context.Context, rep *Reputation) error {
	s.costTracker.TrackDynamoWrite(1)

	// Marshal reputation to JSON
	repJSON, err := json.Marshal(rep)
	if err != nil {
		return fmt.Errorf("failed to marshal reputation: %w", err)
	}

	// Create item
	item := map[string]types.AttributeValue{
		"PK":             &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", rep.ActorID)},
		"SK":             &types.AttributeValueMemberS{Value: fmt.Sprintf("REP#%s", rep.CalculatedAt.Format(time.RFC3339))},
		"ReputationData": &types.AttributeValueMemberS{Value: string(repJSON)},
		"Signature":      &types.AttributeValueMemberS{Value: rep.Signature},
		"TTL":            &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(90*24*time.Hour).Unix())},
	}

	_, err = s.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.repTableName),
		Item:      item,
	})

	return err
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
