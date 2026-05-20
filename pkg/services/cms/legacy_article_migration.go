package cms

import (
	neturl "net/url"
	"sort"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// Legacy Article migration skip and conflict reason codes.
const (
	LegacyArticleSkipNil               = "nil_article"
	LegacyArticleSkipNotArticle        = "not_article"
	LegacyArticleSkipMissingID         = "missing_id"
	LegacyArticleSkipAlreadyCanonical  = "already_canonical_article"
	LegacyArticleSkipNotLegacyObjectID = "not_legacy_objects_id"
	LegacyArticleSkipMissingSlug       = "missing_slug"
	LegacyArticleConflictDuplicate     = "duplicate_legacy_alias"
	LegacyArticleConflictOccupied      = "canonical_alias_occupied"
)

// LegacyArticleMigrationPlan is the dry-run output for deciding how legacy
// Article IDs under /objects/<uuid> could be exposed through non-authoritative
// /articles/<slug> aliases without rewriting ActivityPub object identity.
type LegacyArticleMigrationPlan struct {
	Candidates []LegacyArticleMigrationCandidate
	Conflicts  []LegacyArticleMigrationConflict
	Skipped    []LegacyArticleMigrationSkipped
}

// LegacyArticleMigrationCandidate maps one legacy Article to its proposed
// browser alias. ProposedCanonicalID intentionally remains the stored legacy
// Article ID for the MVP so the dry-run never creates a second ActivityPub
// object identity for existing content.
type LegacyArticleMigrationCandidate struct {
	ArticleID           string
	Tenant              string
	Slug                string
	ProposedCanonicalID string
	ProposedAliasURL    string
}

// LegacyArticleMigrationConflict describes a mapping that cannot be applied
// without creating duplicate alias/canonical identity ambiguity.
type LegacyArticleMigrationConflict struct {
	Tenant     string
	Slug       string
	AliasURL   string
	ArticleIDs []string
	Reason     string
}

// LegacyArticleMigrationSkipped describes an Article row that is not a legacy
// /objects/<uuid> Article migration candidate.
type LegacyArticleMigrationSkipped struct {
	ArticleID string
	Reason    string
}

// PlanLegacyArticleMigration performs a pure dry-run over Article rows. It
// identifies legacy /objects/<uuid> Articles, proposes non-authoritative
// /articles/<slug> aliases, and reports conflicts before any write path exists.
func PlanLegacyArticleMigration(articles []*models.Article, defaultDomain string) LegacyArticleMigrationPlan {
	defaultDomain = cmsNormalizeTenant(defaultDomain)
	plan := LegacyArticleMigrationPlan{}

	canonicalByAlias := map[string]string{}
	for _, article := range articles {
		if article == nil {
			continue
		}
		tenant, slug, ok := legacyArticleCanonicalSlug(article.ID)
		if !ok {
			continue
		}
		canonicalByAlias[legacyArticleAliasKey(tenant, slug)] = strings.TrimSpace(article.ID)
	}

	candidatesByAlias := map[string][]LegacyArticleMigrationCandidate{}
	for _, article := range articles {
		candidate, skipped, ok := legacyArticleMigrationCandidateFor(article, defaultDomain)
		if !ok {
			plan.Skipped = append(plan.Skipped, skipped)
			continue
		}
		plan.Candidates = append(plan.Candidates, candidate)
		key := legacyArticleAliasKey(candidate.Tenant, candidate.Slug)
		candidatesByAlias[key] = append(candidatesByAlias[key], candidate)
	}

	for key, candidates := range candidatesByAlias {
		parts := strings.SplitN(key, "\x00", 2)
		tenant, slug := "", ""
		if len(parts) == 2 {
			tenant, slug = parts[0], parts[1]
		}
		aliasURL := ""
		if len(candidates) > 0 {
			aliasURL = candidates[0].ProposedAliasURL
		}

		if canonicalID := canonicalByAlias[key]; canonicalID != "" {
			articleIDs := []string{canonicalID}
			for _, candidate := range candidates {
				articleIDs = append(articleIDs, candidate.ArticleID)
			}
			plan.Conflicts = append(plan.Conflicts, LegacyArticleMigrationConflict{
				Tenant:     tenant,
				Slug:       slug,
				AliasURL:   aliasURL,
				ArticleIDs: sortedUniqueStrings(articleIDs),
				Reason:     LegacyArticleConflictOccupied,
			})
		}

		if len(candidates) > 1 {
			articleIDs := make([]string, 0, len(candidates))
			for _, candidate := range candidates {
				articleIDs = append(articleIDs, candidate.ArticleID)
			}
			plan.Conflicts = append(plan.Conflicts, LegacyArticleMigrationConflict{
				Tenant:     tenant,
				Slug:       slug,
				AliasURL:   aliasURL,
				ArticleIDs: sortedUniqueStrings(articleIDs),
				Reason:     LegacyArticleConflictDuplicate,
			})
		}
	}

	sort.Slice(plan.Candidates, func(i, j int) bool {
		if plan.Candidates[i].Tenant == plan.Candidates[j].Tenant {
			return plan.Candidates[i].Slug < plan.Candidates[j].Slug
		}
		return plan.Candidates[i].Tenant < plan.Candidates[j].Tenant
	})
	sort.Slice(plan.Conflicts, func(i, j int) bool {
		if plan.Conflicts[i].Tenant == plan.Conflicts[j].Tenant {
			if plan.Conflicts[i].Slug == plan.Conflicts[j].Slug {
				return plan.Conflicts[i].Reason < plan.Conflicts[j].Reason
			}
			return plan.Conflicts[i].Slug < plan.Conflicts[j].Slug
		}
		return plan.Conflicts[i].Tenant < plan.Conflicts[j].Tenant
	})
	return plan
}

func legacyArticleMigrationCandidateFor(article *models.Article, defaultDomain string) (
	LegacyArticleMigrationCandidate,
	LegacyArticleMigrationSkipped,
	bool,
) {
	if article == nil {
		return LegacyArticleMigrationCandidate{}, LegacyArticleMigrationSkipped{Reason: LegacyArticleSkipNil}, false
	}

	articleID := strings.TrimSpace(article.ID)
	if articleID == "" {
		return LegacyArticleMigrationCandidate{}, LegacyArticleMigrationSkipped{Reason: LegacyArticleSkipMissingID}, false
	}
	if article.Type != "" && !strings.EqualFold(strings.TrimSpace(article.Type), activitypub.ArticleType) {
		return LegacyArticleMigrationCandidate{}, LegacyArticleMigrationSkipped{ArticleID: articleID, Reason: LegacyArticleSkipNotArticle}, false
	}
	if _, _, ok := legacyArticleCanonicalSlug(articleID); ok {
		return LegacyArticleMigrationCandidate{}, LegacyArticleMigrationSkipped{ArticleID: articleID, Reason: LegacyArticleSkipAlreadyCanonical}, false
	}

	tenant, ok := legacyArticleObjectTenant(articleID)
	if !ok {
		return LegacyArticleMigrationCandidate{}, LegacyArticleMigrationSkipped{ArticleID: articleID, Reason: LegacyArticleSkipNotLegacyObjectID}, false
	}
	if tenant == "" {
		tenant = defaultDomain
	}
	if tenant == "" {
		return LegacyArticleMigrationCandidate{}, LegacyArticleMigrationSkipped{ArticleID: articleID, Reason: LegacyArticleSkipNotLegacyObjectID}, false
	}

	slug := common.Slugify(article.Slug)
	if slug == "" {
		return LegacyArticleMigrationCandidate{}, LegacyArticleMigrationSkipped{ArticleID: articleID, Reason: LegacyArticleSkipMissingSlug}, false
	}

	return LegacyArticleMigrationCandidate{
		ArticleID:           articleID,
		Tenant:              tenant,
		Slug:                slug,
		ProposedCanonicalID: articleID,
		ProposedAliasURL:    common.GenerateObjectID(tenant, "articles", slug),
	}, LegacyArticleMigrationSkipped{}, true
}

func legacyArticleObjectTenant(articleID string) (string, bool) {
	parsed, err := neturl.Parse(strings.TrimSpace(articleID))
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "objects" || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return cmsNormalizeTenant(parsed.Host), true
}

func legacyArticleCanonicalSlug(articleID string) (tenant string, slug string, ok bool) {
	parsed, err := neturl.Parse(strings.TrimSpace(articleID))
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "articles" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return cmsNormalizeTenant(parsed.Host), strings.TrimSpace(parts[1]), true
}

func legacyArticleAliasKey(tenant string, slug string) string {
	return cmsNormalizeTenant(tenant) + "\x00" + strings.ToLower(strings.TrimSpace(slug))
}

func sortedUniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
