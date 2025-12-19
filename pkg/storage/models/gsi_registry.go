package models

// Central registry of DynamoDB GSI names to prevent drift between CDK (gsi1...gsi9) and DynamORM callers.
// Names mirror the lowercase index identifiers used in struct tags under pkg/storage/models.
const (
	// IndexGSI1 is the shared "gsi1" index used by models for state/time queries (e.g., username search, inbox/timeline, alert and federation state) as documented in struct tags.
	IndexGSI1 = "gsi1"
	// IndexGSI2 is the shared "gsi2" index used where models tag status/retry or secondary time-series lookups.
	IndexGSI2 = "gsi2"
	// IndexGSI3 is the shared "gsi3" index used by models for role/feature and alert status groupings.
	IndexGSI3 = "gsi3"
	// IndexGSI4 is the shared "gsi4" index used by models for reply/popularity timelines and related ranking buckets.
	IndexGSI4 = "gsi4"
	// IndexGSI5 is the shared "gsi5" index used by models for handle prefix search and recency windows.
	IndexGSI5 = "gsi5"
	// IndexGSI6 is the shared "gsi6" index reserved by models for retry/backoff tracking.
	IndexGSI6 = "gsi6"
	// IndexGSI7 is the shared "gsi7" index used by models for URL/canonical resource lookups.
	IndexGSI7 = "gsi7"
	// IndexGSI8 is the shared "gsi8" index used by models for analytics/time-window queries.
	IndexGSI8 = "gsi8"
	// IndexGSI9 is the shared "gsi9" index reserved for future model metadata and rollout gating.
	IndexGSI9 = "gsi9"

	// IndexOAuthClients is the dedicated index for OAuth client listings as provisioned in infrastructure.
	IndexOAuthClients = "oauth-clients-index"
)
