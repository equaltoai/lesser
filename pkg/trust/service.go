package trust

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage"
)

// TrustRepositoryInterface defines the interface for trust operations
type TrustRepositoryInterface interface {
	CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error
	GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error)
	UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error
	DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error
	GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error)
	GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error)
	GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error)
	UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error
	RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error
	GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error)
	GetUserTrustScore(ctx context.Context, userID string) (float64, error)
}

// Service provides methods for managing trust relationships
type Service struct {
	repo   TrustRepositoryInterface
	logger *zap.Logger
}

// NewService creates a new trust service
func NewService(repo TrustRepositoryInterface, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// GetTrustScore retrieves the trust score between two actors
func (s *Service) GetTrustScore(ctx context.Context, fromActor, toActor string) (*TrustScore, error) {
	// First try to get the direct trust relationship
	relationship, err := s.repo.GetTrustRelationship(ctx, fromActor, toActor, string(TrustCategoryGeneral))
	if err != nil && err != storage.ErrNotFound {
		return nil, err
	}

	if relationship != nil {
		// Direct relationship exists
		return &TrustScore{
			ActorID:         toActor,
			Category:        TrustCategoryGeneral,
			Score:           relationship.Score,
			DirectScore:     relationship.Score,
			PropagatedScore: 0.0,
			Confidence:      relationship.Confidence,
			TrusterCount:    1,
			LastCalculated:  time.Now(),
			CacheTTL:        time.Now().Add(time.Hour),
		}, nil
	}

	// No direct relationship, get calculated score
	calculatedScore, err := s.repo.GetTrustScore(ctx, toActor, string(TrustCategoryGeneral))
	if err != nil {
		if err == storage.ErrNotFound {
			// No trust data available, return default neutral score
			return &TrustScore{
				ActorID:         toActor,
				Category:        TrustCategoryGeneral,
				Score:           0.5, // Default neutral trust
				DirectScore:     0.5,
				PropagatedScore: 0.0,
				Confidence:      0.0,
				TrusterCount:    0,
				LastCalculated:  time.Now(),
				CacheTTL:        time.Now().Add(time.Hour),
			}, nil
		}
		return nil, err
	}

	// Convert storage.TrustScore to trust.TrustScore
	return &TrustScore{
		ActorID:         calculatedScore.ActorID,
		Category:        TrustCategory(calculatedScore.Category),
		Score:           calculatedScore.Score,
		DirectScore:     calculatedScore.DirectScore,
		PropagatedScore: calculatedScore.PropagatedScore,
		Confidence:      calculatedScore.Confidence,
		TrusterCount:    calculatedScore.TrusterCount,
		LastCalculated:  calculatedScore.LastCalculated,
		CacheTTL:        calculatedScore.CacheTTL,
		CategoryScores:  calculatedScore.CategoryScores,
	}, nil
}

// CreateTrustRelationship creates a new trust relationship
func (s *Service) CreateTrustRelationship(ctx context.Context, relationship *TrustRelationship) error {
	if relationship == nil {
		return fmt.Errorf("relationship cannot be nil")
	}

	storageRel := &storage.TrustRelationship{
		ID:         relationship.ID,
		TrusterID:  relationship.TrusterID,
		TrusteeID:  relationship.TrusteeID,
		Category:   storage.TrustCategory(relationship.Category),
		Score:      relationship.Score,
		Confidence: relationship.Confidence,
		Evidence:   convertTrustEvidence(relationship.Evidence),
		Created:    relationship.Created,
		Updated:    relationship.Updated,
		TTL:        relationship.TTL,
	}

	return s.repo.CreateTrustRelationship(ctx, storageRel)
}

// UpdateTrustRelationship updates an existing trust relationship
func (s *Service) UpdateTrustRelationship(ctx context.Context, relationship *TrustRelationship) error {
	if relationship == nil {
		return fmt.Errorf("relationship cannot be nil")
	}

	storageRel := &storage.TrustRelationship{
		ID:         relationship.ID,
		TrusterID:  relationship.TrusterID,
		TrusteeID:  relationship.TrusteeID,
		Category:   storage.TrustCategory(relationship.Category),
		Score:      relationship.Score,
		Confidence: relationship.Confidence,
		Evidence:   convertTrustEvidence(relationship.Evidence),
		Created:    relationship.Created,
		Updated:    relationship.Updated,
		TTL:        relationship.TTL,
	}

	return s.repo.UpdateTrustRelationship(ctx, storageRel)
}

// DeleteTrustRelationship removes a trust relationship
func (s *Service) DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID string, category TrustCategory) error {
	return s.repo.DeleteTrustRelationship(ctx, trusterID, trusteeID, string(category))
}

// GetTrustRelationships retrieves trust relationships for a truster
func (s *Service) GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*TrustRelationship, string, error) {
	storageRels, nextCursor, err := s.repo.GetTrustRelationships(ctx, trusterID, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	relationships := make([]*TrustRelationship, len(storageRels))
	for i, rel := range storageRels {
		relationships[i] = convertFromStorageTrustRelationship(rel)
	}

	return relationships, nextCursor, nil
}

// GetTrustedByRelationships retrieves relationships where the actor is trusted
func (s *Service) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*TrustRelationship, string, error) {
	storageRels, nextCursor, err := s.repo.GetTrustedByRelationships(ctx, trusteeID, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	relationships := make([]*TrustRelationship, len(storageRels))
	for i, rel := range storageRels {
		relationships[i] = convertFromStorageTrustRelationship(rel)
	}

	return relationships, nextCursor, nil
}

// RecordTrustUpdate records a trust score update event
func (s *Service) RecordTrustUpdate(ctx context.Context, update *TrustUpdate) error {
	if update == nil {
		return fmt.Errorf("update cannot be nil")
	}

	storageUpdate := &storage.TrustUpdate{
		ActorID:   update.ActorID,
		Category:  storage.TrustCategory(update.Category),
		Delta:     update.Delta,
		Reason:    update.Reason,
		EventID:   update.EventID,
		Timestamp: update.Timestamp,
	}

	return s.repo.RecordTrustUpdate(ctx, storageUpdate)
}

// GetUserTrustScore retrieves the general trust score for a user
func (s *Service) GetUserTrustScore(ctx context.Context, userID string) (float64, error) {
	return s.repo.GetUserTrustScore(ctx, userID)
}

// GetTrustSummary provides a summary view of an actor's trust status
func (s *Service) GetTrustSummary(ctx context.Context, actorID string) (*TrustSummary, error) {
	summary := &TrustSummary{
		ActorID:        actorID,
		CategoryScores: make(map[TrustCategory]float64),
		LastActive:     time.Now(),
	}

	// Get scores for each category
	categories := []TrustCategory{
		TrustCategoryGeneral,
		TrustCategoryContent,
		TrustCategoryBehavior,
		TrustCategoryTechnical,
	}

	var totalScore float64
	var scoreCount int

	for _, category := range categories {
		score, err := s.repo.GetTrustScore(ctx, actorID, string(category))
		if err != nil && err != storage.ErrNotFound {
			s.logger.Warn("failed to get trust score for category",
				zap.String("actor", actorID),
				zap.String("category", string(category)),
				zap.Error(err))
			continue
		}

		if score != nil {
			summary.CategoryScores[category] = score.Score
			totalScore += score.Score
			scoreCount++

			if category == TrustCategoryGeneral {
				summary.TrustedByCount = score.TrusterCount
			}
		} else {
			summary.CategoryScores[category] = 0.5 // Default neutral
			totalScore += 0.5
			scoreCount++
		}
	}

	// Calculate overall score
	if scoreCount > 0 {
		summary.OverallScore = totalScore / float64(scoreCount)
	} else {
		summary.OverallScore = 0.5
	}

	// Determine reputation level based on overall score
	switch {
	case summary.OverallScore >= 0.8:
		summary.ReputationLevel = "high"
	case summary.OverallScore >= 0.6:
		summary.ReputationLevel = "medium"
	case summary.OverallScore >= 0.4:
		summary.ReputationLevel = "low"
	default:
		summary.ReputationLevel = "new"
	}

	// Get count of actors this one trusts
	relationships, _, err := s.repo.GetTrustRelationships(ctx, actorID, 1000, "")
	if err == nil {
		summary.TrustsCount = len(relationships)
	}

	return summary, nil
}

// Helper functions for type conversion

func convertTrustEvidence(evidence []TrustEvidence) []storage.TrustEvidence {
	storageEvidence := make([]storage.TrustEvidence, len(evidence))
	for i, e := range evidence {
		storageEvidence[i] = storage.TrustEvidence{
			Type:        e.Type,
			Score:       e.Score,
			Description: e.Description,
			Timestamp:   e.Timestamp,
		}
	}
	return storageEvidence
}

func convertFromStorageTrustRelationship(rel *storage.TrustRelationship) *TrustRelationship {
	evidence := make([]TrustEvidence, len(rel.Evidence))
	for i, e := range rel.Evidence {
		evidence[i] = TrustEvidence{
			Type:        e.Type,
			Score:       e.Score,
			Description: e.Description,
			Timestamp:   e.Timestamp,
		}
	}

	return &TrustRelationship{
		ID:         rel.ID,
		TrusterID:  rel.TrusterID,
		TrusteeID:  rel.TrusteeID,
		Category:   TrustCategory(rel.Category),
		Score:      rel.Score,
		Confidence: rel.Confidence,
		Evidence:   evidence,
		Created:    rel.Created,
		Updated:    rel.Updated,
		TTL:        rel.TTL,
	}
}
