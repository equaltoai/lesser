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

// ClearAvatarResult carries the refreshed account plus the avatar the account
// pointed at before clearing, so the caller can best-effort delete the orphaned
// object.
type ClearAvatarResult struct {
	Account           *storage.Account   `json:"account"`
	PreviousAvatarURL string             `json:"previous_avatar_url"`
	Events            []*streaming.Event `json:"events"`
}

// SetAvatar points both the user profile and the ActivityPub actor icon at the
// served avatar URL for a previously stored avatar id.
func (s *Service) SetAvatar(ctx context.Context, cmd *SetAvatarCommand) (*AccountResult, error) {
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

	account.User.Avatar = s.avatarURL(account, cmd.AvatarID)
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

	return &AccountResult{
		Account: account,
		Events:  events,
	}, nil
}

// ClearAvatar removes the avatar from both stores and returns the account as
// stored afterwards.
func (s *Service) ClearAvatar(ctx context.Context, cmd *ClearAvatarCommand) (*ClearAvatarResult, error) {
	if cmd == nil || strings.TrimSpace(cmd.Username) == "" {
		return nil, ErrValidationFailed
	}

	previous, err := s.storage.Account().GetAccount(ctx, cmd.Username)
	if err != nil {
		return nil, ErrGetAccount
	}
	previousAvatarURL := storedAvatarURL(previous)

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
		Account:           account,
		PreviousAvatarURL: previousAvatarURL,
		Events:            events,
	}, nil
}

func (s *Service) avatarURL(account *storage.Account, id string) string {
	base := s.normalizeBaseURL(s.domainName)
	if base == "" && account != nil && account.User != nil {
		base = s.normalizeBaseURL(account.User.URL)
	}
	return fmt.Sprintf("%s/api/v1/avatars/%s", base, id)
}

func storedAvatarURL(account *storage.Account) string {
	if account == nil {
		return ""
	}
	if account.User != nil && strings.TrimSpace(account.User.Avatar) != "" {
		return strings.TrimSpace(account.User.Avatar)
	}
	if account.Actor != nil && account.Actor.Icon != nil {
		return strings.TrimSpace(account.Actor.Icon.URL)
	}
	return ""
}
