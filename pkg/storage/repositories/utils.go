package repositories

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
)

// KeyUtils provides utilities for generating consistent DynamoDB keys
type KeyUtils struct{}

// NewKeyUtils creates a new KeyUtils instance
func NewKeyUtils() *KeyUtils {
	return &KeyUtils{}
}

// UserKey generates a user primary key: USER#{username}
func (k *KeyUtils) UserKey(username string) string {
	return fmt.Sprintf("USER#%s", username)
}

// ActorKey generates an actor primary key: ACTOR#{username}
func (k *KeyUtils) ActorKey(username string) string {
	return fmt.Sprintf("ACTOR#%s", username)
}

// ObjectKey generates an object primary key: object#{id}
func (k *KeyUtils) ObjectKey(objectID string) string {
	return fmt.Sprintf("object#%s", objectID)
}

// FollowKey generates a follow primary key: follow#{follower}
func (k *KeyUtils) FollowKey(followerUsername string) string {
	return fmt.Sprintf("follow#%s", followerUsername)
}

// FollowingSK generates a following sort key: following#{followed}
func (k *KeyUtils) FollowingSK(followedUsername string) string {
	return fmt.Sprintf("following#%s", followedUsername)
}

// BlockKey generates a block primary key: ACTOR#{username}#BLOCKS
func (k *KeyUtils) BlockKey(username string) string {
	return fmt.Sprintf("ACTOR#%s#BLOCKS", username)
}

// BlockedSK generates a blocked sort key: BLOCKED#{username}
func (k *KeyUtils) BlockedSK(blockedUsername string) string {
	return fmt.Sprintf("BLOCKED#%s", blockedUsername)
}

// MuteKey generates a mute primary key: MUTE#{username}
func (k *KeyUtils) MuteKey(username string) string {
	return fmt.Sprintf("MUTE#%s", username)
}

// MutedSK generates a muted sort key: MUTED#{username}
func (k *KeyUtils) MutedSK(mutedUsername string) string {
	return fmt.Sprintf("MUTED#%s", mutedUsername)
}

// HashtagKey generates a hashtag primary key: HASHTAG#{tag}
func (k *KeyUtils) HashtagKey(tag string) string {
	return fmt.Sprintf("HASHTAG#%s", strings.ToLower(strings.TrimPrefix(tag, "#")))
}

// ListKey generates a list primary key: LIST#{listID}
func (k *KeyUtils) ListKey(listID string) string {
	return fmt.Sprintf("LIST#%s", listID)
}

// OAuthStateKey generates an OAuth state primary key: OAUTH_STATE#{state}
func (k *KeyUtils) OAuthStateKey(state string) string {
	return fmt.Sprintf("OAUTH_STATE#%s", state)
}

// AuthCodeKey generates an authorization code primary key: AUTH_CODE#{code}
func (k *KeyUtils) AuthCodeKey(code string) string {
	return fmt.Sprintf("AUTH_CODE#%s", code)
}

// RefreshTokenKey generates a refresh token primary key: REFRESH_TOKEN#{token}
func (k *KeyUtils) RefreshTokenKey(token string) string {
	return fmt.Sprintf("REFRESH_TOKEN#%s", token)
}

// WebAuthnCredentialKey generates a WebAuthn credential primary key: WEBAUTHN_CREDENTIAL#{id}
func (k *KeyUtils) WebAuthnCredentialKey(credentialID string) string {
	return fmt.Sprintf("WEBAUTHN_CREDENTIAL#%s", credentialID)
}

// WalletKey generates a wallet primary key: WALLET#{address}
func (k *KeyUtils) WalletKey(address string) string {
	return fmt.Sprintf("WALLET#%s", address)
}

// Common Sort Keys
const (
	// Core sort keys
	SKMetadata   = "METADATA"
	SKProfile    = "PROFILE"
	SKState      = "STATE"
	SKCode       = "CODE"
	SKToken      = "TOKEN"
	SKCredential = "CREDENTIAL"
	SKChallenge  = "CHALLENGE"
	SKWebAuthn   = "WEBAUTHN"

	// Prefixes for range queries
	SKPrefixFilter    = "FILTER#"
	SKPrefixKeyword   = "KEYWORD#"
	SKPrefixStatus    = "STATUS#"
	SKPrefixFollowing = "FOLLOWING#"
	SKPrefixFollower  = "FOLLOWER#"
	SKPrefixWallet    = "WALLET#"
)

// GSIKeyUtils provides utilities for generating GSI keys
type GSIKeyUtils struct{}

// NewGSIKeyUtils creates a new GSIKeyUtils instance
func NewGSIKeyUtils() *GSIKeyUtils {
	return &GSIKeyUtils{}
}

// UserIndexKey generates a user index key: USER#{username}
func (g *GSIKeyUtils) UserIndexKey(username string) string {
	return fmt.Sprintf("USER#%s", username)
}

// DomainIndexKey generates a domain index key: DOMAIN#{domain}
func (g *GSIKeyUtils) DomainIndexKey(domain string) string {
	return fmt.Sprintf("DOMAIN#%s", domain)
}

// EmailIndexKey generates an email index key: EMAIL#{email}
func (g *GSIKeyUtils) EmailIndexKey(email string) string {
	return fmt.Sprintf("EMAIL#%s", strings.ToLower(email))
}

// StatusIndexKey generates a status index key: STATUS#{status}
func (g *GSIKeyUtils) StatusIndexKey(status string) string {
	return fmt.Sprintf("STATUS#%s", status)
}

// TimeUtils provides utilities for time-based operations
type TimeUtils struct{}

// NewTimeUtils creates a new TimeUtils instance
func NewTimeUtils() *TimeUtils {
	return &TimeUtils{}
}

// ToUnixTimestamp converts time to Unix timestamp for DynamoDB TTL
func (t *TimeUtils) ToUnixTimestamp(timestamp time.Time) int64 {
	return timestamp.Unix()
}

// TTLFromDuration creates a TTL timestamp from current time + duration
func (t *TimeUtils) TTLFromDuration(duration time.Duration) int64 {
	return time.Now().Add(duration).Unix()
}

// StandardTTLs provides common TTL durations
var StandardTTLs = struct {
	Session    time.Duration // 24 hours
	OAuthState time.Duration // 10 minutes
	AuthCode   time.Duration // 10 minutes
	Challenge  time.Duration // 5 minutes
	ShortTerm  time.Duration // 1 hour
	MediumTerm time.Duration // 30 days
	LongTerm   time.Duration // 90 days
}{
	Session:    24 * time.Hour,
	OAuthState: 10 * time.Minute,
	AuthCode:   10 * time.Minute,
	Challenge:  5 * time.Minute,
	ShortTerm:  1 * time.Hour,
	MediumTerm: 30 * 24 * time.Hour,
	LongTerm:   90 * 24 * time.Hour,
}

// ValidationUtils provides common validation utilities
type ValidationUtils struct{}

// NewValidationUtils creates a new ValidationUtils instance
func NewValidationUtils() *ValidationUtils {
	return &ValidationUtils{}
}

// IsValidUsername checks if a username is valid
func (v *ValidationUtils) IsValidUsername(username string) bool {
	return len(username) > 0 && len(username) <= 30 && !strings.Contains(username, "#")
}

// IsValidEmail checks if an email is valid (basic check)
func (v *ValidationUtils) IsValidEmail(email string) bool {
	return strings.Contains(email, "@") && len(email) > 3
}

// IsValidHashtag checks if a hashtag is valid
func (v *ValidationUtils) IsValidHashtag(tag string) bool {
	cleaned := strings.TrimPrefix(tag, "#")
	return len(cleaned) > 0 && len(cleaned) <= 100
}

// PaginationUtils provides utilities for cursor-based pagination
type PaginationUtils struct{}

// NewPaginationUtils creates a new PaginationUtils instance
func NewPaginationUtils() *PaginationUtils {
	return &PaginationUtils{}
}

// EncodeCursor encodes pagination cursor using proper base64 encoding
func (p *PaginationUtils) EncodeCursor(pk, sk string) string {
	// Create a proper cursor format with timestamp for enhanced sorting
	cursor := fmt.Sprintf("%s|%s|%d", pk, sk, time.Now().UnixMilli())
	return base64.URLEncoding.EncodeToString([]byte(cursor))
}

// DecodeCursor decodes pagination cursor from base64 format
func (p *PaginationUtils) DecodeCursor(cursor string) (pk, sk string, err error) {
	if cursor == "" {
		return "", "", nil
	}

	// Validate cursor format
	if err := common.ValidateRepositoryCursor(cursor); err != nil {
		return "", "", ErrorHandler.HandleGetError(err, "pagination cursor", cursor)
	}

	// Decode from base64
	decoded, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		// Fallback to legacy format for backward compatibility
		parts := strings.Split(cursor, "|")
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
		return "", "", ErrorHandler.HandleGetError(storage.ErrInvalidInput, "pagination cursor", cursor)
	}

	// Parse the decoded cursor
	parts := strings.Split(string(decoded), "|")
	if len(parts) < 2 {
		return "", "", ErrorHandler.HandleGetError(storage.ErrInvalidInput, "pagination cursor", cursor)
	}

	// Return pk and sk (ignore timestamp if present for now)
	return parts[0], parts[1], nil
}

// CommonUtils aggregates all utility types for easy access
type CommonUtils struct {
	Keys       *KeyUtils
	GSI        *GSIKeyUtils
	Time       *TimeUtils
	Validation *ValidationUtils
	Pagination *PaginationUtils
}

// NewCommonUtils creates a new CommonUtils instance with all utilities
func NewCommonUtils() *CommonUtils {
	return &CommonUtils{
		Keys:       NewKeyUtils(),
		GSI:        NewGSIKeyUtils(),
		Time:       NewTimeUtils(),
		Validation: NewValidationUtils(),
		Pagination: NewPaginationUtils(),
	}
}

// Utils provides a global instance for easy access across repositories
var Utils = NewCommonUtils()
