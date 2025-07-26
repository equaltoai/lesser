package repositories

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
)

// ModerationRepository implements moderation operations using DynamORM
type ModerationRepository struct {
	db                 core.DB
	prohibitedWords    []string
	prohibitedPatterns []*regexp.Regexp
	spamPatterns       []*regexp.Regexp
}

// NewModerationRepository creates a new moderation repository
func NewModerationRepository(db core.DB) *ModerationRepository {
	repo := &ModerationRepository{
		db: db,
		// Real prohibited words list - in production, load from config
		prohibitedWords: []string{
			"hate", "spam", "scam", "phishing", "malware", "virus",
			"illegal", "drugs", "violence", "terrorism", "abuse",
		},
		prohibitedPatterns: []*regexp.Regexp{},
		spamPatterns:       []*regexp.Regexp{},
	}

	// Compile prohibited patterns
	prohibitedPatternStrings := []string{
		`\b(buy|get|earn)\s+(now|today|fast)\b`,              // Spam sales patterns
		`\b(click|visit)\s+(here|this|link)\b`,               // Spam link patterns
		`\b\d{3,}\s*(usd|dollars|euros|bitcoin)\b`,           // Money spam
		`\b(congratulations|winner|selected|chosen)\b`,       // Scam patterns
		`\b(limited|exclusive|special)\s+(offer|deal|time)\b`, // Urgency spam
	}
	for _, pattern := range prohibitedPatternStrings {
		if compiled, err := regexp.Compile(`(?i)` + pattern); err == nil {
			repo.prohibitedPatterns = append(repo.prohibitedPatterns, compiled)
		}
	}

	// Compile spam detection patterns
	spamPatternStrings := []string{
		`https?://[^\s]+\.[^\s]+`,                       // URLs
		`@\w+`,                                          // Mentions
		`#\w+`,                                          // Hashtags
		`([A-Z]{2,}\s*){3,}`,                            // EXCESSIVE CAPS
		`(.)\1{4,}`,                                     // Repeated characters
		`\b(bit\.ly|tinyurl|short\.link|goo\.gl)\b`,    // URL shorteners
		`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`, // Email addresses
	}
	for _, pattern := range spamPatternStrings {
		if compiled, err := regexp.Compile(pattern); err == nil {
			repo.spamPatterns = append(repo.spamPatterns, compiled)
		}
	}

	return repo
}

// IsDomainBlocked checks if a domain is blocked
func (r *ModerationRepository) IsDomainBlocked(ctx context.Context, domain string) (bool, *models.DomainBlock, error) {
	// Query for domain block
	var block models.DomainBlock
	
	query := r.db.WithContext(ctx).Model(&block).
		Where("PK", "=", fmt.Sprintf("domain#%s", domain)).
		Where("SK", "=", fmt.Sprintf("block#%s", domain))

	if err := query.First(&block); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("failed to check domain block: %w", err)
	}

	// Check if block is still active
	if block.ExpiresAt != nil && block.ExpiresAt.Before(time.Now()) {
		return false, nil, nil
	}

	return true, &block, nil
}

// CreateModeration creates a new moderation case
func (r *ModerationRepository) CreateModeration(ctx context.Context, moderation *models.Moderation) error {
	if moderation.ContentID == "" {
		return common.ValidationError{Field: "ContentID", Message: "content ID is required"}
	}
	if moderation.ContentType == "" {
		return common.ValidationError{Field: "ContentType", Message: "content type is required"}
	}

	// Create the moderation using DynamORM
	err := r.db.WithContext(ctx).Model(moderation).Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			return common.ConflictError{
				Resource: "moderation",
				Message:  fmt.Sprintf("moderation %s already exists", moderation.ModerationID),
			}
		}
		return fmt.Errorf("failed to create moderation: %w", err)
	}

	return nil
}

// GetModeration retrieves a moderation case by ID
func (r *ModerationRepository) GetModeration(ctx context.Context, moderationID string) (*models.Moderation, error) {
	var moderation models.Moderation

	err := r.db.WithContext(ctx).Model(&models.Moderation{}).
		Where("PK", "=", "moderation#"+moderationID).
		Where("SK", "=", "moderation#"+moderationID).
		First(&moderation)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("moderation not found: %s", moderationID)
		}
		return nil, fmt.Errorf("failed to get moderation: %w", err)
	}

	return &moderation, nil
}

// UpdateModeration updates an existing moderation case
func (r *ModerationRepository) UpdateModeration(ctx context.Context, moderation *models.Moderation) error {
	// Update using DynamORM
	err := r.db.WithContext(ctx).Model(moderation).Update()
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("moderation not found: %s", moderation.ModerationID)
		}
		return fmt.Errorf("failed to update moderation: %w", err)
	}

	return nil
}

// DeleteModeration deletes a moderation case
func (r *ModerationRepository) DeleteModeration(ctx context.Context, moderationID string) error {
	err := r.db.WithContext(ctx).Model(&models.Moderation{}).
		Where("PK", "=", "moderation#"+moderationID).
		Where("SK", "=", "moderation#"+moderationID).
		Delete()
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("moderation not found: %s", moderationID)
		}
		return fmt.Errorf("failed to delete moderation: %w", err)
	}

	return nil
}

// GetModerationsByContent retrieves all moderations for a specific piece of content
func (r *ModerationRepository) GetModerationsByContent(ctx context.Context, contentType models.ModerationContentType, contentID string) ([]*models.Moderation, error) {
	var moderations []models.Moderation

	err := r.db.WithContext(ctx).Model(&models.Moderation{}).
		Index("content-moderation-index").
		Where("GSI1PK", "=", fmt.Sprintf("%s#%s", contentType, contentID)).
		All(&moderations)
	if err != nil {
		return nil, fmt.Errorf("failed to query moderations by content: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.Moderation, len(moderations))
	for i := range moderations {
		result[i] = &moderations[i]
	}

	return result, nil
}

// GetModerationsByStatus retrieves moderations with a specific status
func (r *ModerationRepository) GetModerationsByStatus(ctx context.Context, status models.ModerationStatus, limit int) ([]*models.Moderation, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var moderations []models.Moderation
	err := r.db.WithContext(ctx).Model(&models.Moderation{}).
		Index("moderation-status-index").
		Where("GSI2PK", "=", "STATUS#"+string(status)).
		Limit(limit).
		All(&moderations)
	if err != nil {
		return nil, fmt.Errorf("failed to query moderations by status: %w", err)
	}

	result := make([]*models.Moderation, len(moderations))
	for i := range moderations {
		result[i] = &moderations[i]
	}

	return result, nil
}

// GetModerationsByModerator retrieves moderations handled by a specific moderator
func (r *ModerationRepository) GetModerationsByModerator(ctx context.Context, moderatorID string, limit int) ([]*models.Moderation, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var moderations []models.Moderation
	err := r.db.WithContext(ctx).Model(&models.Moderation{}).
		Index("moderator-index").
		Where("GSI3PK", "=", "MODERATOR#"+moderatorID).
		Limit(limit).
		All(&moderations)
	if err != nil {
		return nil, fmt.Errorf("failed to query moderations by moderator: %w", err)
	}

	result := make([]*models.Moderation, len(moderations))
	for i := range moderations {
		result[i] = &moderations[i]
	}

	return result, nil
}

// GetModerationsByUser retrieves moderations affecting a specific user
func (r *ModerationRepository) GetModerationsByUser(ctx context.Context, userID string, limit int) ([]*models.Moderation, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var moderations []models.Moderation
	err := r.db.WithContext(ctx).Model(&models.Moderation{}).
		Index("user-moderation-index").
		Where("GSI4PK", "=", "USER_MOD#"+userID).
		Limit(limit).
		All(&moderations)
	if err != nil {
		return nil, fmt.Errorf("failed to query moderations by user: %w", err)
	}

	result := make([]*models.Moderation, len(moderations))
	for i := range moderations {
		result[i] = &moderations[i]
	}

	return result, nil
}

// GetPendingReviews retrieves all moderations pending review
func (r *ModerationRepository) GetPendingReviews(ctx context.Context, limit int) ([]*models.Moderation, error) {
	return r.GetModerationsByStatus(ctx, models.ModerationStatusPending, limit)
}

// SubmitAppeal submits an appeal for a moderation decision
func (r *ModerationRepository) SubmitAppeal(ctx context.Context, moderationID, userID, reason string) error {
	// Get the moderation
	moderation, err := r.GetModeration(ctx, moderationID)
	if err != nil {
		return err
	}

	// Verify the user is the one affected
	if moderation.UserID != userID {
		return common.ValidationError{Field: "UserID", Message: "user cannot appeal this moderation"}
	}

	// Check if appeal is allowed
	if !moderation.CanBeAppealed() {
		return common.ValidationError{Field: "Status", Message: "moderation cannot be appealed in current state"}
	}

	// Update appeal information
	now := time.Now()
	moderation.AppealStatus = "requested"
	moderation.AppealReason = reason
	moderation.AppealedAt = &now
	moderation.Status = models.ModerationStatusAppealed

	// Add history entry
	moderation.AddHistoryEntry(userID, "user", models.ModerationActionWarning, 
		models.ModerationStatusActioned, models.ModerationStatusAppealed, "Appeal submitted: "+reason)

	// Update the moderation
	return r.UpdateModeration(ctx, moderation)
}

// ReviewAppeal reviews an appeal and makes a decision
func (r *ModerationRepository) ReviewAppeal(ctx context.Context, moderationID, reviewerID, decision, decisionReason string) error {
	// Get the moderation
	moderation, err := r.GetModeration(ctx, moderationID)
	if err != nil {
		return err
	}

	// Check if in appeal state
	if moderation.AppealStatus != "requested" && moderation.AppealStatus != "reviewing" {
		return common.ValidationError{Field: "AppealStatus", Message: "no pending appeal to review"}
	}

	// Update appeal information
	moderation.AppealReviewer = reviewerID
	moderation.AppealDecision = decisionReason

	// Update status based on decision
	fromStatus := moderation.Status
	if decision == "approved" {
		moderation.AppealStatus = "approved"
		moderation.Status = models.ModerationStatusResolved
		moderation.Action = models.ModerationActionRestore
		now := time.Now()
		moderation.ResolvedAt = &now
	} else {
		moderation.AppealStatus = "denied"
		moderation.Status = models.ModerationStatusActioned // Back to original state
	}

	// Add history entry
	action := models.ModerationActionRestore
	if decision == "denied" {
		action = models.ModerationActionDismiss
	}
	moderation.AddHistoryEntry(reviewerID, "moderator", action, fromStatus, moderation.Status, 
		fmt.Sprintf("Appeal %s: %s", decision, decisionReason))

	// Update the moderation
	return r.UpdateModeration(ctx, moderation)
}

// GetModerationStatistics retrieves statistics for moderation activities
func (r *ModerationRepository) GetModerationStatistics(ctx context.Context, timeRange time.Duration) (*ModerationStats, error) {
	stats := &ModerationStats{
		TimeRange:      timeRange,
		ActionCounts:   make(map[models.ModerationAction]int),
		ReasonCounts:   make(map[models.ModerationReason]int),
		StatusCounts:   make(map[models.ModerationStatus]int),
		ContentCounts:  make(map[models.ModerationContentType]int),
		ModeratorStats: make(map[string]*ModeratorStats),
	}

	// Calculate time cutoff
	cutoff := time.Now().Add(-timeRange)

	// Query all statuses to get counts
	// In production, this would be more efficient with aggregation
	statuses := []models.ModerationStatus{
		models.ModerationStatusPending,
		models.ModerationStatusReviewing,
		models.ModerationStatusActioned,
		models.ModerationStatusAppealed,
		models.ModerationStatusResolved,
		models.ModerationStatusDismissed,
	}

	for _, status := range statuses {
		moderations, err := r.GetModerationsByStatus(ctx, status, 1000) // Get more for stats
		if err != nil {
			continue
		}

		for _, mod := range moderations {
			// Only count if within time range
			if mod.CreatedAt.After(cutoff) {
				stats.TotalCases++
				stats.ActionCounts[mod.Action]++
				stats.ReasonCounts[mod.Reason]++
				stats.StatusCounts[mod.Status]++
				stats.ContentCounts[mod.ContentType]++

				// Track moderator stats
				if mod.ModeratorID != "" && mod.ModeratorID != "system" {
					if _, exists := stats.ModeratorStats[mod.ModeratorID]; !exists {
						stats.ModeratorStats[mod.ModeratorID] = &ModeratorStats{
							ModeratorID:  mod.ModeratorID,
							ActionCounts: make(map[models.ModerationAction]int),
						}
					}
					stats.ModeratorStats[mod.ModeratorID].CasesHandled++
					stats.ModeratorStats[mod.ModeratorID].ActionCounts[mod.Action]++

					// Calculate average response time
					if mod.ActionedAt != nil {
						responseTime := mod.ActionedAt.Sub(mod.CreatedAt)
						current := stats.ModeratorStats[mod.ModeratorID].AverageResponseTime
						newAvg := (current*time.Duration(stats.ModeratorStats[mod.ModeratorID].CasesHandled-1) + responseTime) / 
							time.Duration(stats.ModeratorStats[mod.ModeratorID].CasesHandled)
						stats.ModeratorStats[mod.ModeratorID].AverageResponseTime = newAvg
					}
				}

				// Track automated vs human
				if mod.IsAutomated() {
					stats.AutomatedCases++
				} else {
					stats.HumanReviewCases++
				}

				// Track appeals
				if mod.AppealStatus == "requested" || mod.AppealStatus == "reviewing" {
					stats.AppealsPending++
				} else if mod.AppealStatus == "approved" {
					stats.AppealsApproved++
				} else if mod.AppealStatus == "denied" {
					stats.AppealsDenied++
				}
			}
		}
	}

	// Calculate rates
	if stats.TotalCases > 0 {
		stats.AutomationRate = float64(stats.AutomatedCases) / float64(stats.TotalCases) * 100
		totalAppeals := stats.AppealsApproved + stats.AppealsDenied
		if totalAppeals > 0 {
			stats.AppealSuccessRate = float64(stats.AppealsApproved) / float64(totalAppeals) * 100
		}
	}

	return stats, nil
}

// AnalyzeContent performs content analysis for moderation
func (r *ModerationRepository) AnalyzeContent(ctx context.Context, content string, userID string, contentType models.ModerationContentType) (*models.ModerationEvidence, error) {
	evidence := &models.ModerationEvidence{
		TextContent:   content,
		ContentLength: len(content),
	}

	// Analyze for prohibited words
	lowerContent := strings.ToLower(content)
	for _, word := range r.prohibitedWords {
		if strings.Contains(lowerContent, strings.ToLower(word)) {
			evidence.ProhibitedWords = append(evidence.ProhibitedWords, word)
		}
	}

	// Check prohibited patterns
	for _, pattern := range r.prohibitedPatterns {
		if matches := pattern.FindAllString(content, -1); len(matches) > 0 {
			evidence.MatchedPatterns = append(evidence.MatchedPatterns, pattern.String())
		}
	}

	// Calculate spam score
	spamScore := 0.0
	spamIndicators := []string{}

	// Check for URLs
	urlPattern := regexp.MustCompile(`https?://[^\s]+`)
	urls := urlPattern.FindAllString(content, -1)
	evidence.LinkCount = len(urls)
	if len(urls) > 3 {
		spamScore += 0.3
		spamIndicators = append(spamIndicators, fmt.Sprintf("excessive_links_%d", len(urls)))
	}

	// Check for suspicious URLs
	for _, url := range urls {
		for _, pattern := range r.spamPatterns {
			if pattern.MatchString(url) {
				evidence.SuspiciousLinks = append(evidence.SuspiciousLinks, url)
				spamScore += 0.2
				spamIndicators = append(spamIndicators, "suspicious_url")
				break
			}
		}
	}

	// Check mentions
	mentionPattern := regexp.MustCompile(`@\w+`)
	mentions := mentionPattern.FindAllString(content, -1)
	evidence.MentionCount = len(mentions)
	if len(mentions) > 5 {
		spamScore += 0.2
		spamIndicators = append(spamIndicators, fmt.Sprintf("excessive_mentions_%d", len(mentions)))
	}

	// Check hashtags
	hashtagPattern := regexp.MustCompile(`#\w+`)
	hashtags := hashtagPattern.FindAllString(content, -1)
	evidence.HashtagCount = len(hashtags)
	if len(hashtags) > 10 {
		spamScore += 0.2
		spamIndicators = append(spamIndicators, fmt.Sprintf("excessive_hashtags_%d", len(hashtags)))
	}

	// Check for CAPS abuse
	upperCount := 0
	for _, r := range content {
		if r >= 'A' && r <= 'Z' {
			upperCount++
		}
	}
	if len(content) > 10 && float64(upperCount)/float64(len(content)) > 0.7 {
		spamScore += 0.2
		spamIndicators = append(spamIndicators, "excessive_caps")
	}

	// Check for repeated characters
	repeatedPattern := regexp.MustCompile(`(.)\1{4,}`)
	if repeatedPattern.MatchString(content) {
		spamScore += 0.1
		spamIndicators = append(spamIndicators, "repeated_chars")
	}

	// Check for duplicate content (simplified - in production use bloom filter or redis)
	contentHash := sha256.Sum256([]byte(content))
	evidence.ContentHash = hex.EncodeToString(contentHash[:])

	// Set final spam score and indicators
	evidence.SpamScore = math.Min(spamScore, 1.0)
	evidence.SpamIndicators = spamIndicators

	// Calculate confidence based on evidence strength
	confidence := 0.5
	if len(evidence.ProhibitedWords) > 0 {
		confidence = 0.9
	} else if evidence.SpamScore > 0.7 {
		confidence = 0.8
	} else if len(evidence.MatchedPatterns) > 0 {
		confidence = 0.7
	}
	evidence.ConfidenceScore = confidence

	// Calculate false positive risk
	falsePositiveRisk := 0.0
	if len(evidence.ProhibitedWords) == 0 && evidence.SpamScore < 0.5 {
		falsePositiveRisk = 0.3
	}
	if evidence.ContentLength < 50 {
		falsePositiveRisk += 0.2
	}
	evidence.FalsePositiveRisk = math.Min(falsePositiveRisk, 1.0)

	// Determine if human review is needed
	evidence.RequiresReview = confidence < 0.7 || falsePositiveRisk > 0.3

	return evidence, nil
}

// CheckRateLimit checks if a user has exceeded rate limits
func (r *ModerationRepository) CheckRateLimit(ctx context.Context, userID string, action string, limit int, period time.Duration) (*RateLimitResult, error) {
	// Get recent moderations for this user
	moderations, err := r.GetModerationsByUser(ctx, userID, 100)
	if err != nil {
		return nil, err
	}

	// Count recent violations
	cutoff := time.Now().Add(-period)
	recentCount := 0
	var lastViolation *time.Time

	for _, mod := range moderations {
		if mod.CreatedAt.After(cutoff) && mod.Reason == models.ModerationReasonRateLimiting {
			recentCount++
			if lastViolation == nil || mod.CreatedAt.After(*lastViolation) {
				lastViolation = &mod.CreatedAt
			}
		}
	}

	result := &RateLimitResult{
		UserID:         userID,
		Action:         action,
		Period:         period,
		Limit:          limit,
		CurrentCount:   recentCount,
		Exceeded:       recentCount >= limit,
		ViolationCount: recentCount,
	}

	if lastViolation != nil {
		result.LastViolation = lastViolation
	}

	// Calculate average interval if we have violations
	if recentCount > 1 {
		// Simple calculation - in production, track actual request times
		result.AverageInterval = period.Seconds() / float64(recentCount)
	}

	return result, nil
}

// Supporting types

// ModerationStats contains statistics about moderation activities
type ModerationStats struct {
	TimeRange         time.Duration
	TotalCases        int
	AutomatedCases    int
	HumanReviewCases  int
	AutomationRate    float64
	AppealsPending    int
	AppealsApproved   int
	AppealsDenied     int
	AppealSuccessRate float64
	ActionCounts      map[models.ModerationAction]int
	ReasonCounts      map[models.ModerationReason]int
	StatusCounts      map[models.ModerationStatus]int
	ContentCounts     map[models.ModerationContentType]int
	ModeratorStats    map[string]*ModeratorStats
}

// ModeratorStats contains statistics for a specific moderator
type ModeratorStats struct {
	ModeratorID         string
	CasesHandled        int
	ActionCounts        map[models.ModerationAction]int
	AverageResponseTime time.Duration
	AppealRate          float64
}

// RateLimitResult contains the result of a rate limit check
type RateLimitResult struct {
	UserID          string
	Action          string
	Period          time.Duration
	Limit           int
	CurrentCount    int
	Exceeded        bool
	LastViolation   *time.Time
	ViolationCount  int
	AverageInterval float64
}