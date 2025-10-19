package graph

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// UpdateProfile updates the authenticated user's profile.
func (r *mutationResolver) UpdateProfile(ctx context.Context, input model.UpdateProfileInput) (*activitypub.Actor, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	accountsService := r.Registry.Accounts()
	if accountsService == nil {
		return nil, errors.New("accounts service is not available")
	}

	account, err := accountsService.GetAccount(ctx, username)
	if err != nil {
		r.Logger.Error("Failed to load account for profile update",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to load account"), err)
	}

	state := r.loadPreferenceState(ctx, username)

	cmd := &accounts.UpdateProfileCommand{
		Username:     username,
		UpdaterID:    username,
		DisplayName:  coalesceStringPtr(input.DisplayName, account.User.DisplayName),
		Bio:          coalesceStringPtr(input.Bio, account.User.Note),
		Avatar:       coalesceStringPtr(input.Avatar, currentAvatar(account)),
		Header:       coalesceStringPtr(input.Header, currentHeader(account)),
		Locked:       coalesceBoolPtr(input.Locked, account.User.Locked),
		Bot:          coalesceBoolPtr(input.Bot, isAccountBot(account)),
		Discoverable: coalesceBoolPtr(input.Discoverable, account.User.Discoverable),
		NoIndex:      coalesceBoolPtr(input.NoIndex, isAccountNoIndex(account)),
		Sensitive:    coalesceBoolPtr(input.Sensitive, state.DefaultSensitive),
		Language:     coalesceStringPtr(input.Language, state.DefaultLanguage),
		Fields:       resolveProfileFields(input.Fields, account),
	}

	result, err := accountsService.UpdateProfile(ctx, cmd)
	if err != nil {
		r.Logger.Error("Failed to update profile",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to update profile"), err)
	}

	return r.convertAccountToActor(result.Account), nil
}

func resolveProfileFields(inputs []*model.ProfileFieldInput, account *storage.Account) []accounts.ProfileField {
	if len(inputs) == 0 {
		return convertStoredFields(account)
	}

	fields := make([]accounts.ProfileField, 0, len(inputs))
	for _, field := range inputs {
		if field == nil {
			continue
		}
		var verifiedAt *time.Time
		if field.VerifiedAt != nil {
			t := time.Time(*field.VerifiedAt)
			verifiedAt = &t
		}
		fields = append(fields, accounts.ProfileField{
			Name:       field.Name,
			Value:      field.Value,
			VerifiedAt: verifiedAt,
		})
	}
	return fields
}

func convertStoredFields(account *storage.Account) []accounts.ProfileField {
	if account == nil || account.User == nil || len(account.User.Fields) == 0 {
		return nil
	}
	fields := make([]accounts.ProfileField, 0, len(account.User.Fields))
	for _, field := range account.User.Fields {
		name := field["name"]
		value := field["value"]
		fields = append(fields, accounts.ProfileField{
			Name:  name,
			Value: value,
		})
	}
	return fields
}

func currentAvatar(account *storage.Account) string {
	if account != nil && account.Actor != nil && account.Actor.Icon != nil {
		return account.Actor.Icon.URL
	}
	if account != nil && account.User != nil {
		return account.User.Avatar
	}
	return ""
}

func currentHeader(account *storage.Account) string {
	if account != nil && account.Actor != nil && account.Actor.Image != nil {
		return account.Actor.Image.URL
	}
	if account != nil && account.User != nil {
		return account.User.Header
	}
	return ""
}

func isAccountBot(account *storage.Account) bool {
	if account == nil || account.Actor == nil {
		return false
	}
	return strings.EqualFold(account.Actor.Type, string(activitypub.ServiceType))
}

func isAccountNoIndex(account *storage.Account) bool {
	if account == nil || account.User == nil || account.User.Metadata == nil {
		return false
	}
	if value, ok := account.User.Metadata["no_index"].(bool); ok {
		return value
	}
	return false
}

func coalesceStringPtr(input *string, fallback string) string {
	if input != nil {
		return *input
	}
	return fallback
}

func coalesceBoolPtr(input *bool, fallback bool) bool {
	if input != nil {
		return *input
	}
	return fallback
}
