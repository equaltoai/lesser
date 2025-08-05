package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/trust"
)

// TrustRepository handles trust-related operations using DynamORM
type TrustRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewTrustRepository creates a new trust repository
func NewTrustRepository(db core.DB, logger *zap.Logger) *TrustRepository {
	return &TrustRepository{
		db:     db,
		logger: logger,
	}
}

// convertToModelEvidence converts storage.TrustEvidence to models.TrustEvidence
func convertToModelEvidence(evidence []storage.TrustEvidence) []models.TrustEvidence {
	result := make([]models.TrustEvidence, len(evidence))
	for i, e := range evidence {
		result[i] = models.TrustEvidence{
			Type:        e.Type,
			Score:       e.Score,
			Description: e.Description,
			Timestamp:   e.Timestamp,
		}
	}
	return result
}

// convertFromModelEvidence converts models.TrustEvidence to storage.TrustEvidence
func convertFromModelEvidence(evidence []models.TrustEvidence) []storage.TrustEvidence {
	result := make([]storage.TrustEvidence, len(evidence))
	for i, e := range evidence {
		result[i] = storage.TrustEvidence{
			Type:        e.Type,
			Score:       e.Score,
			Description: e.Description,
			Timestamp:   e.Timestamp,
		}
	}
	return result
}

// isNotFound checks if an error is a not found error
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "item not found" || err.Error() == "no items found"
}

// CreateTrustRelationship creates or updates a trust relationship
func (r *TrustRepository) CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	if relationship == nil {
		return fmt.Errorf("relationship cannot be nil")
	}

	r.logger.Debug("creating trust relationship",
		zap.String("truster", relationship.TrusterID),
		zap.String("trustee", relationship.TrusteeID),
		zap.String("category", string(relationship.Category)))

	model := &models.TrustRelationship{
		ID:         relationship.ID,
		TrusterID:  relationship.TrusterID,
		TrusteeID:  relationship.TrusteeID,
		Category:   models.TrustCategory(relationship.Category),
		Score:      relationship.Score,
		Confidence: relationship.Confidence,
		Evidence:   convertToModelEvidence(relationship.Evidence),
		Created:    relationship.Created,
		Updated:    relationship.Updated,
		TTL:        relationship.TTL,
	}

	model.UpdateKeys()

	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("failed to create trust relationship", zap.Error(err),
			zap.String("truster", relationship.TrusterID),
			zap.String("trustee", relationship.TrusteeID),
			zap.String("category", string(relationship.Category)))
		return err
	}

	// Invalidate cached trust scores
	r.invalidateTrustScoreCache(ctx, relationship.TrusteeID, string(relationship.Category))

	return nil
}

// GetTrustRelationship retrieves a specific trust relationship
func (r *TrustRepository) GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
	model := &models.TrustRelationship{}

	pk := fmt.Sprintf("TRUST#%s#%s", trusterID, category)
	sk := fmt.Sprintf("TRUSTEE#%s", trusteeID)

	err := r.db.WithContext(ctx).Model(&models.TrustRelationship{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(model)

	if err != nil {
		if isNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	return r.modelToTrustRelationship(model), nil
}

// UpdateTrustRelationship updates an existing trust relationship
func (r *TrustRepository) UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	// Just use CreateTrustRelationship as it's an upsert operation
	relationship.Updated = time.Now()
	return r.CreateTrustRelationship(ctx, relationship)
}

// DeleteTrustRelationship removes a trust relationship
func (r *TrustRepository) DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error {
	model := &models.TrustRelationship{
		PK: fmt.Sprintf("TRUST#%s#%s", trusterID, category),
		SK: fmt.Sprintf("TRUSTEE#%s", trusteeID),
	}

	if err := r.db.WithContext(ctx).Model(model).Delete(); err != nil {
		r.logger.Error("failed to delete trust relationship", zap.Error(err),
			zap.String("truster", trusterID),
			zap.String("trustee", trusteeID),
			zap.String("category", category))
		return err
	}

	// Invalidate cached trust scores
	r.invalidateTrustScoreCache(ctx, trusteeID, category)

	return nil
}

// GetTrustRelationships retrieves all trust relationships for a truster
func (r *TrustRepository) GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	// Query by truster using the primary key pattern
	var trustModels []models.TrustRelationship
	query := r.db.WithContext(ctx).Model(&models.TrustRelationship{}).
		Where("PK", "begins_with", fmt.Sprintf("TRUST#%s#", trusterID))
	
	if cursor != "" {
		query = query.Cursor(cursor)
	}
	
	// Get one more item than requested to determine if there are more results
	err := query.Limit(limit + 1).Scan(&trustModels)
	
	// Generate next cursor
	var nextCursor string
	if len(trustModels) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = trustModels[limit-1].SK
		trustModels = trustModels[:limit] // Trim to requested limit
	}

	if err != nil {
		return nil, "", err
	}

	relationships := make([]*storage.TrustRelationship, 0)
	for _, model := range trustModels {
		relationships = append(relationships, r.modelToTrustRelationship(&model))
	}

	return relationships, nextCursor, nil
}

// GetTrustedByRelationships retrieves all relationships where the actor is trusted
func (r *TrustRepository) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	// Query using GSI1 for reverse lookup
	var trustModels []models.TrustRelationship
	query := r.db.WithContext(ctx).Model(&models.TrustRelationship{}).
		Index("gsi1-index").
		Where("GSI1PK", "begins_with", fmt.Sprintf("TRUSTED#%s#", trusteeID))
	
	if cursor != "" {
		query = query.Cursor(cursor)
	}
	
	// Get one more item than requested to determine if there are more results
	err := query.Limit(limit + 1).Scan(&trustModels)
	
	// Generate next cursor
	var nextCursor string
	if len(trustModels) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = trustModels[limit-1].GSI1SK
		trustModels = trustModels[:limit] // Trim to requested limit
	}

	if err != nil {
		return nil, "", err
	}

	relationships := make([]*storage.TrustRelationship, 0)
	for _, model := range trustModels {
		relationships = append(relationships, r.modelToTrustRelationship(&model))
	}

	return relationships, nextCursor, nil
}

// GetTrustScore retrieves a cached trust score or calculates it
func (r *TrustRepository) GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	// Try to get cached score first
	cacheModel := &models.TrustScore{}
	pk := fmt.Sprintf("SCORE#%s#%s", actorID, category)

	err := r.db.WithContext(ctx).Model(cacheModel).
		Where("PK", "=", pk).
		Where("SK", "=", "CURRENT").
		First(cacheModel)

	if err == nil && !cacheModel.CacheTTL.Before(time.Now()) {
		// Cache hit and not expired
		return r.modelToTrustScore(cacheModel), nil
	}

	// Calculate fresh score
	score, err := r.calculateTrustScore(ctx, actorID, category)
	if err != nil {
		return nil, err
	}

	// Cache the calculated score
	if err := r.UpdateTrustScore(ctx, score); err != nil {
		r.logger.Warn("failed to cache trust score", zap.Error(err),
			zap.String("actor", actorID),
			zap.String("category", category))
	}

	return score, nil
}

// UpdateTrustScore updates a cached trust score
func (r *TrustRepository) UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error {
	if score == nil {
		return fmt.Errorf("score cannot be nil")
	}

	model := &models.TrustScore{
		ActorID:         score.ActorID,
		Category:        models.TrustCategory(score.Category),
		Score:           score.Score,
		DirectScore:     score.DirectScore,
		PropagatedScore: score.PropagatedScore,
		Confidence:      score.Confidence,
		TrusterCount:    score.TrusterCount,
		CategoryScores:  score.CategoryScores,
		LastCalculated:  score.LastCalculated,
		CacheTTL:        score.CacheTTL,
	}

	model.UpdateKeys()

	return r.db.WithContext(ctx).Model(model).Create()
}

// RecordTrustUpdate records a trust score update event
func (r *TrustRepository) RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error {
	if update == nil {
		return fmt.Errorf("update cannot be nil")
	}

	model := &models.TrustUpdate{
		ActorID:   update.ActorID,
		EventID:   update.EventID,
		Category:  models.TrustCategory(update.Category),
		Delta:     update.Delta,
		Reason:    update.Reason,
		Timestamp: update.Timestamp,
	}

	model.UpdateKeys()

	return r.db.WithContext(ctx).Model(model).Create()
}

// GetAllTrustRelationships retrieves all trust relationships for admin visualization
func (r *TrustRepository) GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error) {
	// Scan for all trust relationships
	var trustModels []models.TrustRelationship
	err := r.db.WithContext(ctx).Model(&models.TrustRelationship{}).
		Where("Type", "=", "RELATIONSHIP").
		Limit(limit).
		Scan(&trustModels)
	if err != nil {
		return nil, err
	}

	relationships := make([]*storage.TrustRelationship, len(trustModels))
	for i, model := range trustModels {
		relationships[i] = r.modelToTrustRelationship(&model)
	}

	return relationships, nil
}

// GetUserTrustScore retrieves the trust score for a user
func (r *TrustRepository) GetUserTrustScore(ctx context.Context, userID string) (float64, error) {
	// Get general trust score
	score, err := r.GetTrustScore(ctx, userID, string(trust.TrustCategoryGeneral))
	if err != nil {
		if err == storage.ErrNotFound {
			return 0.5, nil // Default neutral score
		}
		return 0.0, err
	}

	return score.Score, nil
}

// invalidateTrustScoreCache invalidates cached trust scores for an actor
func (r *TrustRepository) invalidateTrustScoreCache(ctx context.Context, actorID, category string) {
	// Delete cached score to force recalculation
	model := &models.TrustScore{
		PK: fmt.Sprintf("SCORE#%s#%s", actorID, category),
		SK: "CURRENT",
	}

	if err := r.db.WithContext(ctx).Model(model).Delete(); err != nil {
		r.logger.Debug("failed to invalidate trust score cache", zap.Error(err),
			zap.String("actor", actorID),
			zap.String("category", category))
	}
}

// calculateTrustScore calculates the trust score for an actor using PageRank-inspired algorithm
func (r *TrustRepository) calculateTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	score := &storage.TrustScore{
		ActorID:         actorID,
		Category:        storage.TrustCategory(category),
		Score:           0.5, // Start with neutral
		DirectScore:     0.0,
		PropagatedScore: 0.0,
		Confidence:      0.0,
		TrusterCount:    0,
		CategoryScores:  make(map[string]float64),
		LastCalculated:  time.Now(),
		CacheTTL:        time.Now().Add(2 * time.Hour), // Cache for 2 hours
	}

	// Get direct trust relationships (who trusts this actor)
	relationships, _, err := r.GetTrustedByRelationships(ctx, actorID, 100, "")
	if err != nil {
		return score, err
	}

	// Calculate direct trust score
	totalDirectScore := 0.0
	totalWeight := 0.0
	trusterScores := make(map[string]float64)

	for _, rel := range relationships {
		// Weight by category relevance and confidence
		weight := rel.Confidence
		if string(rel.Category) == category || rel.Category == trust.TrustCategoryGeneral {
			weight *= 1.0 // Full weight for matching/general category
		} else {
			weight *= 0.5 // Reduced weight for other categories
		}

		if weight > 0 {
			score.TrusterCount++
			trusterScores[rel.TrusterID] = rel.Score * weight
			totalDirectScore += rel.Score * weight
			totalWeight += weight
		}
	}

	if totalWeight > 0 {
		score.DirectScore = totalDirectScore / totalWeight
		score.Confidence = totalWeight / float64(score.TrusterCount)
	}

	// Calculate network propagation (simplified PageRank)
	const (
		maxHops         = 2   // Maximum propagation depth
		propagationRate = 0.5 // Trust diminishes by 50% per hop
		minTrustScore   = 0.1 // Minimum trust score to propagate
	)

	// Queue for BFS traversal
	type node struct {
		actorID   string
		hopCount  int
		pathTrust float64
	}

	visited := make(map[string]bool)
	queue := []node{{actorID: actorID, hopCount: 0, pathTrust: 1.0}}
	visited[actorID] = true

	propagatedTrust := 0.0
	propagatedWeight := 0.0

	for len(queue) > 0 && queue[0].hopCount < maxHops {
		current := queue[0]
		queue = queue[1:]

		// Get trust from this node
		if trustValue, exists := trusterScores[current.actorID]; exists && trustValue >= minTrustScore {
			// This node contributes to propagated trust
			propagatedWeight += current.pathTrust
		}

		// Add neighbors for next hop
		if current.hopCount+1 < maxHops {
			// Get the trust score for this node to determine if it's trustworthy enough to propagate
			nodeScore, err := r.GetTrustScore(ctx, current.actorID, category)
			if err != nil {
				// If we can't get the score, skip this node
				continue
			}

			// Only propagate trust through nodes with reasonable trust scores
			if nodeScore.Score < minTrustScore {
				continue
			}

			// Trust diminishes with each hop (propagationRate) and is weighted by the path trust
			nextPathTrust := current.pathTrust * propagationRate * nodeScore.Score

			if nextPathTrust >= 0.01 { // Only continue if meaningful trust remains
				propagatedTrust += nextPathTrust

				// Get this node's trust relationships to continue propagation
				nodeRelationships, _, err := r.GetTrustedByRelationships(ctx, current.actorID, 50, "")
				if err == nil {
					for _, rel := range nodeRelationships {
						if !visited[rel.TrusterID] && (string(rel.Category) == category || rel.Category == trust.TrustCategoryGeneral) {
							queue = append(queue, node{
								actorID:   rel.TrusterID,
								hopCount:  current.hopCount + 1,
								pathTrust: nextPathTrust,
							})
							visited[rel.TrusterID] = true
						}
					}
				}
			}
		}
	}

	// Combine propagated trust
	if propagatedWeight > 0 {
		score.PropagatedScore = propagatedTrust / propagatedWeight
	}

	// Final score combines direct and propagated trust
	directWeight := 0.7
	propagatedWeight = 0.3

	if score.TrusterCount == 0 {
		// No direct trust, rely more on propagation
		directWeight = 0.3
		propagatedWeight = 0.7
	}

	score.Score = (score.DirectScore * directWeight) + (score.PropagatedScore * propagatedWeight)

	// Ensure score is within bounds
	if score.Score > 1.0 {
		score.Score = 1.0
	}
	if score.Score < 0.0 {
		score.Score = 0.0
	}

	return score, nil
}

// modelToTrustRelationship converts a model to storage type
func (r *TrustRepository) modelToTrustRelationship(model *models.TrustRelationship) *storage.TrustRelationship {
	return &storage.TrustRelationship{
		ID:         model.ID,
		TrusterID:  model.TrusterID,
		TrusteeID:  model.TrusteeID,
		Category:   storage.TrustCategory(model.Category),
		Score:      model.Score,
		Confidence: model.Confidence,
		Evidence:   convertFromModelEvidence(model.Evidence),
		Created:    model.Created,
		Updated:    model.Updated,
		TTL:        model.TTL,
	}
}

// modelToTrustScore converts a model to storage type
func (r *TrustRepository) modelToTrustScore(model *models.TrustScore) *storage.TrustScore {
	return &storage.TrustScore{
		ActorID:         model.ActorID,
		Category:        storage.TrustCategory(model.Category),
		Score:           model.Score,
		DirectScore:     model.DirectScore,
		PropagatedScore: model.PropagatedScore,
		Confidence:      model.Confidence,
		TrusterCount:    model.TrusterCount,
		CategoryScores:  model.CategoryScores,
		LastCalculated:  model.LastCalculated,
		CacheTTL:        model.CacheTTL,
	}
}
