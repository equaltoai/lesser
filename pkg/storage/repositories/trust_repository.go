package repositories

import (
	"context"
	"encoding/base64"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/theory-cloud/tabletheory/pkg/core"
	dmerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/trust"
)

type trustRelationshipCursor struct {
	Category string `json:"category"`
	LastSK   string `json:"last_sk"`
}

func encodeTrustRelationshipCursor(cursor trustRelationshipCursor) string {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeTrustRelationshipCursor(encoded string) (trustRelationshipCursor, error) {
	if strings.TrimSpace(encoded) == "" {
		return trustRelationshipCursor{}, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return trustRelationshipCursor{}, err
	}

	var cursor trustRelationshipCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return trustRelationshipCursor{}, err
	}

	cursor.Category = strings.TrimSpace(cursor.Category)
	cursor.LastSK = strings.TrimSpace(cursor.LastSK)
	return cursor, nil
}

var trustRelationshipCategoryOrder = []models.TrustCategory{
	models.TrustCategoryContent,
	models.TrustCategoryBehavior,
	models.TrustCategoryTechnical,
	models.TrustCategoryGeneral,
}

func trustCategoryIndex(category string) int {
	for i, candidate := range trustRelationshipCategoryOrder {
		if category == string(candidate) {
			return i
		}
	}
	return -1
}

// TrustRepository handles trust-related operations using enhanced repository patterns
type TrustRepository struct {
	*EnhancedBaseRepository[*models.TrustRelationship]
	scoreRepo  *EnhancedBaseRepository[*models.TrustScore]
	updateRepo *EnhancedBaseRepository[*models.TrustUpdate]
	logger     *zap.Logger
}

// NewTrustRepository creates a new trust repository with enhanced functionality
func NewTrustRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *TrustRepository {
	// Create enhanced repositories for trust operations
	trustRepo := NewEnhancedBaseRepository[*models.TrustRelationship](db, tableName, logger, costService, "TrustRepository", "trust")
	trustRepo.SetValidationService(NewDefaultValidationService())
	trustRepo.SetPermissionService(NewDefaultPermissionService()) // Trust relationship permissions
	trustRepo.SetCachingService(NewInMemoryCachingService())      // Cache trust data
	trustRepo.SetEventService(NewDefaultEventService())           // Trust relationship events

	scoreRepo := NewEnhancedBaseRepository[*models.TrustScore](db, tableName, logger, costService, "TrustRepository.Score", "trust_score")
	scoreRepo.SetValidationService(NewDefaultValidationService())
	scoreRepo.SetPermissionService(NewDefaultPermissionService())
	scoreRepo.SetCachingService(NewInMemoryCachingService()) // Cache trust scores
	scoreRepo.SetEventService(NewDefaultEventService())

	updateRepo := NewEnhancedBaseRepository[*models.TrustUpdate](db, tableName, logger, costService, "TrustRepository.Update", "trust_update")
	updateRepo.SetValidationService(NewDefaultValidationService())
	updateRepo.SetPermissionService(NewDefaultPermissionService())
	updateRepo.SetCachingService(NewInMemoryCachingService())
	updateRepo.SetEventService(NewDefaultEventService())

	return &TrustRepository{
		EnhancedBaseRepository: trustRepo,
		scoreRepo:              scoreRepo,
		updateRepo:             updateRepo,
		logger:                 logger,
	}
}

// convertToModelEvidence converts storage.TrustEvidence to models.TrustEvidence
// Since storage.TrustEvidence is an alias for models.TrustEvidence, no conversion needed
func convertToModelEvidence(evidence []storage.TrustEvidence) []models.TrustEvidence {
	return evidence
}

// convertFromModelEvidence converts models.TrustEvidence to storage.TrustEvidence
// Since storage.TrustEvidence is an alias for models.TrustEvidence, no conversion needed
func convertFromModelEvidence(evidence []models.TrustEvidence) []storage.TrustEvidence {
	return evidence
}

// CreateTrustRelationship creates or updates a trust relationship
func (r *TrustRepository) CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	if relationship == nil {
		return common.ValidationError{Field: "relationship", Message: "cannot be nil"}
	}

	r.logger.Debug("creating trust relationship",
		zap.String("truster", relationship.TrusterID),
		zap.String("trustee", relationship.TrusteeID),
		zap.String("category", string(relationship.Category)))

	model := &models.TrustRelationship{
		ID:         relationship.ID,
		TrusterID:  relationship.TrusterID,
		TrusteeID:  relationship.TrusteeID,
		Category:   relationship.Category,
		Score:      relationship.Score,
		Confidence: relationship.Confidence,
		Evidence:   convertToModelEvidence(relationship.Evidence),
		Created:    relationship.Created,
		Updated:    relationship.Updated,
		TTL:        relationship.TTL,
	}

	// Update keys (sets PK, SK, GSI keys, and Type field)
	if err := model.UpdateKeys(); err != nil {
		r.logger.Error("failed to update trust relationship keys", zap.Error(err))
		return err
	}

	// Use enhanced validation and creation
	if err := r.ValidateAndCreate(ctx, model); err != nil {
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

	// Use BaseRepository Get method
	err := r.Get(ctx, pk, sk, model)
	if err != nil {
		if err.Error() == fmt.Sprintf("item not found: pk=%s, sk=%s", pk, sk) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	return r.modelToTrustRelationship(model), nil
}

// UpdateTrustRelationship updates an existing trust relationship
func (r *TrustRepository) UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	if relationship == nil {
		return common.ValidationError{Field: "relationship", Message: "cannot be nil"}
	}

	r.logger.Debug("updating trust relationship",
		zap.String("truster", relationship.TrusterID),
		zap.String("trustee", relationship.TrusteeID),
		zap.String("category", string(relationship.Category)))

	model := &models.TrustRelationship{
		ID:         relationship.ID,
		TrusterID:  relationship.TrusterID,
		TrusteeID:  relationship.TrusteeID,
		Category:   relationship.Category,
		Score:      relationship.Score,
		Confidence: relationship.Confidence,
		Evidence:   convertToModelEvidence(relationship.Evidence),
		Created:    relationship.Created,
		Updated:    time.Now(), // Always update the timestamp
		TTL:        relationship.TTL,
	}

	// Update keys (sets PK, SK, GSI keys, and Type field)
	if err := model.UpdateKeys(); err != nil {
		r.logger.Error("failed to update trust relationship keys", zap.Error(err))
		return err
	}

	// Check if relationship exists - if not, create it; if yes, update it
	existingPK := fmt.Sprintf("TRUST#%s#%s", relationship.TrusterID, relationship.Category)
	existingSK := fmt.Sprintf("TRUSTEE#%s", relationship.TrusteeID)
	var existing models.TrustRelationship
	err := r.Get(ctx, existingPK, existingSK, &existing)

	if err != nil {
		// Item doesn't exist, create it
		// Check for NotFound in multiple ways since error format can vary
		if dmerrors.IsNotFound(err) ||
			strings.Contains(strings.ToLower(err.Error()), "not found") ||
			strings.Contains(strings.ToLower(err.Error()), "item not found") {
			r.logger.Debug("trust relationship does not exist, creating new one",
				zap.String("truster", relationship.TrusterID),
				zap.String("trustee", relationship.TrusteeID))
			return r.CreateTrustRelationship(ctx, relationship)
		}
		r.logger.Error("failed to check if trust relationship exists",
			zap.String("truster", relationship.TrusterID),
			zap.String("trustee", relationship.TrusteeID),
			zap.Error(err))
		return err
	}

	// Item exists, update it using UpdateBuilder
	r.logger.Debug("trust relationship exists, updating",
		zap.String("truster", relationship.TrusterID),
		zap.String("trustee", relationship.TrusteeID))

	updateBuilder := r.db.WithContext(ctx).Model(&models.TrustRelationship{}).
		Where("PK", "=", existingPK).
		Where("SK", "=", existingSK).
		UpdateBuilder()

	updateBuilder.Set("Score", model.Score)
	updateBuilder.Set("Confidence", model.Confidence)
	updateBuilder.Set("Evidence", model.Evidence)
	updateBuilder.Set("Updated", model.Updated)
	if model.TTL > 0 {
		updateBuilder.Set("TTL", model.TTL)
	}

	if err := updateBuilder.Execute(); err != nil {
		r.logger.Error("failed to update trust relationship", zap.Error(err),
			zap.String("truster", relationship.TrusterID),
			zap.String("trustee", relationship.TrusteeID))
		return err
	}

	// Invalidate cached trust scores
	r.invalidateTrustScoreCache(ctx, relationship.TrusteeID, string(relationship.Category))

	return nil
}

// DeleteTrustRelationship removes a trust relationship
func (r *TrustRepository) DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error {
	pk := fmt.Sprintf("TRUST#%s#%s", trusterID, category)
	sk := fmt.Sprintf("TRUSTEE#%s", trusteeID)

	// Use BaseRepository Delete method
	if err := r.Delete(ctx, pk, sk); err != nil {
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
	decodedCursor, err := decodeTrustRelationshipCursor(cursor)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "trust relationship", "cursor decode")
	}

	startCategoryIndex := 0
	categoryCursorSK := decodedCursor.LastSK
	if decodedCursor.Category != "" {
		startCategoryIndex = trustCategoryIndex(decodedCursor.Category)
		if startCategoryIndex < 0 {
			return nil, "", ErrorHandler.HandleQueryError(storage.ErrInvalidInput, "trust relationship", "invalid cursor category")
		}
	}

	relationships := make([]*storage.TrustRelationship, 0, limit)
	for i := startCategoryIndex; i < len(trustRelationshipCategoryOrder) && len(relationships) < limit; i++ {
		category := trustRelationshipCategoryOrder[i]
		pk := fmt.Sprintf("TRUST#%s#%s", trusterID, category)

		remaining := limit - len(relationships)
		query := r.GetDB().WithContext(ctx).Model(&models.TrustRelationship{}).
			Where("PK", "=", pk).
			OrderBy("SK", "ASC").
			Limit(remaining + 1)

		if categoryCursorSK != "" {
			query = query.Where("SK", ">", categoryCursorSK)
		}

		var page []*models.TrustRelationship
		if err := query.All(&page); err != nil {
			return nil, "", ErrorHandler.HandleQueryError(err, "trust relationship", "list")
		}

		if len(page) == 0 {
			categoryCursorSK = ""
			continue
		}

		hasMore := len(page) > remaining
		if hasMore {
			page = page[:remaining]
		}

		for _, model := range page {
			relationships = append(relationships, r.modelToTrustRelationship(model))
		}

		if hasMore {
			nextCursor := encodeTrustRelationshipCursor(trustRelationshipCursor{
				Category: string(category),
				LastSK:   page[len(page)-1].SK,
			})
			return relationships, nextCursor, nil
		}

		// If we hit the requested limit exactly at a category boundary, advance the cursor
		// to the next category so callers can continue pagination.
		if len(relationships) >= limit {
			if i+1 < len(trustRelationshipCategoryOrder) {
				nextCursor := encodeTrustRelationshipCursor(trustRelationshipCursor{
					Category: string(trustRelationshipCategoryOrder[i+1]),
					LastSK:   "",
				})
				return relationships, nextCursor, nil
			}
			return relationships, "", nil
		}

		categoryCursorSK = ""
	}

	return relationships, "", nil
}

// GetTrustedByRelationships retrieves all relationships where the actor is trusted
func (r *TrustRepository) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	decodedCursor, err := decodeTrustRelationshipCursor(cursor)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "trust relationship", "cursor decode")
	}

	startCategoryIndex := 0
	categoryCursorSK := decodedCursor.LastSK
	if decodedCursor.Category != "" {
		startCategoryIndex = trustCategoryIndex(decodedCursor.Category)
		if startCategoryIndex < 0 {
			return nil, "", ErrorHandler.HandleQueryError(storage.ErrInvalidInput, "trust relationship", "invalid cursor category")
		}
	}

	relationships := make([]*storage.TrustRelationship, 0, limit)
	for i := startCategoryIndex; i < len(trustRelationshipCategoryOrder) && len(relationships) < limit; i++ {
		category := trustRelationshipCategoryOrder[i]
		gsi1PK := fmt.Sprintf("TRUSTED#%s#%s", trusteeID, category)

		remaining := limit - len(relationships)
		query := r.GetDB().WithContext(ctx).Model(&models.TrustRelationship{}).
			Index("gsi1").
			Where("gsi1PK", "=", gsi1PK).
			OrderBy("gsi1SK", "ASC").
			Limit(remaining + 1)

		if categoryCursorSK != "" {
			query = query.Where("gsi1SK", ">", categoryCursorSK)
		}

		var page []*models.TrustRelationship
		if err := query.All(&page); err != nil {
			return nil, "", ErrorHandler.HandleQueryError(err, "trust relationship", "list trusted-by")
		}

		if len(page) == 0 {
			categoryCursorSK = ""
			continue
		}

		hasMore := len(page) > remaining
		if hasMore {
			page = page[:remaining]
		}

		for _, model := range page {
			relationships = append(relationships, r.modelToTrustRelationship(model))
		}

		if hasMore {
			nextCursor := encodeTrustRelationshipCursor(trustRelationshipCursor{
				Category: string(category),
				LastSK:   page[len(page)-1].GSI1SK,
			})
			return relationships, nextCursor, nil
		}

		// If we hit the requested limit exactly at a category boundary, advance the cursor
		// to the next category so callers can continue pagination.
		if len(relationships) >= limit {
			if i+1 < len(trustRelationshipCategoryOrder) {
				nextCursor := encodeTrustRelationshipCursor(trustRelationshipCursor{
					Category: string(trustRelationshipCategoryOrder[i+1]),
					LastSK:   "",
				})
				return relationships, nextCursor, nil
			}
			return relationships, "", nil
		}

		categoryCursorSK = ""
	}

	return relationships, "", nil
}

// GetTrustScore retrieves a cached trust score or calculates it
func (r *TrustRepository) GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	start := time.Now()
	r.logger.Info("trust repository get score started",
		zap.String("actor", actorID),
		zap.String("category", category))

	// Try to get cached score first
	cacheModel := &models.TrustScore{}
	pk := fmt.Sprintf("SCORE#%s#%s", actorID, category)
	sk := StatusCurrent

	err := r.scoreRepo.Get(ctx, pk, sk, cacheModel)
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

	r.logger.Info("trust repository get score completed",
		zap.String("actor", actorID),
		zap.String("category", category),
		zap.Duration("duration", time.Since(start)),
		zap.Float64("score", score.Score))

	return score, nil
}

// getCachedTrustScore returns a cached trust score without triggering recalculation.
func (r *TrustRepository) getCachedTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	cacheModel := &models.TrustScore{}
	pk := fmt.Sprintf("SCORE#%s#%s", actorID, category)
	sk := "CURRENT"

	err := r.scoreRepo.Get(ctx, pk, sk, cacheModel)
	if err != nil {
		if dmerrors.IsNotFound(err) || stdErrors.Is(err, storage.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if cacheModel.CacheTTL.Before(time.Now()) {
		return nil, nil
	}

	return r.modelToTrustScore(cacheModel), nil
}

// UpdateTrustScore updates a cached trust score
func (r *TrustRepository) UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error {
	if score == nil {
		return common.ValidationError{Field: "score", Message: "cannot be nil"}
	}

	model := &models.TrustScore{
		ActorID:         score.ActorID,
		Category:        score.Category,
		Score:           score.Score,
		DirectScore:     score.DirectScore,
		PropagatedScore: score.PropagatedScore,
		Confidence:      score.Confidence,
		TrusterCount:    score.TrusterCount,
		CategoryScores:  score.CategoryScores,
		LastCalculated:  score.LastCalculated,
		CacheTTL:        score.CacheTTL,
	}

	if err := model.UpdateKeys(); err != nil {
		return err
	}

	return r.scoreRepo.ValidateAndCreate(ctx, model)
}

// RecordTrustUpdate records a trust score update event
func (r *TrustRepository) RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error {
	if update == nil {
		return common.ValidationError{Field: "update", Message: "cannot be nil"}
	}

	model := &models.TrustUpdate{
		ActorID:   update.ActorID,
		EventID:   update.EventID,
		Category:  update.Category,
		Delta:     update.Delta,
		Reason:    update.Reason,
		Timestamp: update.Timestamp,
	}

	return r.updateRepo.ValidateAndCreate(ctx, model)
}

// GetAllTrustRelationships retrieves all trust relationships for admin visualization
func (r *TrustRepository) GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error) {
	// Scan for all trust relationships
	var trustModels []*models.TrustRelationship
	err := r.GetDB().WithContext(ctx).Model(&models.TrustRelationship{}).
		Where("Type", "=", "RELATIONSHIP").
		Limit(limit).
		Scan(&trustModels)
	if err != nil {
		return nil, err
	}

	relationships := make([]*storage.TrustRelationship, len(trustModels))
	for i, model := range trustModels {
		relationships[i] = r.modelToTrustRelationship(model)
	}

	return relationships, nil
}

// GetUserTrustScore retrieves the trust score for a user
func (r *TrustRepository) GetUserTrustScore(ctx context.Context, userID string) (float64, error) {
	// Get general trust score
	score, err := r.GetTrustScore(ctx, userID, string(trust.TrustCategoryGeneral))
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) {
			return 0.5, nil // Default neutral score
		}
		return 0.0, err
	}

	return score.Score, nil
}

// invalidateTrustScoreCache invalidates cached trust scores for an actor
func (r *TrustRepository) invalidateTrustScoreCache(ctx context.Context, actorID, category string) {
	// Delete cached score to force recalculation
	pk := fmt.Sprintf("SCORE#%s#%s", actorID, category)
	sk := "CURRENT"

	if err := r.scoreRepo.Delete(ctx, pk, sk); err != nil {
		r.logger.Debug("failed to invalidate trust score cache", zap.Error(err),
			zap.String("actor", actorID),
			zap.String("category", category))
	}
}

// calculateTrustScore calculates the trust score for an actor using PageRank-inspired algorithm
func (r *TrustRepository) calculateTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	score := r.initializeTrustScore(actorID, category)

	// Get direct trust relationships (who trusts this actor)
	relationships, _, err := r.GetTrustedByRelationships(ctx, actorID, 100, "")
	if err != nil {
		return score, err
	}

	// Calculate direct trust score
	trusterScores := r.calculateDirectTrust(score, relationships, category)

	// Calculate network propagation
	r.calculatePropagatedTrust(ctx, score, actorID, category, trusterScores)

	// Combine scores and normalize
	r.finalizeTrustScore(score)

	return score, nil
}

// initializeTrustScore creates a new trust score with default values
func (r *TrustRepository) initializeTrustScore(actorID, category string) *storage.TrustScore {
	return &storage.TrustScore{
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
}

// calculateDirectTrust calculates the direct trust score from immediate relationships
func (r *TrustRepository) calculateDirectTrust(score *storage.TrustScore, relationships []*storage.TrustRelationship, category string) map[string]float64 {
	totalDirectScore := 0.0
	totalWeight := 0.0
	trusterScores := make(map[string]float64)

	for _, rel := range relationships {
		weight := r.calculateRelationshipWeight(rel, category)
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

	return trusterScores
}

// calculateRelationshipWeight calculates the weight of a trust relationship
func (r *TrustRepository) calculateRelationshipWeight(rel *storage.TrustRelationship, category string) float64 {
	weight := rel.Confidence
	if string(rel.Category) == category || rel.Category == trust.TrustCategoryGeneral {
		weight *= 1.0 // Full weight for matching/general category
	} else {
		weight *= 0.5 // Reduced weight for other categories
	}
	return weight
}

// trustNode represents a node in the trust propagation graph
type trustNode struct {
	actorID   string
	hopCount  int
	pathTrust float64
}

// trustPropagationConfig contains configuration for trust propagation
type trustPropagationConfig struct {
	maxHops         int
	propagationRate float64
	minTrustScore   float64
}

// calculatePropagatedTrust calculates trust propagated through the network
func (r *TrustRepository) calculatePropagatedTrust(ctx context.Context, score *storage.TrustScore, actorID, category string, trusterScores map[string]float64) {
	config := trustPropagationConfig{
		maxHops:         2,   // Maximum propagation depth
		propagationRate: 0.5, // Trust diminishes by 50% per hop
		minTrustScore:   0.1, // Minimum trust score to propagate
	}

	propagatedTrust, propagatedWeight := r.performTrustPropagation(ctx, actorID, category, trusterScores, config)

	if propagatedWeight > 0 {
		score.PropagatedScore = propagatedTrust / propagatedWeight
	}
}

// performTrustPropagation performs BFS traversal for trust propagation
func (r *TrustRepository) performTrustPropagation(ctx context.Context, actorID, category string, trusterScores map[string]float64, config trustPropagationConfig) (float64, float64) {
	visited := make(map[string]bool)
	queue := []trustNode{{actorID: actorID, hopCount: 0, pathTrust: 1.0}}
	visited[actorID] = true

	propagatedTrust := 0.0
	propagatedWeight := 0.0

	for len(queue) > 0 && queue[0].hopCount < config.maxHops {
		current := queue[0]
		queue = queue[1:]

		// Process current node
		propagatedWeight = r.processNodeTrust(current, trusterScores, config.minTrustScore, propagatedWeight)

		// Add neighbors for next hop if applicable
		if current.hopCount+1 < config.maxHops {
			newNodes, trust := r.expandTrustNetwork(ctx, current, category, visited, config)
			queue = append(queue, newNodes...)
			propagatedTrust += trust
		}
	}

	return propagatedTrust, propagatedWeight
}

// processNodeTrust processes trust contribution from a single node
func (r *TrustRepository) processNodeTrust(node trustNode, trusterScores map[string]float64, minTrustScore float64, currentWeight float64) float64 {
	if trustValue, exists := trusterScores[node.actorID]; exists && trustValue >= minTrustScore {
		return currentWeight + node.pathTrust
	}
	return currentWeight
}

// expandTrustNetwork expands the trust network for propagation
func (r *TrustRepository) expandTrustNetwork(ctx context.Context, current trustNode, category string, visited map[string]bool, config trustPropagationConfig) ([]trustNode, float64) {
	var newNodes []trustNode
	propagatedTrust := 0.0

	// Get the trust score for this node
	nodeScore, err := r.getCachedTrustScore(ctx, current.actorID, category)
	if err != nil {
		r.logger.Debug("failed to get cached trust score for propagation",
			zap.String("actor", current.actorID),
			zap.String("category", category),
			zap.Error(err))
		return newNodes, propagatedTrust
	}
	if nodeScore == nil || nodeScore.Score < config.minTrustScore {
		return newNodes, propagatedTrust
	}

	// Calculate trust for next hop
	nextPathTrust := current.pathTrust * config.propagationRate * nodeScore.Score
	if nextPathTrust < 0.01 { // Only continue if meaningful trust remains
		return newNodes, propagatedTrust
	}

	propagatedTrust = nextPathTrust

	// Get this node's trust relationships
	nodeRelationships, _, err := r.GetTrustedByRelationships(ctx, current.actorID, 50, "")
	if err != nil {
		return newNodes, propagatedTrust
	}

	// Add unvisited nodes to queue
	for _, rel := range nodeRelationships {
		if r.shouldAddToQueue(rel, category, visited) {
			newNodes = append(newNodes, trustNode{
				actorID:   rel.TrusterID,
				hopCount:  current.hopCount + 1,
				pathTrust: nextPathTrust,
			})
			visited[rel.TrusterID] = true
		}
	}

	return newNodes, propagatedTrust
}

// shouldAddToQueue determines if a relationship should be added to propagation queue
func (r *TrustRepository) shouldAddToQueue(rel *storage.TrustRelationship, category string, visited map[string]bool) bool {
	return !visited[rel.TrusterID] &&
		(string(rel.Category) == category || rel.Category == trust.TrustCategoryGeneral)
}

// finalizeTrustScore combines direct and propagated scores and normalizes
func (r *TrustRepository) finalizeTrustScore(score *storage.TrustScore) {
	directWeight := 0.7
	propagatedWeight := 0.3

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
}

// modelToTrustRelationship converts a model to storage type
func (r *TrustRepository) modelToTrustRelationship(model *models.TrustRelationship) *storage.TrustRelationship {
	return &storage.TrustRelationship{
		ID:         model.ID,
		TrusterID:  model.TrusterID,
		TrusteeID:  model.TrusteeID,
		Category:   model.Category,
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
		Category:        model.Category,
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
