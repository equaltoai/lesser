package federation

import (
	"context"
	stdErrors "errors"
	"fmt"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

type remoteNoteObjectRepository interface {
	CreateObject(ctx context.Context, object any) error
}

type remoteNoteStatusRepository interface {
	CreateStatus(ctx context.Context, status *models.Status) error
	GetStatus(ctx context.Context, statusID string) (*models.Status, error)
}

// MaterializeRemoteNote stores a remote Note object and projects it into the
// canonical Status row used by product-facing read and reply paths.
func MaterializeRemoteNote(
	ctx context.Context,
	objectRepo remoteNoteObjectRepository,
	statusRepo remoteNoteStatusRepository,
	note *activitypub.Note,
	localDomain string,
) (*models.Status, error) {
	if statusRepo == nil {
		return nil, fmt.Errorf("status repository not configured")
	}

	if note == nil {
		return nil, fmt.Errorf("remote note is required")
	}

	if objectRepo != nil {
		if err := objectRepo.CreateObject(ctx, note); err != nil && !dynamormerrors.IsConditionFailed(err) && !stdErrors.Is(err, storage.ErrAlreadyExists) {
			return nil, err
		}
	}

	status := BuildCanonicalRemoteStatus(note, localDomain)
	if status == nil {
		return nil, fmt.Errorf("canonical remote status payload is invalid")
	}
	if err := status.UpdateKeys(); err != nil {
		return nil, err
	}

	if err := statusRepo.CreateStatus(ctx, status); err != nil {
		if !dynamormerrors.IsConditionFailed(err) {
			return nil, err
		}

		existing, getErr := statusRepo.GetStatus(ctx, status.StatusID)
		if getErr != nil {
			return nil, getErr
		}
		return existing, nil
	}

	return status, nil
}
