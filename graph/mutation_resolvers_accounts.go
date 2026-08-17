package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

const registerAccountGraphQLUnsupportedMessage = "registration is not supported over GraphQL; use POST /api/v1/accounts"

// RegisterAccount creates a new local user account.
func (r *mutationResolver) RegisterAccount(ctx context.Context, input model.RegisterAccountInput) (*model.RegisterAccountPayload, error) {
	if err := common.ValidateRequiredParam("username", input.Username); err != nil {
		return nil, err
	}

	accountService := r.Registry.Accounts()
	if accountService == nil {
		return nil, errors.New("accounts service is not available")
	}

	r.Logger.Warn("registerAccount GraphQL mutation is deprecated and disabled",
		zap.Any("path", graphql.GetPath(ctx)),
		zap.String("username", input.Username))
	return nil, apperrors.NewValidationError("registerAccount", registerAccountGraphQLUnsupportedMessage)
}

// UpdateAccountQuotePermissions updates quote permissions for the current viewer.
func (r *mutationResolver) UpdateAccountQuotePermissions(ctx context.Context, input model.UpdateAccountQuotePermissionsInput) (*model.AccountQuotePermissions, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	repos := r.Registry.GetStorage()
	if repos == nil || repos.Quote() == nil {
		return nil, errors.New("quote repository is not available")
	}

	quoteRepo := repos.Quote()

	perms, fetchErr := quoteRepo.GetQuotePermissions(ctx, username)
	notFound := fetchErr != nil && strings.Contains(strings.ToLower(fetchErr.Error()), "not found")
	if fetchErr != nil && !notFound {
		r.Logger.Error("Failed to load quote permissions",
			zap.String("username", username),
			zap.Error(fetchErr))
		return nil, errors.Join(errors.New("failed to load quote permissions"), fetchErr)
	}

	if perms == nil {
		perms = &storageModels.QuotePermissions{Username: username}
		perms.SetDefaults()
	}

	if input.AllowPublic != nil {
		perms.AllowPublic = *input.AllowPublic
	}
	if input.AllowFollowers != nil {
		perms.AllowFollowers = *input.AllowFollowers
	}
	if input.AllowMentioned != nil {
		perms.AllowMentioned = *input.AllowMentioned
	}
	if input.BlockList != nil {
		perms.BlockList = input.BlockList
	}
	if perms.BlockList == nil {
		perms.BlockList = []string{}
	}

	perms.Username = username
	if err := perms.UpdateKeys(); err != nil {
		return nil, err
	}

	if notFound {
		if err := quoteRepo.CreateQuotePermissions(ctx, perms); err != nil {
			return nil, errors.Join(errors.New("failed to create quote permissions"), err)
		}
	}

	if err := quoteRepo.UpdateQuotePermissions(ctx, perms); err != nil {
		return nil, errors.Join(errors.New("failed to update quote permissions"), err)
	}

	return &model.AccountQuotePermissions{
		Username:       username,
		AllowPublic:    perms.AllowPublic,
		AllowFollowers: perms.AllowFollowers,
		AllowMentioned: perms.AllowMentioned,
		BlockList:      perms.BlockList,
	}, nil
}

// SaveMarkers saves timeline markers for the current viewer.
func (r *mutationResolver) SaveMarkers(ctx context.Context, input []*model.SaveMarkerInput) (*model.MarkerSet, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := common.ValidateSliceNotEmpty("input", input); err != nil {
		return nil, common.ErrValidation("input", "no markers provided").InternalError
	}

	accountService := r.Registry.Accounts()
	if accountService == nil {
		return nil, errors.New("accounts service is not available")
	}

	current, err := accountService.GetMarkers(ctx, &accounts.GetMarkersQuery{
		Username: username,
	})
	if err != nil {
		r.Logger.Error("Failed to get current markers",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get current markers"), err)
	}
	currentMarkers := current.Markers

	versionByTimeline := make(map[string]int, len(currentMarkers))
	for timeline, marker := range currentMarkers {
		if marker == nil {
			continue
		}
		versionByTimeline[timeline] = marker.Version
	}

	for _, marker := range input {
		if marker == nil {
			continue
		}
		if err := common.ValidateRequiredParam("last_read_id", marker.LastReadID); err != nil {
			return nil, err
		}

		timelineKey := markerTimelineToKey(marker.Timeline)
		version := versionByTimeline[timelineKey] + 1

		if _, err := accountService.SaveMarker(ctx, &accounts.SaveMarkerCommand{
			Username:   username,
			Timeline:   timelineKey,
			LastReadID: marker.LastReadID,
			Version:    version,
		}); err != nil {
			r.Logger.Error("Failed to save marker",
				zap.String("username", username),
				zap.String("timeline", timelineKey),
				zap.Error(err))
			return nil, errors.Join(errors.New("failed to save marker"), err)
		}

		// Update local version tracking to avoid conflicts when saving multiple inputs.
		versionByTimeline[timelineKey] = version
	}

	updated, err := accountService.GetMarkers(ctx, &accounts.GetMarkersQuery{
		Username: username,
	})
	if err != nil {
		r.Logger.Error("Failed to get updated markers",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get updated markers"), err)
	}

	return &model.MarkerSet{
		Home:          convertStorageMarker(updated.Markers[TimelineTypeHome]),
		Notifications: convertStorageMarker(updated.Markers[ServiceTypeNotifications]),
	}, nil
}

func postingVisibilityFromGraphQL(visibility model.Visibility) string {
	switch visibility {
	case model.VisibilityPublic:
		return "public"
	case model.VisibilityUnlisted:
		return "unlisted"
	case model.VisibilityFollowers:
		return "private"
	case model.VisibilityDirect:
		return "direct"
	default:
		return ""
	}
}
