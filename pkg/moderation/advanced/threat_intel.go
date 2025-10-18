package advanced

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// ThreatIntelRepository interface for dependency injection
type ThreatIntelRepository interface {
	ShareThreat(ctx context.Context, threat *repositories.ThreatIntel) error
	GetSharedThreats(ctx context.Context, since time.Time) ([]*repositories.ThreatIntel, error)
	GetThreatsByType(ctx context.Context, threatType string, limit int) ([]*repositories.ThreatIntel, error)
	UpdateThreatConfidence(ctx context.Context, threatID string, newConfidence float64) error
	IncrementHitCount(ctx context.Context, threatID string) error
	LoadActiveThreats(ctx context.Context) ([]*repositories.ThreatIntel, error)
	GetThreatByID(ctx context.Context, threatID string) (*repositories.ThreatIntel, error)
	GetIndicatorThreat(ctx context.Context, indicator string) (string, error)
}

// ThreatIntelligence manages cross-instance threat sharing
type ThreatIntelligence struct {
	repo   ThreatIntelRepository
	logger *zap.Logger

	// Cache for active threats
	threatCache sync.Map
	lastUpdate  time.Time
	updateMutex sync.RWMutex
}

// NewThreatIntelligence creates a new threat intelligence component
func NewThreatIntelligence(repo ThreatIntelRepository, logger *zap.Logger) *ThreatIntelligence {
	ti := &ThreatIntelligence{
		repo:   repo,
		logger: logger,
	}

	// Load threats on initialization
	ctx := context.Background()
	if err := ti.loadThreats(ctx); err != nil {
		logger.Warn("failed to load threats on init", zap.Error(err))
	}

	return ti
}

// ShareThreat shares a new threat with the network
func (ti *ThreatIntelligence) ShareThreat(ctx context.Context, threat *ThreatIntel) error {
	// Validate threat
	if err := ti.validateThreat(threat); err != nil {
		return fmt.Errorf("invalid threat: %w", err)
	}

	// Generate ID if not provided
	if err := common.ValidateRequiredParam("threat.ID", threat.ID); err != nil {
		threat.ID = ti.generateThreatID(threat)
	}

	// Set timestamps
	now := time.Now()
	if threat.FirstSeen.IsZero() {
		threat.FirstSeen = now
	}
	threat.LastSeen = now

	// Convert to repository threat
	repoThreat := &repositories.ThreatIntel{
		ID:           threat.ID,
		ThreatType:   threat.ThreatType,
		Indicators:   threat.Indicators,
		Severity:     string(threat.Severity),
		Description:  threat.Description,
		SourceDomain: threat.SourceDomain,
		FirstSeen:    threat.FirstSeen,
		LastSeen:     threat.LastSeen,
		HitCount:     threat.HitCount,
		Confidence:   threat.Confidence,
		TTL:          threat.TTL,
	}

	// Store in repository
	if err := ti.repo.ShareThreat(ctx, repoThreat); err != nil {
		return fmt.Errorf("share threat: %w", err)
	}

	// Update cache
	ti.threatCache.Store(threat.ID, threat)

	ti.logger.Info("shared threat",
		zap.String("threatID", threat.ID),
		zap.String("type", threat.ThreatType),
		zap.String("severity", string(threat.Severity)),
		zap.Int("indicators", len(threat.Indicators)))

	return nil
}

// GetSharedThreats retrieves threats shared since a given time
func (ti *ThreatIntelligence) GetSharedThreats(ctx context.Context, since time.Time) ([]*ThreatIntel, error) {
	repoThreats, err := ti.repo.GetSharedThreats(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("query threats: %w", err)
	}

	threats := make([]*ThreatIntel, 0, len(repoThreats))
	for _, repoThreat := range repoThreats {
		threat := &ThreatIntel{
			ID:           repoThreat.ID,
			ThreatType:   repoThreat.ThreatType,
			Indicators:   repoThreat.Indicators,
			Severity:     Severity(repoThreat.Severity),
			Description:  repoThreat.Description,
			SourceDomain: repoThreat.SourceDomain,
			FirstSeen:    repoThreat.FirstSeen,
			LastSeen:     repoThreat.LastSeen,
			HitCount:     repoThreat.HitCount,
			Confidence:   repoThreat.Confidence,
			TTL:          repoThreat.TTL,
		}
		threats = append(threats, threat)
	}

	return threats, nil
}

// CheckContent checks content against known threats
func (ti *ThreatIntelligence) CheckContent(_ context.Context, content string, metadata ContentMetadata) ([]ThreatMatch, error) {
	matches := []ThreatMatch{}
	lowerContent := strings.ToLower(content)

	// Check against cached threats
	ti.threatCache.Range(func(_, value any) bool {
		threat, ok := value.(*ThreatIntel)
		if !ok {
			return true
		}

		// Check each indicator
		for _, indicator := range threat.Indicators {
			if ti.matchesIndicator(indicator, content, lowerContent, metadata) {
				matches = append(matches, ThreatMatch{
					ThreatID:   threat.ID,
					ThreatType: threat.ThreatType,
					Indicator:  indicator,
					Confidence: threat.Confidence,
				})

				// Increment hit count asynchronously
				go ti.incrementHitCount(threat.ID)
			}
		}

		return true
	})

	// Check URLs against threat domains
	for _, url := range metadata.URLs {
		if threatID := ti.checkURLThreat(url); threatID != "" {
			matches = append(matches, ThreatMatch{
				ThreatID:   threatID,
				ThreatType: "malicious_url",
				Indicator:  url,
				Confidence: 0.9,
			})
		}
	}

	// Check for hash matches (for images/files)
	if metadata.ContentType == ContentTypeImage {
		hash := ti.hashContent(content)
		if threatID := ti.checkHashThreat(hash); threatID != "" {
			matches = append(matches, ThreatMatch{
				ThreatID:   threatID,
				ThreatType: "malicious_content",
				Indicator:  hash,
				Confidence: 1.0,
			})
		}
	}

	return matches, nil
}

// GetThreatsByType retrieves threats of a specific type
func (ti *ThreatIntelligence) GetThreatsByType(ctx context.Context, threatType string, limit int) ([]*ThreatIntel, error) {
	repoThreats, err := ti.repo.GetThreatsByType(ctx, threatType, limit)
	if err != nil {
		return nil, fmt.Errorf("query threats by type: %w", err)
	}

	threats := make([]*ThreatIntel, 0, len(repoThreats))
	for _, repoThreat := range repoThreats {
		threat := &ThreatIntel{
			ID:           repoThreat.ID,
			ThreatType:   repoThreat.ThreatType,
			Indicators:   repoThreat.Indicators,
			Severity:     Severity(repoThreat.Severity),
			Description:  repoThreat.Description,
			SourceDomain: repoThreat.SourceDomain,
			FirstSeen:    repoThreat.FirstSeen,
			LastSeen:     repoThreat.LastSeen,
			HitCount:     repoThreat.HitCount,
			Confidence:   repoThreat.Confidence,
			TTL:          repoThreat.TTL,
		}
		threats = append(threats, threat)
	}

	return threats, nil
}

// UpdateThreatConfidence updates the confidence score of a threat
func (ti *ThreatIntelligence) UpdateThreatConfidence(ctx context.Context, threatID string, newConfidence float64) error {
	return ti.repo.UpdateThreatConfidence(ctx, threatID, newConfidence)
}

// Helper methods

func (ti *ThreatIntelligence) validateThreat(threat *ThreatIntel) error {
	if err := common.ValidateRequiredParam("threat.ThreatType", threat.ThreatType); err != nil {
		return fmt.Errorf("threat type required")
	}

	if err := common.ValidateSliceNotEmpty("threat.Indicators", threat.Indicators); err != nil {
		return fmt.Errorf("at least one indicator required")
	}

	if err := common.ValidateRequiredParam("threat.Severity", string(threat.Severity)); err != nil {
		threat.Severity = SeverityMedium
	}

	if threat.Confidence == 0 {
		threat.Confidence = 0.7
	}

	if threat.TTL == 0 {
		threat.TTL = 7 * 24 * time.Hour // Default 7 days
	}

	return nil
}

func (ti *ThreatIntelligence) generateThreatID(threat *ThreatIntel) string {
	// Generate ID based on threat type and indicators
	h := sha256.New()
	h.Write([]byte(threat.ThreatType))
	for _, indicator := range threat.Indicators {
		h.Write([]byte(indicator))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func (ti *ThreatIntelligence) matchesIndicator(indicator, _, lowerContent string, metadata ContentMetadata) bool {
	// Simple string matching - in production, use more sophisticated matching
	indicatorLower := strings.ToLower(indicator)

	// Check content
	if strings.Contains(lowerContent, indicatorLower) {
		return true
	}

	// Check URLs
	for _, url := range metadata.URLs {
		if strings.Contains(strings.ToLower(url), indicatorLower) {
			return true
		}
	}

	// Check hashtags
	for _, tag := range metadata.Hashtags {
		if strings.EqualFold(tag, indicator) {
			return true
		}
	}

	return false
}

func (ti *ThreatIntelligence) checkURLThreat(url string) string {
	// Check if URL matches known malicious domains
	// This is simplified - in production, use proper URL parsing and domain extraction
	maliciousDomains := []string{
		"malicious.com",
		"phishing-site.net",
		"scam-domain.org",
	}

	urlLower := strings.ToLower(url)
	for _, domain := range maliciousDomains {
		if strings.Contains(urlLower, domain) {
			return "known-malicious-domain"
		}
	}

	return ""
}

func (ti *ThreatIntelligence) checkHashThreat(hash string) string {
	// Check if hash matches known malicious content using repository
	ctx := context.Background()
	threatID, err := ti.repo.GetIndicatorThreat(ctx, hash)
	if err != nil {
		ti.logger.Warn("failed to check hash threat",
			zap.String("hash", hash),
			zap.Error(err))
		return ""
	}
	return threatID
}

func (ti *ThreatIntelligence) hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

func (ti *ThreatIntelligence) incrementHitCount(threatID string) {
	ctx := context.Background()

	if err := ti.repo.IncrementHitCount(ctx, threatID); err != nil {
		ti.logger.Warn("failed to increment hit count",
			zap.String("threatID", threatID),
			zap.Error(err))
	}
}

func (ti *ThreatIntelligence) loadThreats(ctx context.Context) error {
	ti.updateMutex.Lock()
	defer ti.updateMutex.Unlock()

	// Clear existing threats
	ti.threatCache.Range(func(key, _ any) bool {
		ti.threatCache.Delete(key)
		return true
	})

	// Load active threats from repository
	repoThreats, err := ti.repo.LoadActiveThreats(ctx)
	if err != nil {
		return fmt.Errorf("load active threats: %w", err)
	}

	for _, repoThreat := range repoThreats {
		threat := &ThreatIntel{
			ID:           repoThreat.ID,
			ThreatType:   repoThreat.ThreatType,
			Indicators:   repoThreat.Indicators,
			Severity:     Severity(repoThreat.Severity),
			Description:  repoThreat.Description,
			SourceDomain: repoThreat.SourceDomain,
			FirstSeen:    repoThreat.FirstSeen,
			LastSeen:     repoThreat.LastSeen,
			HitCount:     repoThreat.HitCount,
			Confidence:   repoThreat.Confidence,
			TTL:          repoThreat.TTL,
		}

		ti.threatCache.Store(threat.ID, threat)
	}

	ti.lastUpdate = time.Now()
	ti.logger.Info("loaded threats", zap.Int("count", len(repoThreats)))

	return nil
}

// RefreshThreats loads the latest threats from the database into the cache.
// This method should be called on-demand, typically triggered by EventBridge
// scheduled events in a serverless environment.
//
// Example usage in a Lambda function:
//
//	func handler(ctx context.Context, event events.CloudWatchEvent) error {
//	    return threatIntel.RefreshThreats(ctx)
//	}
func (ti *ThreatIntelligence) RefreshThreats(ctx context.Context) error {
	if err := ti.loadThreats(ctx); err != nil {
		ti.logger.Error("failed to refresh threats", zap.Error(err))
		return fmt.Errorf("refresh threats: %w", err)
	}

	ti.logger.Info("successfully refreshed threat intelligence cache")
	return nil
}
