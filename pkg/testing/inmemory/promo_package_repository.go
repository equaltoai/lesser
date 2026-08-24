// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// PromoPackageRepository is a thread-safe in-memory implementation of
// interfaces.PromoPackageRepository. It mirrors the production field-scoped
// writers and version-conditioned CAS semantics so service tests exercise the
// same conflict signals the real repository produces.
type PromoPackageRepository struct {
	mu sync.RWMutex

	packages map[string]*models.PromoPackage

	// grants by ownerID:packageID:reviewer
	grants map[string]*models.PromoReviewGrant
	// reviewer queue: reviewer -> []grantKey (composite below)
	grantsByReviewer map[string][]string
	// owner queue: ownerID -> []grantKey
	grantsByOwner map[string][]string

	// verdicts by ownerID:packageID (ordered)
	verdicts map[string][]*models.PromoReviewVerdict
}

// NewPromoPackageRepository creates a new in-memory promo package repository.
func NewPromoPackageRepository() *PromoPackageRepository {
	return &PromoPackageRepository{
		packages:         make(map[string]*models.PromoPackage),
		grants:           make(map[string]*models.PromoReviewGrant),
		grantsByReviewer: make(map[string][]string),
		grantsByOwner:    make(map[string][]string),
		verdicts:         make(map[string][]*models.PromoReviewVerdict),
	}
}

func promoPackageKey(ownerID, packageID string) string {
	return fmt.Sprintf("%s:%s", ownerID, packageID)
}

func promoGrantKey(ownerID, packageID, reviewer string) string {
	return fmt.Sprintf("%s:%s:%s", ownerID, packageID, reviewer)
}

func promoVerdsKey(ownerID, packageID string) string {
	return fmt.Sprintf("%s:%s", ownerID, packageID)
}

// CreatePromoPackage creates a first-time promo package.
func (r *PromoPackageRepository) CreatePromoPackage(_ context.Context, pkg *models.PromoPackage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pkg == nil || strings.TrimSpace(pkg.OwnerID) == "" || strings.TrimSpace(pkg.PackageID) == "" {
		return storage.ErrInvalidInput
	}
	key := promoPackageKey(pkg.OwnerID, pkg.PackageID)
	if _, exists := r.packages[key]; exists {
		return storage.ErrAlreadyExists
	}
	if err := pkg.UpdateKeys(); err != nil {
		return err
	}
	r.packages[key] = pkg
	return nil
}

// GetPromoPackage loads a package by owner and package ID.
func (r *PromoPackageRepository) GetPromoPackage(_ context.Context, ownerID, packageID string) (*models.PromoPackage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	pkg, exists := r.packages[promoPackageKey(strings.TrimSpace(ownerID), strings.TrimSpace(packageID))]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return pkg, nil
}

// UpdatePromoPackageContent is the field-scoped CAS writer for package content.
func (r *PromoPackageRepository) UpdatePromoPackageContent(_ context.Context, ownerID string, pkg *models.PromoPackage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.updatePromoPackageContentLocked(ownerID, pkg)
}

func (r *PromoPackageRepository) updatePromoPackageContentLocked(ownerID string, pkg *models.PromoPackage) error {
	if pkg == nil || strings.TrimSpace(ownerID) == "" || strings.TrimSpace(pkg.OwnerID) != strings.TrimSpace(ownerID) {
		return storage.ErrInvalidInput
	}
	stored, exists := r.packages[promoPackageKey(ownerID, pkg.PackageID)]
	if !exists {
		return storage.ErrNotFound
	}
	if stored.ModelVersion != pkg.ModelVersion {
		return apperrors.DynamoDBConditionalCheckFailed("promo package " + pkg.PackageID).
			WithInternalError(storage.ErrVersionConflict)
	}
	if err := pkg.UpdateKeys(); err != nil {
		return err
	}
	next := *pkg
	next.Assets = append([]models.PromoPackageAsset(nil), pkg.Assets...)
	next.ModelVersion = stored.ModelVersion + 1
	if next.ModelVersion <= 0 {
		next.ModelVersion = 1
	}
	r.packages[promoPackageKey(ownerID, pkg.PackageID)] = &next
	pkg.ModelVersion = next.ModelVersion
	return nil
}

// MarkPromoPackageReleased stamps the outbound Status via a version-conditioned
// field-scoped write.
func (r *PromoPackageRepository) MarkPromoPackageReleased(_ context.Context, ownerID string, pkg *models.PromoPackage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.markPromoPackageReleasedLocked(ownerID, pkg)
}

func (r *PromoPackageRepository) markPromoPackageReleasedLocked(ownerID string, pkg *models.PromoPackage) error {
	if pkg == nil || strings.TrimSpace(ownerID) == "" || strings.TrimSpace(pkg.OwnerID) != strings.TrimSpace(ownerID) {
		return storage.ErrInvalidInput
	}
	key := promoPackageKey(ownerID, pkg.PackageID)
	stored, exists := r.packages[key]
	if !exists {
		return storage.ErrNotFound
	}
	if stored.ModelVersion != pkg.ModelVersion {
		return apperrors.DynamoDBConditionalCheckFailed("promo package " + pkg.PackageID).
			WithInternalError(storage.ErrVersionConflict)
	}
	if err := pkg.UpdateKeys(); err != nil {
		return err
	}
	next := *stored
	next.Status = pkg.Status
	next.ReleasedStatusID = pkg.ReleasedStatusID
	next.ReleasedAt = pkg.ReleasedAt
	next.UpdatedAt = pkg.UpdatedAt
	next.ModelVersion = stored.ModelVersion + 1
	if next.ModelVersion <= 0 {
		next.ModelVersion = 1
	}
	r.packages[key] = &next
	pkg.ModelVersion = next.ModelVersion
	return nil
}

// ListPromoPackages lists one owner's packages, paginated by SK cursors.
func (r *PromoPackageRepository) ListPromoPackages(_ context.Context, ownerID string, limit int, cursor string) ([]*models.PromoPackage, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 {
		limit = 25
	}
	rows := make([]*models.PromoPackage, 0)
	for key, pkg := range r.packages {
		if strings.HasPrefix(key, strings.TrimSpace(ownerID)+":") {
			rows = append(rows, pkg)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].SK < rows[j].SK })
	startIdx := 0
	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		startIdx = len(rows)
		for i, pkg := range rows {
			if pkg.SK > cursor {
				startIdx = i
				break
			}
		}
	}
	endIdx := startIdx + limit
	if endIdx > len(rows) {
		endIdx = len(rows)
	}
	result := rows[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(rows) && len(result) > 0 {
		nextCursor = result[len(result)-1].SK
	}
	return result, nextCursor, nil
}

// CreatePromoReviewGrant creates a first-time review grant.
func (r *PromoPackageRepository) CreatePromoReviewGrant(_ context.Context, grant *models.PromoReviewGrant) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if grant == nil || strings.TrimSpace(grant.OwnerID) == "" || strings.TrimSpace(grant.PackageID) == "" || strings.TrimSpace(grant.Reviewer) == "" {
		return storage.ErrInvalidInput
	}
	key := promoGrantKey(grant.OwnerID, grant.PackageID, grant.Reviewer)
	if _, exists := r.grants[key]; exists {
		return storage.ErrAlreadyExists
	}
	if err := grant.UpdateKeys(); err != nil {
		return err
	}
	r.grants[key] = grant
	r.grantsByReviewer[grant.Reviewer] = append(r.grantsByReviewer[grant.Reviewer], key)
	r.grantsByOwner[grant.OwnerID] = append(r.grantsByOwner[grant.OwnerID], key)
	return nil
}

// RegrantPromoReviewGrant clears revocation and refreshes the queue keys.
func (r *PromoPackageRepository) RegrantPromoReviewGrant(_ context.Context, grant *models.PromoReviewGrant) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if grant == nil || grant.RevokedAt != nil {
		return storage.ErrInvalidInput
	}
	key := promoGrantKey(grant.OwnerID, grant.PackageID, grant.Reviewer)
	stored, exists := r.grants[key]
	if !exists {
		return storage.ErrNotFound
	}
	if stored.Version != grant.Version {
		return apperrors.DynamoDBConditionalCheckFailed("").WithInternalError(storage.ErrVersionConflict)
	}
	next := *grant
	next.Version = stored.Version + 1
	if next.Version <= 0 {
		next.Version = 1
	}
	if err := next.UpdateKeys(); err != nil {
		return err
	}
	r.grants[key] = &next
	grant.Version = next.Version
	if !stringSliceContains(r.grantsByReviewer[grant.Reviewer], key) {
		r.grantsByReviewer[grant.Reviewer] = append(r.grantsByReviewer[grant.Reviewer], key)
	}
	return nil
}

// RevokePromoReviewGrant persists revocation and removes the queue keys.
func (r *PromoPackageRepository) RevokePromoReviewGrant(_ context.Context, grant *models.PromoReviewGrant) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if grant == nil || grant.RevokedAt == nil {
		return storage.ErrInvalidInput
	}
	key := promoGrantKey(grant.OwnerID, grant.PackageID, grant.Reviewer)
	stored, exists := r.grants[key]
	if !exists {
		return storage.ErrNotFound
	}
	if stored.Version != grant.Version {
		return apperrors.DynamoDBConditionalCheckFailed("").WithInternalError(storage.ErrVersionConflict)
	}
	next := *stored
	next.RevokedAt = grant.RevokedAt
	next.Version = stored.Version + 1
	if next.Version <= 0 {
		next.Version = 1
	}
	next.GSI2PK = ""
	next.GSI2SK = ""
	r.grants[key] = &next
	grant.Version = next.Version
	r.grantsByReviewer[grant.Reviewer] = stringSliceRemove(r.grantsByReviewer[grant.Reviewer], key)
	return nil
}

// GetPromoReviewGrant loads one grant.
func (r *PromoPackageRepository) GetPromoReviewGrant(_ context.Context, ownerID, packageID, reviewer string) (*models.PromoReviewGrant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	grant, exists := r.grants[promoGrantKey(strings.TrimSpace(ownerID), strings.TrimSpace(packageID), strings.TrimSpace(reviewer))]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return grant, nil
}

// ListActivePromoReviewGrants pages the sparse reviewer queue.
func (r *PromoPackageRepository) ListActivePromoReviewGrants(_ context.Context, reviewer string, limit int, cursor string) ([]*models.PromoReviewGrant, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 {
		limit = 25
	}
	keys := r.grantsByReviewer[strings.TrimSpace(reviewer)]
	rows := make([]*models.PromoReviewGrant, 0, len(keys))
	for _, key := range keys {
		if grant, exists := r.grants[key]; exists && grant.RevokedAt == nil {
			rows = append(rows, grant)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].GSI2SK > rows[j].GSI2SK })
	startIdx := 0
	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		startIdx = len(rows)
		for i, grant := range rows {
			if grant.GSI2SK < cursor {
				startIdx = i
				break
			}
		}
	}
	endIdx := startIdx + limit
	if endIdx > len(rows) {
		endIdx = len(rows)
	}
	result := rows[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(rows) && len(result) > 0 {
		nextCursor = result[len(result)-1].GSI2SK
	}
	return result, nextCursor, nil
}

// ListPromoReviewGrants returns all grants for one package.
func (r *PromoPackageRepository) ListPromoReviewGrants(_ context.Context, ownerID, packageID string) ([]*models.PromoReviewGrant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rows := make([]*models.PromoReviewGrant, 0)
	prefix := promoGrantKey(strings.TrimSpace(ownerID), strings.TrimSpace(packageID), "")
	for key, grant := range r.grants {
		if strings.HasPrefix(key, prefix) {
			rows = append(rows, grant)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].SK < rows[j].SK })
	return rows, nil
}

// ListPromoReviewGrantsByOwner returns every grant created by one owner.
func (r *PromoPackageRepository) ListPromoReviewGrantsByOwner(_ context.Context, ownerID string) ([]*models.PromoReviewGrant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := r.grantsByOwner[strings.TrimSpace(ownerID)]
	rows := make([]*models.PromoReviewGrant, 0, len(keys))
	for _, key := range keys {
		if grant, exists := r.grants[key]; exists {
			rows = append(rows, grant)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].SK < rows[j].SK })
	return rows, nil
}

// CreatePromoReviewVerdict records an immutable verdict.
func (r *PromoPackageRepository) CreatePromoReviewVerdict(_ context.Context, verdict *models.PromoReviewVerdict) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if verdict == nil || strings.TrimSpace(verdict.OwnerID) == "" || strings.TrimSpace(verdict.PackageID) == "" || strings.TrimSpace(verdict.Reviewer) == "" || strings.TrimSpace(verdict.Verdict) == "" {
		return storage.ErrInvalidInput
	}
	if err := verdict.UpdateKeys(); err != nil {
		return err
	}
	key := promoVerdsKey(verdict.OwnerID, verdict.PackageID)
	r.verdicts[key] = append(r.verdicts[key], verdict)
	return nil
}

// ListPromoReviewVerdicts returns ordered verdict history for one package.
func (r *PromoPackageRepository) ListPromoReviewVerdicts(_ context.Context, ownerID, packageID string) ([]*models.PromoReviewVerdict, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rows := append([]*models.PromoReviewVerdict(nil), r.verdicts[promoVerdsKey(strings.TrimSpace(ownerID), strings.TrimSpace(packageID))]...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].SK < rows[j].SK })
	return rows, nil
}

func stringSliceContains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

func stringSliceRemove(slice []string, value string) []string {
	out := make([]string, 0, len(slice))
	for _, v := range slice {
		if v != value {
			out = append(out, v)
		}
	}
	return out
}

var _ interfaces.PromoPackageRepository = (*PromoPackageRepository)(nil)
