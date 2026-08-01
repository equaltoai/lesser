package notes

import (
	"strings"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ValidateChildReach rejects reply and quote visibility that would broaden the
// audience beyond the referenced status. Callers retain the author's requested
// visibility; this helper never clamps it.
func ValidateChildReach(relationship string, parent *models.Status, requestedVisibility string) error {
	if parent == nil {
		return apperrors.NewAppError(
			apperrors.CodeUnprocessableEntity,
			apperrors.CategoryValidation,
			"parent status is not usable",
		)
	}

	parentRank, parentOK := childReachVisibilityRank(parent.Visibility)
	requestedRank, requestedOK := childReachVisibilityRank(requestedVisibility)
	if !parentOK || !requestedOK {
		return apperrors.NewAppError(
			apperrors.CodeUnprocessableEntity,
			apperrors.CategoryValidation,
			"status visibility is not supported for this relationship",
		).
			WithMetadata("relationship", relationship).
			WithMetadata("parent_visibility", parent.Visibility).
			WithMetadata("requested_visibility", requestedVisibility)
	}
	if requestedRank < parentRank {
		return apperrors.NewAppError(
			apperrors.CodeUnprocessableEntity,
			apperrors.CategoryValidation,
			"requested visibility exceeds parent reach",
		).
			WithMetadata("relationship", relationship).
			WithMetadata("parent_id", parent.StatusID).
			WithMetadata("parent_visibility", parent.Visibility).
			WithMetadata("requested_visibility", requestedVisibility)
	}

	return nil
}

func childReachVisibilityRank(visibility string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(visibility)) {
	case models.VisibilityPublic:
		return 0, true
	case models.VisibilityUnlisted:
		return 1, true
	case models.VisibilityPrivate:
		return 2, true
	case models.VisibilityDirect:
		return 3, true
	default:
		return 0, false
	}
}
