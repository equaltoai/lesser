package common

import (
	"fmt"
	"mime"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Mastodon API validation patterns
var (
	// StatusIDPattern validates Mastodon status IDs (numeric or alphanumeric)
	MastodonStatusIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)
	
	// AccountIDPattern validates Mastodon account IDs
	MastodonAccountIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_\-\.@:\/]+$`)
	
	// HashtagPattern validates hashtag format
	HashtagPattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	
	// LanguageCodePattern validates ISO 639-1 language codes
	LanguageCodePattern = regexp.MustCompile(`^[a-z]{2}$`)
	
	// MimeTypePattern validates basic MIME type format
	MimeTypePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9!#$&\-\^_]*\/[a-zA-Z0-9][a-zA-Z0-9!#$&\-\^_.]*$`)
)

// Mastodon API constants
const (
	// Status limits
	MaxStatusLength     = 500
	MaxStatusSpoiler    = 200
	MaxStatusAttachment = 4
	
	// Account limits
	MaxDisplayNameLength = 30
	MaxBioLength         = 500
	MaxFieldNameLength   = 255
	MaxFieldValueLength  = 255
	MaxAccountFields     = 4
	
	// Media limits
	MaxMediaDescLength = 1500
	MaxMediaFileSize   = 100 * 1024 * 1024 // 100MB
	
	// Filter limits
	MaxFilterTitleLength   = 200
	MaxFilterKeywordLength = 500
	MaxFilterKeywords      = 50
	
	// Poll limits
	MaxPollOptions     = 4
	MaxPollOptionLength = 50
	MinPollDuration    = 5 * 60       // 5 minutes
	MaxPollDuration    = 30 * 24 * 60 * 60 // 30 days
	
	// List limits
	MaxListTitleLength = 100
	MaxListAccounts    = 500
	
	// Application limits
	MaxAppNameLength = 100
	MaxAppWebsiteLength = 2000
	MaxAppScopesLength = 500
)

// Mastodon visibility levels
var ValidVisibilityLevels = []string{"public", "unlisted", "private", "direct"}

// Mastodon notification types
var ValidNotificationTypes = []string{"mention", "status", "reblog", "follow", "follow_request", "favourite", "poll", "update", "admin.sign_up", "admin.report"}

// Mastodon filter contexts
var ValidFilterContexts = []string{"home", "notifications", "public", "thread", "account"}

// Mastodon filter actions
var ValidFilterActions = []string{"warn", "hide", "blur"}

// Mastodon report categories
var ValidReportCategories = []string{"spam", "violation", "other"}

// ValidateStatusParams validates parameters for status creation/update
func ValidateStatusParams(params map[string]interface{}) error {
	// Validate status content
	if status, exists := params["status"]; exists {
		if statusStr, ok := status.(string); ok {
			if err := ValidateStatusContent(statusStr); err != nil {
				return err
			}
		}
	}
	
	// Validate media_ids
	if mediaIDs, exists := params["media_ids"]; exists {
		if err := ValidateMediaIDs(mediaIDs); err != nil {
			return err
		}
	}
	
	// Validate poll options
	if pollOptions, exists := params["poll"]; exists {
		if err := ValidatePollParams(pollOptions); err != nil {
			return err
		}
	}
	
	// Validate in_reply_to_id
	if replyToID, exists := params["in_reply_to_id"]; exists {
		if replyStr, ok := replyToID.(string); ok && replyStr != "" {
			if err := ValidateMastodonStatusID(replyStr); err != nil {
				return err
			}
		}
	}
	
	// Validate sensitive flag
	if sensitive, exists := params["sensitive"]; exists {
		if _, ok := sensitive.(bool); !ok {
			return ValidationError{Field: "sensitive", Message: "must be a boolean"}
		}
	}
	
	// Validate spoiler_text
	if spoilerText, exists := params["spoiler_text"]; exists {
		if spoilerStr, ok := spoilerText.(string); ok {
			if err := ValidateSpoilerText(spoilerStr); err != nil {
				return err
			}
		}
	}
	
	// Validate visibility
	if visibility, exists := params["visibility"]; exists {
		if visStr, ok := visibility.(string); ok {
			if err := ValidateVisibility(visStr); err != nil {
				return err
			}
		}
	}
	
	// Validate language
	if language, exists := params["language"]; exists {
		if langStr, ok := language.(string); ok && langStr != "" {
			if err := ValidateLanguageCode(langStr); err != nil {
				return err
			}
		}
	}
	
	// Validate scheduled_at
	if scheduledAt, exists := params["scheduled_at"]; exists {
		if schedStr, ok := scheduledAt.(string); ok && schedStr != "" {
			if err := ValidateScheduledTime(schedStr); err != nil {
				return err
			}
		}
	}
	
	// Ensure either content or media is provided
	hasContent := false
	hasMedia := false
	
	if status, exists := params["status"]; exists {
		if statusStr, ok := status.(string); ok && strings.TrimSpace(statusStr) != "" {
			hasContent = true
		}
	}
	
	if mediaIDs, exists := params["media_ids"]; exists {
		if mediaArray, ok := mediaIDs.([]interface{}); ok && len(mediaArray) > 0 {
			hasMedia = true
		}
	}
	
	if !hasContent && !hasMedia {
		return ValidationError{Field: "status", Message: "status content or media attachments required"}
	}
	
	return nil
}

// ValidateAccountParams validates parameters for account updates
func ValidateAccountParams(params map[string]interface{}) error {
	// Validate display_name
	if displayName, exists := params["display_name"]; exists {
		if nameStr, ok := displayName.(string); ok {
			if err := ValidateDisplayName(nameStr); err != nil {
				return err
			}
		}
	}
	
	// Validate note (bio)
	if note, exists := params["note"]; exists {
		if noteStr, ok := note.(string); ok {
			if err := ValidateAccountBio(noteStr); err != nil {
				return err
			}
		}
	}
	
	// Validate avatar
	if avatar, exists := params["avatar"]; exists {
		if avatarStr, ok := avatar.(string); ok && avatarStr != "" {
			if err := ValidateMediaFile(avatarStr, "avatar"); err != nil {
				return err
			}
		}
	}
	
	// Validate header
	if header, exists := params["header"]; exists {
		if headerStr, ok := header.(string); ok && headerStr != "" {
			if err := ValidateMediaFile(headerStr, "header"); err != nil {
				return err
			}
		}
	}
	
	// Validate locked
	if locked, exists := params["locked"]; exists {
		if _, ok := locked.(bool); !ok {
			return ValidationError{Field: "locked", Message: "must be a boolean"}
		}
	}
	
	// Validate bot
	if bot, exists := params["bot"]; exists {
		if _, ok := bot.(bool); !ok {
			return ValidationError{Field: "bot", Message: "must be a boolean"}
		}
	}
	
	// Validate discoverable
	if discoverable, exists := params["discoverable"]; exists {
		if _, ok := discoverable.(bool); !ok {
			return ValidationError{Field: "discoverable", Message: "must be a boolean"}
		}
	}
	
	// Validate fields_attributes
	if fieldsAttr, exists := params["fields_attributes"]; exists {
		if err := ValidateAccountFields(fieldsAttr); err != nil {
			return err
		}
	}
	
	return nil
}

// ValidateFilterParams validates parameters for filter creation/update
func ValidateFilterParams(params map[string]interface{}) error {
	// Validate title
	if title, exists := params["title"]; exists {
		if titleStr, ok := title.(string); ok {
			if err := ValidateFilterTitle(titleStr); err != nil {
				return err
			}
		} else {
			return ValidationError{Field: "title", Message: "must be a string"}
		}
	} else {
		return ValidationError{Field: "title", Message: "is required"}
	}
	
	// Validate context
	if context, exists := params["context"]; exists {
		if err := ValidateFilterContextParam(context); err != nil {
			return err
		}
	} else {
		return ValidationError{Field: "context", Message: "is required"}
	}
	
	// Validate filter_action
	if filterAction, exists := params["filter_action"]; exists {
		if actionStr, ok := filterAction.(string); ok {
			if err := ValidateFilterAction(actionStr); err != nil {
				return err
			}
		}
	}
	
	// Validate expires_in
	if expiresIn, exists := params["expires_in"]; exists {
		if err := ValidateFilterExpiration(expiresIn); err != nil {
			return err
		}
	}
	
	// Validate keywords_attributes
	if keywordsAttr, exists := params["keywords_attributes"]; exists {
		if err := ValidateFilterKeywords(keywordsAttr); err != nil {
			return err
		}
	}
	
	return nil
}

// ValidateMediaParams validates parameters for media uploads
func ValidateMediaParams(params map[string]interface{}) error {
	// Validate file (required)
	if file, exists := params["file"]; exists {
		if fileStr, ok := file.(string); ok {
			if err := ValidateMediaFile(fileStr, "file"); err != nil {
				return err
			}
		} else {
			return ValidationError{Field: "file", Message: "must be provided"}
		}
	} else {
		return ValidationError{Field: "file", Message: "is required"}
	}
	
	// Validate description
	if description, exists := params["description"]; exists {
		if descStr, ok := description.(string); ok {
			if err := ValidateMediaDescription(descStr); err != nil {
				return err
			}
		}
	}
	
	// Validate focus (for images)
	if focus, exists := params["focus"]; exists {
		if focusStr, ok := focus.(string); ok {
			if err := ValidateMediaFocus(focusStr); err != nil {
				return err
			}
		}
	}
	
	return nil
}

// ValidateReportParams validates parameters for report creation
func ValidateReportParams(params map[string]interface{}) error {
	// Validate account_id
	if accountID, exists := params["account_id"]; exists {
		if accountStr, ok := accountID.(string); ok {
			if err := ValidateMastodonAccountID(accountStr); err != nil {
				return err
			}
		} else {
			return ValidationError{Field: "account_id", Message: "must be a string"}
		}
	} else {
		return ValidationError{Field: "account_id", Message: "is required"}
	}
	
	// Validate status_ids
	if statusIDs, exists := params["status_ids"]; exists {
		if err := ValidateReportStatusIDs(statusIDs); err != nil {
			return err
		}
	}
	
	// Validate comment
	if comment, exists := params["comment"]; exists {
		if commentStr, ok := comment.(string); ok {
			if err := ValidateReportComment(commentStr); err != nil {
				return err
			}
		}
	}
	
	// Validate category
	if category, exists := params["category"]; exists {
		if categoryStr, ok := category.(string); ok {
			if err := ValidateReportCategory(categoryStr); err != nil {
				return err
			}
		}
	}
	
	// Validate forward (boolean)
	if forward, exists := params["forward"]; exists {
		if _, ok := forward.(bool); !ok {
			return ValidationError{Field: "forward", Message: "must be a boolean"}
		}
	}
	
	return nil
}

// ValidateListParams validates parameters for list creation/update
func ValidateListParams(params map[string]interface{}) error {
	// Validate title
	if title, exists := params["title"]; exists {
		if titleStr, ok := title.(string); ok {
			if err := ValidateListTitle(titleStr); err != nil {
				return err
			}
		} else {
			return ValidationError{Field: "title", Message: "must be a string"}
		}
	} else {
		return ValidationError{Field: "title", Message: "is required"}
	}
	
	// Validate replies_policy
	if repliesPolicy, exists := params["replies_policy"]; exists {
		if policyStr, ok := repliesPolicy.(string); ok {
			if err := ValidateListRepliesPolicy(policyStr); err != nil {
				return err
			}
		}
	}
	
	return nil
}

// ValidateApplicationParams validates parameters for application creation
func ValidateApplicationParams(params map[string]interface{}) error {
	// Validate client_name
	if clientName, exists := params["client_name"]; exists {
		if nameStr, ok := clientName.(string); ok {
			if err := ValidateApplicationName(nameStr); err != nil {
				return err
			}
		} else {
			return ValidationError{Field: "client_name", Message: "must be a string"}
		}
	} else {
		return ValidationError{Field: "client_name", Message: "is required"}
	}
	
	// Validate redirect_uris
	if redirectURIs, exists := params["redirect_uris"]; exists {
		if urisStr, ok := redirectURIs.(string); ok {
			if err := ValidateRedirectURIs(urisStr); err != nil {
				return err
			}
		} else {
			return ValidationError{Field: "redirect_uris", Message: "must be a string"}
		}
	} else {
		return ValidationError{Field: "redirect_uris", Message: "is required"}
	}
	
	// Validate scopes
	if scopes, exists := params["scopes"]; exists {
		if scopesStr, ok := scopes.(string); ok {
			if err := ValidateApplicationScopes(scopesStr); err != nil {
				return err
			}
		}
	}
	
	// Validate website
	if website, exists := params["website"]; exists {
		if websiteStr, ok := website.(string); ok && websiteStr != "" {
			if err := ValidateURL(websiteStr, "website"); err != nil {
				return err
			}
		}
	}
	
	return nil
}

// Mastodon-specific validation functions

// ValidateMastodonStatusID validates Mastodon status ID format
func ValidateMastodonStatusID(statusID string) error {
	if statusID == "" {
		return ValidationError{Field: "status_id", Message: "cannot be empty"}
	}
	
	if len(statusID) > 100 {
		return ValidationError{Field: "status_id", Message: "cannot be longer than 100 characters"}
	}
	
	if !MastodonStatusIDPattern.MatchString(statusID) {
		return ValidationError{Field: "status_id", Message: "invalid format"}
	}
	
	return nil
}

// ValidateMastodonAccountID validates Mastodon account ID format
func ValidateMastodonAccountID(accountID string) error {
	if accountID == "" {
		return ValidationError{Field: "account_id", Message: "cannot be empty"}
	}
	
	if len(accountID) > 500 {
		return ValidationError{Field: "account_id", Message: "cannot be longer than 500 characters"}
	}
	
	if !MastodonAccountIDPattern.MatchString(accountID) {
		return ValidationError{Field: "account_id", Message: "invalid format"}
	}
	
	return nil
}


// ValidateAccountFields validates account profile fields
func ValidateAccountFields(fields interface{}) error {
	fieldsArray, ok := fields.([]interface{})
	if !ok {
		return ValidationError{Field: "fields_attributes", Message: "must be an array"}
	}
	
	if len(fieldsArray) > MaxAccountFields {
		return ValidationError{
			Field:   "fields_attributes",
			Message: fmt.Sprintf("cannot have more than %d fields", MaxAccountFields),
		}
	}
	
	for i, field := range fieldsArray {
		fieldObj, ok := field.(map[string]interface{})
		if !ok {
			return ValidationError{
				Field:   fmt.Sprintf("fields_attributes[%d]", i),
				Message: "must be an object",
			}
		}
		
		// Validate name
		if name, exists := fieldObj["name"]; exists {
			if nameStr, ok := name.(string); ok {
				if len(nameStr) > MaxFieldNameLength {
					return ValidationError{
						Field:   fmt.Sprintf("fields_attributes[%d].name", i),
						Message: fmt.Sprintf("cannot be longer than %d characters", MaxFieldNameLength),
					}
				}
			}
		}
		
		// Validate value
		if value, exists := fieldObj["value"]; exists {
			if valueStr, ok := value.(string); ok {
				if len(valueStr) > MaxFieldValueLength {
					return ValidationError{
						Field:   fmt.Sprintf("fields_attributes[%d].value", i),
						Message: fmt.Sprintf("cannot be longer than %d characters", MaxFieldValueLength),
					}
				}
			}
		}
	}
	
	return nil
}

// ValidateMediaIDs validates media attachment IDs
func ValidateMediaIDs(mediaIDs interface{}) error {
	idsArray, ok := mediaIDs.([]interface{})
	if !ok {
		return ValidationError{Field: "media_ids", Message: "must be an array"}
	}
	
	if len(idsArray) > MaxStatusAttachment {
		return ValidationError{
			Field:   "media_ids",
			Message: fmt.Sprintf("cannot have more than %d attachments", MaxStatusAttachment),
		}
	}
	
	for i, id := range idsArray {
		if idStr, ok := id.(string); ok {
			if err := ValidateAlphanumericID("media_id", idStr); err != nil {
				return ValidationError{
					Field:   fmt.Sprintf("media_ids[%d]", i),
					Message: err.Error(),
				}
			}
		} else {
			return ValidationError{
				Field:   fmt.Sprintf("media_ids[%d]", i),
				Message: "must be a string",
			}
		}
	}
	
	return nil
}

// ValidateMediaDescription validates media attachment descriptions
func ValidateMediaDescription(description string) error {
	if len(description) > MaxMediaDescLength {
		return ValidationError{
			Field:   "description",
			Message: fmt.Sprintf("cannot be longer than %d characters", MaxMediaDescLength),
		}
	}
	
	return nil
}

// ValidateMediaFocus validates media focus point coordinates
func ValidateMediaFocus(focus string) error {
	if focus == "" {
		return nil
	}
	
	// Focus should be in format "x,y" where x and y are floats between -1 and 1
	parts := strings.Split(focus, ",")
	if len(parts) != 2 {
		return ValidationError{Field: "focus", Message: "must be in format 'x,y'"}
	}
	
	for i, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return ValidationError{Field: "focus", Message: "coordinates must be numbers"}
		}
		
		if value < -1.0 || value > 1.0 {
			return ValidationError{Field: "focus", Message: "coordinates must be between -1 and 1"}
		}
		
		// Store back normalized value
		parts[i] = fmt.Sprintf("%.2f", value)
	}
	
	return nil
}

// ValidateMediaFile validates media file format/type
func ValidateMediaFile(fileData, fieldName string) error {
	if fileData == "" {
		return ValidationError{Field: fieldName, Message: "cannot be empty"}
	}
	
	// Basic validation - in real implementation would check file headers, size, etc.
	if len(fileData) > MaxMediaFileSize {
		return ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("file too large (max %d bytes)", MaxMediaFileSize),
		}
	}
	
	return nil
}

// ValidatePollParams validates poll creation parameters
func ValidatePollParams(poll interface{}) error {
	pollObj, ok := poll.(map[string]interface{})
	if !ok {
		return ValidationError{Field: "poll", Message: "must be an object"}
	}
	
	// Validate options
	if options, exists := pollObj["options"]; exists {
		optionsArray, ok := options.([]interface{})
		if !ok {
			return ValidationError{Field: "poll.options", Message: "must be an array"}
		}
		
		if len(optionsArray) < 2 {
			return ValidationError{Field: "poll.options", Message: "must have at least 2 options"}
		}
		
		if len(optionsArray) > MaxPollOptions {
			return ValidationError{
				Field:   "poll.options",
				Message: fmt.Sprintf("cannot have more than %d options", MaxPollOptions),
			}
		}
		
		for i, option := range optionsArray {
			if optionStr, ok := option.(string); ok {
				if optionStr == "" {
					return ValidationError{
						Field:   fmt.Sprintf("poll.options[%d]", i),
						Message: "cannot be empty",
					}
				}
				
				if len(optionStr) > MaxPollOptionLength {
					return ValidationError{
						Field:   fmt.Sprintf("poll.options[%d]", i),
						Message: fmt.Sprintf("cannot be longer than %d characters", MaxPollOptionLength),
					}
				}
			} else {
				return ValidationError{
					Field:   fmt.Sprintf("poll.options[%d]", i),
					Message: "must be a string",
				}
			}
		}
	} else {
		return ValidationError{Field: "poll.options", Message: "is required"}
	}
	
	// Validate expires_in
	if expiresIn, exists := pollObj["expires_in"]; exists {
		if expiresNum, ok := expiresIn.(float64); ok {
			duration := int(expiresNum)
			if duration < MinPollDuration {
				return ValidationError{
					Field:   "poll.expires_in",
					Message: fmt.Sprintf("must be at least %d seconds", MinPollDuration),
				}
			}
			
			if duration > MaxPollDuration {
				return ValidationError{
					Field:   "poll.expires_in",
					Message: fmt.Sprintf("cannot be more than %d seconds", MaxPollDuration),
				}
			}
		} else {
			return ValidationError{Field: "poll.expires_in", Message: "must be a number"}
		}
	} else {
		return ValidationError{Field: "poll.expires_in", Message: "is required"}
	}
	
	// Validate multiple (optional, default false)
	if multiple, exists := pollObj["multiple"]; exists {
		if _, ok := multiple.(bool); !ok {
			return ValidationError{Field: "poll.multiple", Message: "must be a boolean"}
		}
	}
	
	// Validate hide_totals (optional, default false)
	if hideTotals, exists := pollObj["hide_totals"]; exists {
		if _, ok := hideTotals.(bool); !ok {
			return ValidationError{Field: "poll.hide_totals", Message: "must be a boolean"}
		}
	}
	
	return nil
}

// ValidateSpoilerText validates content warning text
func ValidateSpoilerText(spoilerText string) error {
	if len(spoilerText) > MaxStatusSpoiler {
		return ValidationError{
			Field:   "spoiler_text",
			Message: fmt.Sprintf("cannot be longer than %d characters", MaxStatusSpoiler),
		}
	}
	
	return nil
}

// ValidateScheduledTime validates scheduled posting time
func ValidateScheduledTime(scheduledAt string) error {
	if scheduledAt == "" {
		return nil
	}
	
	scheduledTime, err := time.Parse(time.RFC3339, scheduledAt)
	if err != nil {
		return ValidationError{Field: "scheduled_at", Message: "must be a valid RFC3339 timestamp"}
	}
	
	// Must be in the future
	if scheduledTime.Before(time.Now()) {
		return ValidationError{Field: "scheduled_at", Message: "must be in the future"}
	}
	
	// Cannot be more than 1 year in the future
	maxFuture := time.Now().Add(365 * 24 * time.Hour)
	if scheduledTime.After(maxFuture) {
		return ValidationError{Field: "scheduled_at", Message: "cannot be more than 1 year in the future"}
	}
	
	return nil
}

// ValidateFilterContextParam validates filter context parameter
func ValidateFilterContextParam(context interface{}) error {
	contextArray, ok := context.([]interface{})
	if !ok {
		return ValidationError{Field: "context", Message: "must be an array"}
	}
	
	if len(contextArray) == 0 {
		return ValidationError{Field: "context", Message: "must have at least one context"}
	}
	
	for i, ctx := range contextArray {
		if ctxStr, ok := ctx.(string); ok {
			found := false
			for _, validCtx := range ValidFilterContexts {
				if ctxStr == validCtx {
					found = true
					break
				}
			}
			
			if !found {
				return ValidationError{
					Field:   fmt.Sprintf("context[%d]", i),
					Message: fmt.Sprintf("must be one of: %s", strings.Join(ValidFilterContexts, ", ")),
				}
			}
		} else {
			return ValidationError{
				Field:   fmt.Sprintf("context[%d]", i),
				Message: "must be a string",
			}
		}
	}
	
	return nil
}

// ValidateFilterExpiration validates filter expiration parameters
func ValidateFilterExpiration(expiresIn interface{}) error {
	if expiresInNum, ok := expiresIn.(float64); ok {
		duration := int(expiresInNum)
		if duration < 0 {
			return ValidationError{Field: "expires_in", Message: "cannot be negative"}
		}
		
		// Max expiration of 1 year
		maxExpiration := 365 * 24 * 60 * 60 // 1 year in seconds
		if duration > maxExpiration {
			return ValidationError{Field: "expires_in", Message: "cannot be more than 1 year"}
		}
	} else {
		return ValidationError{Field: "expires_in", Message: "must be a number"}
	}
	
	return nil
}

// ValidateFilterKeywords validates filter keywords
func ValidateFilterKeywords(keywords interface{}) error {
	keywordsArray, ok := keywords.([]interface{})
	if !ok {
		return ValidationError{Field: "keywords_attributes", Message: "must be an array"}
	}
	
	if len(keywordsArray) > MaxFilterKeywords {
		return ValidationError{
			Field:   "keywords_attributes",
			Message: fmt.Sprintf("cannot have more than %d keywords", MaxFilterKeywords),
		}
	}
	
	for i, keyword := range keywordsArray {
		keywordObj, ok := keyword.(map[string]interface{})
		if !ok {
			return ValidationError{
				Field:   fmt.Sprintf("keywords_attributes[%d]", i),
				Message: "must be an object",
			}
		}
		
		// Validate keyword text
		if keywordText, exists := keywordObj["keyword"]; exists {
			if keywordStr, ok := keywordText.(string); ok {
				if err := ValidateFilterKeyword(keywordStr); err != nil {
					return ValidationError{
						Field:   fmt.Sprintf("keywords_attributes[%d].keyword", i),
						Message: err.Error(),
					}
				}
			} else {
				return ValidationError{
					Field:   fmt.Sprintf("keywords_attributes[%d].keyword", i),
					Message: "must be a string",
				}
			}
		}
		
		// Validate whole_word (optional boolean)
		if wholeWord, exists := keywordObj["whole_word"]; exists {
			if _, ok := wholeWord.(bool); !ok {
				return ValidationError{
					Field:   fmt.Sprintf("keywords_attributes[%d].whole_word", i),
					Message: "must be a boolean",
				}
			}
		}
	}
	
	return nil
}

// ValidateReportStatusIDs validates status IDs in reports
func ValidateReportStatusIDs(statusIDs interface{}) error {
	idsArray, ok := statusIDs.([]interface{})
	if !ok {
		return ValidationError{Field: "status_ids", Message: "must be an array"}
	}
	
	if len(idsArray) > 20 {
		return ValidationError{Field: "status_ids", Message: "cannot report more than 20 statuses"}
	}
	
	for i, id := range idsArray {
		if idStr, ok := id.(string); ok {
			if err := ValidateMastodonStatusID(idStr); err != nil {
				return ValidationError{
					Field:   fmt.Sprintf("status_ids[%d]", i),
					Message: err.Error(),
				}
			}
		} else {
			return ValidationError{
				Field:   fmt.Sprintf("status_ids[%d]", i),
				Message: "must be a string",
			}
		}
	}
	
	return nil
}

// ValidateReportComment validates report comment
func ValidateReportComment(comment string) error {
	if len(comment) > 1000 {
		return ValidationError{Field: "comment", Message: "cannot be longer than 1000 characters"}
	}
	
	return nil
}

// ValidateReportCategory validates report category
func ValidateReportCategory(category string) error {
	for _, validCategory := range ValidReportCategories {
		if category == validCategory {
			return nil
		}
	}
	
	return ValidationError{
		Field:   "category",
		Message: fmt.Sprintf("must be one of: %s", strings.Join(ValidReportCategories, ", ")),
	}
}

// ValidateListTitle validates list title
func ValidateListTitle(title string) error {
	if title == "" {
		return ValidationError{Field: "title", Message: "cannot be empty"}
	}
	
	if len(title) > MaxListTitleLength {
		return ValidationError{
			Field:   "title",
			Message: fmt.Sprintf("cannot be longer than %d characters", MaxListTitleLength),
		}
	}
	
	return nil
}

// ValidateListRepliesPolicy validates list replies policy
func ValidateListRepliesPolicy(policy string) error {
	validPolicies := []string{"followed", "list", "none"}
	
	for _, validPolicy := range validPolicies {
		if policy == validPolicy {
			return nil
		}
	}
	
	return ValidationError{
		Field:   "replies_policy",
		Message: fmt.Sprintf("must be one of: %s", strings.Join(validPolicies, ", ")),
	}
}

// ValidateApplicationName validates application name
func ValidateApplicationName(name string) error {
	if name == "" {
		return ValidationError{Field: "client_name", Message: "cannot be empty"}
	}
	
	if len(name) > MaxAppNameLength {
		return ValidationError{
			Field:   "client_name",
			Message: fmt.Sprintf("cannot be longer than %d characters", MaxAppNameLength),
		}
	}
	
	return nil
}

// ValidateRedirectURIs validates OAuth redirect URIs
func ValidateRedirectURIs(uris string) error {
	if uris == "" {
		return ValidationError{Field: "redirect_uris", Message: "cannot be empty"}
	}
	
	// Split URIs by newlines or spaces
	uriList := strings.Fields(strings.ReplaceAll(uris, "\n", " "))
	
	for _, uri := range uriList {
		if uri == "urn:ietf:wg:oauth:2.0:oob" {
			continue // Special OAuth out-of-band URI
		}
		
		if err := ValidateURL(uri, "redirect_uri"); err != nil {
			return ValidationError{Field: "redirect_uris", Message: fmt.Sprintf("invalid URI: %s", uri)}
		}
	}
	
	return nil
}

// ValidateApplicationScopes validates OAuth application scopes
func ValidateApplicationScopes(scopes string) error {
	if len(scopes) > MaxAppScopesLength {
		return ValidationError{
			Field:   "scopes",
			Message: fmt.Sprintf("cannot be longer than %d characters", MaxAppScopesLength),
		}
	}
	
	// Validate individual scopes
	scopeList := strings.Fields(scopes)
	validScopes := []string{"read", "write", "follow", "push", "admin"}
	
	for _, scope := range scopeList {
		// Allow hierarchical scopes like "read:accounts"
		baseScope := strings.Split(scope, ":")[0]
		found := false
		
		for _, validScope := range validScopes {
			if baseScope == validScope {
				found = true
				break
			}
		}
		
		if !found {
			return ValidationError{
				Field:   "scopes",
				Message: fmt.Sprintf("invalid scope: %s", scope),
			}
		}
	}
	
	return nil
}

// ValidateMastodonMimeType validates MIME types for Mastodon API
func ValidateMastodonMimeType(mimeType string) error {
	if mimeType == "" {
		return ValidationError{Field: "mime_type", Message: "cannot be empty"}
	}
	
	// Parse MIME type
	mediaType, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return ValidationError{Field: "mime_type", Message: "invalid MIME type format"}
	}
	
	// Validate against allowed media types
	allowedTypes := []string{
		"image/jpeg", "image/png", "image/gif", "image/webp",
		"video/mp4", "video/webm", "video/quicktime",
		"audio/mpeg", "audio/ogg", "audio/wav", "audio/flac",
	}
	
	for _, allowedType := range allowedTypes {
		if mediaType == allowedType {
			return nil
		}
	}
	
	return ValidationError{
		Field:   "mime_type",
		Message: fmt.Sprintf("unsupported media type: %s", mediaType),
	}
}

// ValidateHashtag validates hashtag format for Mastodon
func ValidateHashtag(hashtag string) error {
	if hashtag == "" {
		return ValidationError{Field: "hashtag", Message: "cannot be empty"}
	}
	
	// Remove # if present
	tag := strings.TrimPrefix(hashtag, "#")
	
	if len(tag) > 100 {
		return ValidationError{Field: "hashtag", Message: "cannot be longer than 100 characters"}
	}
	
	if !HashtagPattern.MatchString(tag) {
		return ValidationError{Field: "hashtag", Message: "can only contain letters, numbers, and underscores"}
	}
	
	return nil
}

// ValidateMastodonTimeline validates timeline parameters
func ValidateMastodonTimeline(params map[string]interface{}) error {
	// Validate max_id
	if maxID, exists := params["max_id"]; exists {
		if maxStr, ok := maxID.(string); ok && maxStr != "" {
			if err := ValidateMastodonStatusID(maxStr); err != nil {
				return ValidationError{Field: "max_id", Message: err.Error()}
			}
		}
	}
	
	// Validate since_id
	if sinceID, exists := params["since_id"]; exists {
		if sinceStr, ok := sinceID.(string); ok && sinceStr != "" {
			if err := ValidateMastodonStatusID(sinceStr); err != nil {
				return ValidationError{Field: "since_id", Message: err.Error()}
			}
		}
	}
	
	// Validate min_id
	if minID, exists := params["min_id"]; exists {
		if minStr, ok := minID.(string); ok && minStr != "" {
			if err := ValidateMastodonStatusID(minStr); err != nil {
				return ValidationError{Field: "min_id", Message: err.Error()}
			}
		}
	}
	
	// Validate limit
	if limit, exists := params["limit"]; exists {
		if limitNum, ok := limit.(float64); ok {
			limitInt := int(limitNum)
			if err := ValidateLimit(limitInt, 80); err != nil {
				return err
			}
		} else {
			return ValidationError{Field: "limit", Message: "must be a number"}
		}
	}
	
	// Validate local (boolean)
	if local, exists := params["local"]; exists {
		if _, ok := local.(bool); !ok {
			return ValidationError{Field: "local", Message: "must be a boolean"}
		}
	}
	
	return nil
}