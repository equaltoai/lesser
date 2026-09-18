package accounts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/streaming"
)

// ErrInvalidAvatarID is returned when an avatar id is not a lowercase UUID.
var ErrInvalidAvatarID = errors.New("invalid avatar id")

// ErrClearAvatar is returned when clearing an account avatar fails.
var ErrClearAvatar = errors.New("failed to clear avatar")

// ErrAvatarURLNotAccepted is returned when a profile update carries an avatar
// URL. Avatars are stored bytes with a server-minted id, so the profile URL is
// never taken from a caller.
var ErrAvatarURLNotAccepted = errors.New("avatar cannot be set by URL; upload it with POST /api/v1/accounts/avatar")

// SetAvatarCommand contains the stored avatar id to attach to an account.
type SetAvatarCommand struct {
	Username    string
	AvatarID    string
	ContentType string
}

// ClearAvatarCommand identifies the account whose avatar should be cleared.
type ClearAvatarCommand struct {
	Username string
}

// SetAvatarResult carries the refreshed account plus the avatar id the account
// previously recorded, so the caller can best-effort delete the orphaned object.
type SetAvatarResult struct {
	Account          *storage.Account   `json:"account"`
	PreviousAvatarID string             `json:"previous_avatar_id"`
	Events           []*streaming.Event `json:"events"`
}

// ClearAvatarResult carries the refreshed account plus the avatar id the account
// recorded before clearing, so the caller can best-effort delete the orphaned
// object.
type ClearAvatarResult struct {
	Account          *storage.Account   `json:"account"`
	PreviousAvatarID string             `json:"previous_avatar_id"`
	Events           []*streaming.Event `json:"events"`
}

// SetAvatar points both the user profile and the ActivityPub actor icon at the
// served avatar URL for a previously stored avatar id, and records that id on
// the user's own row. Recording the id is what authorizes a later clear to
// delete the object: deletion is never derived from a stored URL.
func (s *Service) SetAvatar(ctx context.Context, cmd *SetAvatarCommand) (*SetAvatarResult, error) {
	if cmd == nil || strings.TrimSpace(cmd.Username) == "" {
		return nil, ErrValidationFailed
	}
	if !media.IsValidAvatarID(cmd.AvatarID) {
		return nil, ErrInvalidAvatarID
	}

	account, err := s.storage.Account().GetAccount(ctx, cmd.Username)
	if err != nil {
		return nil, ErrGetAccount
	}
	if account == nil || account.User == nil {
		return nil, ErrAccountNotFound
	}

	previousAvatarID := recordedAvatarID(account)
	if previousAvatarID == cmd.AvatarID {
		previousAvatarID = ""
	}

	account.User.Avatar = s.avatarURL(account, cmd.AvatarID)
	account.User.AvatarID = cmd.AvatarID
	s.hydrateAccountActor(account)
	if account.Actor != nil {
		account.Actor.Icon = &activitypub.Image{
			URL:       account.User.Avatar,
			MediaType: strings.TrimSpace(cmd.ContentType),
		}
	}

	if err := s.storage.Account().UpdateAccount(ctx, account); err != nil {
		return nil, ErrStoreAccount
	}

	events := s.emitAccountUpdatedEvents(ctx, account)
	s.queueFederationUpdate(ctx, account)

	return &SetAvatarResult{
		Account:          account,
		PreviousAvatarID: previousAvatarID,
		Events:           events,
	}, nil
}

// ClearAvatar removes the avatar from both stores and returns the account as
// stored afterwards, together with the avatar id the account had recorded.
func (s *Service) ClearAvatar(ctx context.Context, cmd *ClearAvatarCommand) (*ClearAvatarResult, error) {
	if cmd == nil || strings.TrimSpace(cmd.Username) == "" {
		return nil, ErrValidationFailed
	}

	previous, err := s.storage.Account().GetAccount(ctx, cmd.Username)
	if err != nil {
		return nil, ErrGetAccount
	}
	previousAvatarID := recordedAvatarID(previous)

	if err := s.storage.Account().ClearAccountAvatar(ctx, cmd.Username); err != nil {
		return nil, ErrClearAvatar
	}

	account, err := s.storage.Account().GetAccount(ctx, cmd.Username)
	if err != nil {
		return nil, ErrGetAccount
	}

	events := s.emitAccountUpdatedEvents(ctx, account)
	s.queueFederationUpdate(ctx, account)

	return &ClearAvatarResult{
		Account:          account,
		PreviousAvatarID: previousAvatarID,
		Events:           events,
	}, nil
}

// recordedAvatarID returns the avatar object id an account's own row records. It
// reads the recorded id only — a stored avatar URL is never parsed to decide
// what may be deleted, because the URL is not proof of ownership. Rows written
// before the id was recorded (or with a malformed id) yield "" and authorize no
// deletion at all.
func recordedAvatarID(account *storage.Account) string {
	if account == nil || account.User == nil {
		return ""
	}
	id := strings.TrimSpace(account.User.AvatarID)
	if !media.IsValidAvatarID(id) {
		return ""
	}
	return id
}

func (s *Service) avatarURL(account *storage.Account, id string) string {
	base := s.normalizeBaseURL(s.domainName)
	if base == "" && account != nil && account.User != nil {
		base = s.normalizeBaseURL(account.User.URL)
	}
	return fmt.Sprintf("%s/api/v1/avatars/%s", base, id)
}
