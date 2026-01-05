// Package transformers provides consolidated Mastodon API transformations for
// converting between storage models and Mastodon API response formats.
package transformers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
)

// MastodonTransformer provides consolidated Mastodon API transformations
type MastodonTransformer struct {
	baseURL string
}

// NewMastodonTransformer creates a new Mastodon API transformer
func NewMastodonTransformer(baseURL string) *MastodonTransformer {
	return &MastodonTransformer{
		baseURL: baseURL,
	}
}

// PaginationInfo represents pagination metadata for Mastodon API responses
type PaginationInfo struct {
	MaxID      string `json:"max_id,omitempty"`
	MinID      string `json:"min_id,omitempty"`
	SinceID    string `json:"since_id,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more,omitempty"`
}

// === Storage to Mastodon API Transformations ===

// StorageStatusToMastodon converts storage Status to Mastodon API Status
func (t *MastodonTransformer) StorageStatusToMastodon(status *storageModels.Status, _ string) (*models.Status, error) {
	if status == nil {
		return nil, fmt.Errorf("status cannot be nil")
	}

	// Convert basic fields
	apiStatus := &models.Status{
		ID:          status.StatusID,
		Content:     status.Content,
		Sensitive:   status.Sensitive,
		SpoilerText: "", // Note: SpoilerText is not in storage model, would come from Note
		Language:    status.Language,
		Visibility:  status.Visibility,
		CreatedAt:   status.CreatedAt.Format("2006-01-02T15:04:05.000Z"),
		URI:         fmt.Sprintf("%s/users/%s/statuses/%s", t.baseURL, status.AuthorUsername, status.StatusID),
		URL:         fmt.Sprintf("%s/@%s/%s", t.baseURL, status.AuthorUsername, status.StatusID),
	}

	// Handle reply relationships
	if status.InReplyToID != "" {
		apiStatus.InReplyToID = &status.InReplyToID
		// Note: InReplyToAccountID would require additional lookup
	}

	// Build account from status fields
	account := models.Account{
		ID:       status.AuthorID,
		Username: status.AuthorUsername,
		Acct:     status.AuthorUsername,
	}
	apiStatus.Account = account

	// Transform hashtags
	apiStatus.Tags = t.transformHashtags(status.Hashtags)

	// Transform mentions
	apiStatus.Mentions = t.transformMentions(status.Mentions)

	// Initialize empty collections
	apiStatus.MediaAttachments = []interface{}{}
	apiStatus.Emojis = []interface{}{}

	// Set interaction counts (would be populated by caller)
	apiStatus.RepliesCount = 0
	apiStatus.ReblogsCount = 0
	apiStatus.FavouritesCount = 0

	// Set user-specific flags (would be populated by caller)
	apiStatus.Favourited = false
	apiStatus.Reblogged = false
	apiStatus.Bookmarked = false
	apiStatus.Muted = false
	apiStatus.Pinned = false

	return apiStatus, nil
}

// StorageAccountToMastodon converts storage Account to Mastodon API Account
func (t *MastodonTransformer) StorageAccountToMastodon(account *storage.Account) (*models.Account, error) {
	if account == nil {
		return nil, fmt.Errorf("account cannot be nil")
	}
	if account.User == nil {
		return nil, fmt.Errorf("account user cannot be nil")
	}

	apiAccount := &models.Account{
		ID:       account.User.Username, // Using username as ID
		Username: account.User.Username,
		Acct:     account.User.Username,
		URL:      fmt.Sprintf("%s/@%s", t.baseURL, account.User.Username),
	}

	// Set user fields
	if account.User != nil {
		apiAccount.DisplayName = account.User.DisplayName
		apiAccount.CreatedAt = account.User.CreatedAt.Format("2006-01-02T15:04:05.000Z")
		// Note: Bio, Locked, Bot, Discoverable fields would come from Actor, not User
	}

	// Set actor fields if available
	if account.Actor != nil {
		return t.enrichAccountWithActor(apiAccount, account.Actor), nil
	}

	// Set default values
	apiAccount.Avatar = t.getDefaultAvatar()
	apiAccount.AvatarStatic = t.getDefaultAvatar()
	apiAccount.Header = t.getDefaultHeader()
	apiAccount.HeaderStatic = t.getDefaultHeader()
	apiAccount.FollowersCount = 0
	apiAccount.FollowingCount = 0
	apiAccount.StatusesCount = 0
	apiAccount.Fields = []interface{}{}
	apiAccount.Emojis = []interface{}{}

	return apiAccount, nil
}

// StorageNotificationToMastodon converts storage Notification to Mastodon API Notification
func (t *MastodonTransformer) StorageNotificationToMastodon(notif *storageModels.Notification, account *models.Account, status *models.Status) (*models.Notification, error) {
	if notif == nil {
		return nil, fmt.Errorf("notification cannot be nil")
	}

	apiNotif := &models.Notification{
		ID:        notif.ID,
		Type:      notif.Type,
		CreatedAt: notif.CreatedAt,
	}

	if account != nil {
		apiNotif.Account = *account
	}

	if status != nil {
		apiNotif.Status = status
	}

	return apiNotif, nil
}

// === Mastodon API Input to Storage Transformations ===

// StatusCreateRequest represents a request to create a status
type StatusCreateRequest struct {
	AuthorUsername string   `json:"author_username"`
	Content        string   `json:"content"`
	Visibility     string   `json:"visibility"`
	Sensitive      bool     `json:"sensitive"`
	Language       string   `json:"language,omitempty"`
	InReplyToID    string   `json:"in_reply_to_id,omitempty"`
	MediaIDs       []string `json:"media_ids,omitempty"`
}

// MastodonStatusParamsToStorage converts Mastodon status creation params to storage format
func (t *MastodonTransformer) MastodonStatusParamsToStorage(params *models.CreateStatusRequest, authorUsername string) (*StatusCreateRequest, error) {
	if params == nil {
		return nil, fmt.Errorf("params cannot be nil")
	}

	req := &StatusCreateRequest{
		AuthorUsername: authorUsername,
		Content:        params.Status,
		Visibility:     params.Visibility,
		Sensitive:      params.Sensitive,
		Language:       params.Language,
		InReplyToID:    params.InReplyToID,
		MediaIDs:       params.MediaIDs,
	}

	// Set default visibility if not specified
	if req.Visibility == "" {
		req.Visibility = "public"
	}

	return req, nil
}

// AccountUpdateRequest represents a request to update an account
type AccountUpdateRequest struct {
	Username     string `json:"username"`
	DisplayName  string `json:"display_name,omitempty"`
	Bio          string `json:"bio,omitempty"`
	Locked       bool   `json:"locked"`
	Bot          bool   `json:"bot"`
	Discoverable bool   `json:"discoverable"`
}

// MastodonAccountParamsToStorage converts Mastodon account update params to storage format
func (t *MastodonTransformer) MastodonAccountParamsToStorage(params *models.UpdateCredentialsRequest, username string) (*AccountUpdateRequest, error) {
	if params == nil {
		return nil, fmt.Errorf("params cannot be nil")
	}

	req := &AccountUpdateRequest{
		Username:     username,
		DisplayName:  params.DisplayName,
		Bio:          params.Note,
		Locked:       params.Locked,
		Bot:          params.Bot,
		Discoverable: params.Discoverable,
	}

	return req, nil
}

// === Response Formatting Utilities ===

// FormatMastodonAPIResponse formats data for Mastodon API response
func (t *MastodonTransformer) FormatMastodonAPIResponse(data interface{}) map[string]interface{} {
	return map[string]interface{}{
		"data":      data,
		"timestamp": time.Now().Format(time.RFC3339),
	}
}

// FormatPaginatedResponse formats a paginated response with Link header information
func (t *MastodonTransformer) FormatPaginatedResponse(items []interface{}, pagination *PaginationInfo) map[string]interface{} {
	response := map[string]interface{}{
		"data": items,
	}

	if pagination != nil {
		if pagination.NextCursor != "" {
			response["next_cursor"] = pagination.NextCursor
		}
		if pagination.MaxID != "" {
			response["max_id"] = pagination.MaxID
		}
		if pagination.MinID != "" {
			response["min_id"] = pagination.MinID
		}
		response["has_more"] = pagination.HasMore
	}

	return response
}

// FormatMastodonError formats an error for Mastodon API response
func (t *MastodonTransformer) FormatMastodonError(err error) map[string]interface{} {
	if err == nil {
		return map[string]interface{}{
			"error": "unknown error",
		}
	}

	return map[string]interface{}{
		"error":      err.Error(),
		"error_type": fmt.Sprintf("%T", err),
		"timestamp":  time.Now().Format(time.RFC3339),
	}
}

// BuildLinkHeader builds a Link header for pagination
func (t *MastodonTransformer) BuildLinkHeader(baseURL string, pagination *PaginationInfo) string {
	if pagination == nil {
		return ""
	}

	var links []string

	if pagination.NextCursor != "" {
		nextURL := fmt.Sprintf("<%s?max_id=%s&limit=%d>; rel=\"next\"", baseURL, pagination.NextCursor, pagination.Limit)
		links = append(links, nextURL)
	}

	if pagination.MinID != "" {
		prevURL := fmt.Sprintf("<%s?min_id=%s&limit=%d>; rel=\"prev\"", baseURL, pagination.MinID, pagination.Limit)
		links = append(links, prevURL)
	}

	return strings.Join(links, ", ")
}

// === Relationship Augmentation ===

// AugmentAccountWithRelationship adds relationship status to account
func (t *MastodonTransformer) AugmentAccountWithRelationship(account *models.Account, relationship *models.Relationship) {
	if account == nil || relationship == nil {
		return
	}

	// Store relationship info in account metadata (if supported)
	// This would typically be handled by adding fields to the Account struct
	// For now, we'll prepare the data structure
	_ = relationship
}

// AugmentAccountWithCounts adds follower/following/status counts to account
func (t *MastodonTransformer) AugmentAccountWithCounts(account *models.Account, followersCount, followingCount, statusesCount int) {
	if account == nil {
		return
	}

	account.FollowersCount = followersCount
	account.FollowingCount = followingCount
	account.StatusesCount = statusesCount
}

// AugmentStatusWithCounts adds interaction counts to status
func (t *MastodonTransformer) AugmentStatusWithCounts(status *models.Status, repliesCount, reblogsCount, favouritesCount int) {
	if status == nil {
		return
	}

	status.RepliesCount = repliesCount
	status.ReblogsCount = reblogsCount
	status.FavouritesCount = favouritesCount
}

// AugmentStatusWithUserInteractions adds user-specific interaction state to status
func (t *MastodonTransformer) AugmentStatusWithUserInteractions(status *models.Status, favourited, reblogged, bookmarked, muted, pinned bool) {
	if status == nil {
		return
	}

	status.Favourited = favourited
	status.Reblogged = reblogged
	status.Bookmarked = bookmarked
	status.Muted = muted
	status.Pinned = pinned
}

// === Media Attachment Handling ===

// TransformStorageMediaToMastodon converts storage media to Mastodon media format
func (t *MastodonTransformer) TransformStorageMediaToMastodon(attachments []interface{}) []interface{} {
	if len(attachments) == 0 {
		return []interface{}{}
	}

	result := make([]interface{}, 0, len(attachments))
	for _, attachment := range attachments {
		if mediaAttachment := t.transformSingleMediaAttachment(attachment); mediaAttachment != nil {
			result = append(result, mediaAttachment)
		}
	}

	return result
}

// transformSingleMediaAttachment converts a single media attachment
func (t *MastodonTransformer) transformSingleMediaAttachment(attachment interface{}) map[string]interface{} {
	// Handle ActivityPub attachment format
	if attachmentMap, ok := attachment.(map[string]interface{}); ok {
		mediaAttachment := map[string]interface{}{
			"id":          t.getAttachmentID(attachmentMap),
			"type":        t.getAttachmentType(attachmentMap),
			"url":         t.getAttachmentURL(attachmentMap),
			"preview_url": t.getAttachmentPreviewURL(attachmentMap),
			"remote_url":  nil,
			"meta":        map[string]interface{}{},
		}

		if description := t.getAttachmentDescription(attachmentMap); description != "" {
			mediaAttachment["description"] = description
		}

		return mediaAttachment
	}

	return nil
}

// === Custom Emoji Processing ===

// TransformStorageEmojiToMastodon converts storage emoji to Mastodon emoji format
func (t *MastodonTransformer) TransformStorageEmojiToMastodon(emojis []interface{}) []interface{} {
	if len(emojis) == 0 {
		return []interface{}{}
	}

	result := make([]interface{}, 0, len(emojis))
	for _, emoji := range emojis {
		if emojiObj := t.transformSingleEmoji(emoji); emojiObj != nil {
			result = append(result, emojiObj)
		}
	}

	return result
}

// transformSingleEmoji converts a single emoji
func (t *MastodonTransformer) transformSingleEmoji(emoji interface{}) map[string]interface{} {
	if emojiMap, ok := emoji.(map[string]interface{}); ok {
		return map[string]interface{}{
			"shortcode":         t.getEmojiShortcode(emojiMap),
			"url":               t.getEmojiURL(emojiMap),
			"static_url":        t.getEmojiStaticURL(emojiMap),
			"visible_in_picker": true,
			"category":          nil,
		}
	}
	return nil
}

// === Mention and Hashtag Formatting ===

// transformMentions converts storage mentions to Mastodon mention format
func (t *MastodonTransformer) transformMentions(mentions []string) []interface{} {
	if len(mentions) == 0 {
		return []interface{}{}
	}

	result := make([]interface{}, 0, len(mentions))
	for _, mention := range mentions {
		mentionObj := map[string]interface{}{
			"id":       mention,
			"username": mention,
			"url":      fmt.Sprintf("%s/@%s", t.baseURL, mention),
			"acct":     mention,
		}
		result = append(result, mentionObj)
	}

	return result
}

// transformHashtags converts storage hashtags to Mastodon hashtag format
func (t *MastodonTransformer) transformHashtags(hashtags []string) []interface{} {
	if len(hashtags) == 0 {
		return []interface{}{}
	}

	result := make([]interface{}, 0, len(hashtags))
	for _, hashtag := range hashtags {
		tagObj := map[string]interface{}{
			"name": hashtag,
			"url":  fmt.Sprintf("%s/tags/%s", t.baseURL, hashtag),
		}
		result = append(result, tagObj)
	}

	return result
}

// === Integration with Transformation Framework ===

// WithTransformationFramework enhances this transformer to work with the existing transformation framework
func (t *MastodonTransformer) WithTransformationFramework() *TransformationFrameworkBridge {
	return &TransformationFrameworkBridge{
		transformer: t,
	}
}

// TransformationFrameworkBridge bridges this transformer with the existing transformation framework
type TransformationFrameworkBridge struct {
	transformer *MastodonTransformer
}

// Transform implements the Transformer interface for Account transformations
func (tfb *TransformationFrameworkBridge) Transform(_ context.Context, source *storage.Account) (models.Account, error) {
	account, err := tfb.transformer.StorageAccountToMastodon(source)
	if err != nil {
		return models.Account{}, err
	}
	return *account, nil
}

// TransformList implements the Transformer interface for Account list transformations
func (tfb *TransformationFrameworkBridge) TransformList(ctx context.Context, sources []*storage.Account) ([]models.Account, error) {
	if len(sources) == 0 {
		return []models.Account{}, nil
	}

	results := make([]models.Account, 0, len(sources))
	for _, source := range sources {
		transformed, err := tfb.Transform(ctx, source)
		if err != nil {
			return nil, fmt.Errorf("failed to transform account: %w", err)
		}
		results = append(results, transformed)
	}
	return results, nil
}

// === Caching Support ===

// CachedTransformer provides caching for frequently accessed transformations
type CachedTransformer struct {
	*MastodonTransformer
	cache map[string]interface{}
}

// NewCachedTransformer creates a cached transformer
func NewCachedTransformer(baseURL string) *CachedTransformer {
	return &CachedTransformer{
		MastodonTransformer: NewMastodonTransformer(baseURL),
		cache:               make(map[string]interface{}),
	}
}

// ClearCache clears the transformation cache
func (ct *CachedTransformer) ClearCache() {
	ct.cache = make(map[string]interface{})
}

// === Helper Methods ===

// enrichAccountWithActor enhances account with Actor information
func (t *MastodonTransformer) enrichAccountWithActor(account *models.Account, actor *activitypub.Actor) *models.Account {
	// Use existing transformation framework
	enrichedAccount := transformations.ActorToAccountBase(actor, t.baseURL)

	// Merge with existing account data, preserving storage-specific fields
	enrichedAccount.ID = account.ID
	if account.DisplayName != "" {
		enrichedAccount.DisplayName = account.DisplayName
	}
	if account.Note != "" {
		enrichedAccount.Note = account.Note
	}
	if account.CreatedAt != "" {
		enrichedAccount.CreatedAt = account.CreatedAt
	}
	if account.URL != "" {
		enrichedAccount.URL = account.URL
	}

	return &enrichedAccount
}

// getDefaultAvatar returns the default avatar URL
func (t *MastodonTransformer) getDefaultAvatar() string {
	return fmt.Sprintf("%s/avatars/original/missing.png", t.baseURL)
}

// getDefaultHeader returns the default header URL
func (t *MastodonTransformer) getDefaultHeader() string {
	return fmt.Sprintf("%s/headers/original/missing.png", t.baseURL)
}

// Media attachment helper methods
func (t *MastodonTransformer) getAttachmentID(attachment map[string]interface{}) string {
	if id, ok := attachment["id"].(string); ok {
		return id
	}
	if url, ok := attachment["url"].(string); ok {
		return url // Use URL as fallback ID
	}
	return ""
}

func (t *MastodonTransformer) getAttachmentType(attachment map[string]interface{}) string {
	if mediaType, ok := attachment["mediaType"].(string); ok {
		if strings.HasPrefix(mediaType, "image/") {
			return "image"
		}
		if strings.HasPrefix(mediaType, "video/") {
			return "video"
		}
		if strings.HasPrefix(mediaType, "audio/") {
			return "audio"
		}
	}
	return "unknown"
}

func (t *MastodonTransformer) getAttachmentURL(attachment map[string]interface{}) string {
	if url, ok := attachment["url"].(string); ok {
		return url
	}
	return ""
}

func (t *MastodonTransformer) getAttachmentPreviewURL(attachment map[string]interface{}) string {
	if previewURL, ok := attachment["preview_url"].(string); ok {
		return previewURL
	}
	// Fallback to main URL for preview
	return t.getAttachmentURL(attachment)
}

func (t *MastodonTransformer) getAttachmentDescription(attachment map[string]interface{}) string {
	if name, ok := attachment["name"].(string); ok {
		return name
	}
	if description, ok := attachment["description"].(string); ok {
		return description
	}
	return ""
}

// Emoji helper methods
func (t *MastodonTransformer) getEmojiShortcode(emoji map[string]interface{}) string {
	if shortcode, ok := emoji["shortcode"].(string); ok {
		return shortcode
	}
	if name, ok := emoji["name"].(string); ok {
		// Remove colons if present
		return strings.Trim(name, ":")
	}
	return ""
}

func (t *MastodonTransformer) getEmojiURL(emoji map[string]interface{}) string {
	if url, ok := emoji["url"].(string); ok {
		return url
	}
	return ""
}

func (t *MastodonTransformer) getEmojiStaticURL(emoji map[string]interface{}) string {
	if staticURL, ok := emoji["static_url"].(string); ok {
		return staticURL
	}
	// Fallback to main URL
	return t.getEmojiURL(emoji)
}

// BatchProcessor provides batch processing for multiple transformations
type BatchProcessor struct {
	transformer *MastodonTransformer
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(baseURL string) *BatchProcessor {
	return &BatchProcessor{
		transformer: NewMastodonTransformer(baseURL),
	}
}

// ProcessStatusBatch processes multiple statuses in batch
func (bp *BatchProcessor) ProcessStatusBatch(statuses []*storageModels.Status, viewerUsername string) ([]*models.Status, error) {
	if len(statuses) == 0 {
		return []*models.Status{}, nil
	}

	results := make([]*models.Status, 0, len(statuses))
	for _, status := range statuses {
		transformed, err := bp.transformer.StorageStatusToMastodon(status, viewerUsername)
		if err != nil {
			var statusID string
			if status != nil {
				statusID = status.StatusID
			}
			return nil, fmt.Errorf("failed to transform status %s: %w", statusID, err)
		}
		results = append(results, transformed)
	}

	return results, nil
}

// ProcessAccountBatch processes multiple accounts in batch
func (bp *BatchProcessor) ProcessAccountBatch(accounts []*storage.Account) ([]*models.Account, error) {
	if len(accounts) == 0 {
		return []*models.Account{}, nil
	}

	results := make([]*models.Account, 0, len(accounts))
	for _, account := range accounts {
		transformed, err := bp.transformer.StorageAccountToMastodon(account)
		if err != nil {
			var username string
			if account != nil && account.User != nil {
				username = account.User.Username
			}
			return nil, fmt.Errorf("failed to transform account %s: %w", username, err)
		}
		results = append(results, transformed)
	}

	return results, nil
}
