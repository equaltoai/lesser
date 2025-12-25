package graph

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services"
	importexportsvc "github.com/equaltoai/lesser/pkg/services/importexport"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	importMergeStrategyMerge   = "merge"
	importMergeStrategyReplace = "replace"
	importFormatCSV            = "csv"

	jobStatusFailed    = "failed"
	jobStatusCancelled = "cancelled"
)

func (r *mutationResolver) CreateExport(ctx context.Context, input model.CreateExportInput) (*model.ExportJob, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	exportRepo := r.Storage.Export()
	if exportRepo == nil {
		return nil, errors.New("export repository is not available")
	}

	// Prevent duplicate pending/processing exports of the same type (mirrors REST behavior).
	existing, err := exportRepo.GetUserExportsByStatus(ctx, username, []string{"pending", "processing"})
	if err == nil {
		typeStr, _ := exportTypeToString(input.Type)
		for _, exp := range existing {
			if exp != nil && exp.Type == typeStr {
				return nil, errors.New("export already in progress for this type")
			}
		}
	}

	if r.Registry == nil || r.Registry.ImportExport() == nil {
		return nil, errors.New("import/export service is not available")
	}

	typeStr, err := exportTypeToString(input.Type)
	if err != nil {
		return nil, err
	}
	formatStr, err := exportFormatToString(input.Format)
	if err != nil {
		return nil, err
	}

	includeMedia := false
	if input.IncludeMedia != nil {
		includeMedia = *input.IncludeMedia
	}

	var dateRange *importexportsvc.DateRange
	if input.DateRange != nil {
		dateRange = &importexportsvc.DateRange{
			Start: time.Time(input.DateRange.Start),
			End:   time.Time(input.DateRange.End),
		}
	}

	result, err := r.Registry.ImportExport().CreateExport(ctx, &importexportsvc.CreateExportCommand{
		Username:     username,
		Type:         typeStr,
		Format:       formatStr,
		IncludeMedia: includeMedia,
		DateRange:    dateRange,
		Options:      map[string]string{},
		RequestedBy:  username,
	})
	if err != nil {
		r.Logger.Error("Failed to create export",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to create export"), err)
	}

	return convertExportToGraphQLJob(result.Export, result.DownloadURL, result.ExpiresAt)
}

func (r *mutationResolver) CreateImport(ctx context.Context, input model.CreateImportInput) (*model.ImportJob, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if r.Registry == nil || r.Registry.ImportExport() == nil {
		return nil, errors.New("import/export service is not available")
	}

	typeStr, err := importTypeToString(input.Type)
	if err != nil {
		return nil, err
	}

	mode := model.ImportModeMerge
	if input.Mode != nil {
		mode = *input.Mode
	}

	mergeStrategy, err := mergeStrategyForImportMode(mode)
	if err != nil {
		return nil, err
	}

	filename := "import.csv"
	if input.Filename != nil && strings.TrimSpace(*input.Filename) != "" {
		filename = strings.TrimSpace(*input.Filename)
	} else if strings.TrimSpace(input.File.Filename) != "" {
		filename = strings.TrimSpace(input.File.Filename)
	}
	filename = path.Base(filename)

	if input.File.File == nil {
		return nil, errors.New("missing upload file")
	}

	// Read upload (enforce a conservative size limit; matches REST behavior).
	const maxUploadBytes = 10 * 1024 * 1024
	if input.File.Size > maxUploadBytes {
		return nil, fmt.Errorf("file too large (max %d bytes)", maxUploadBytes)
	}

	if _, err := input.File.File.Seek(0, io.SeekStart); err != nil {
		return nil, errors.Join(errors.New("failed to read upload"), err)
	}
	data, err := io.ReadAll(io.LimitReader(input.File.File, maxUploadBytes+1))
	if err != nil {
		return nil, errors.Join(errors.New("failed to read upload"), err)
	}
	if len(data) > maxUploadBytes {
		return nil, fmt.Errorf("file too large (max %d bytes)", maxUploadBytes)
	}
	if err := common.ValidateSliceNotEmpty("file", data); err != nil {
		return nil, err
	}

	// Upload to S3 using the same storage client used by the import/export service.
	storageClient, err := services.NewAWSS3StorageClient(ctx, r.Logger)
	if err != nil {
		return nil, errors.Join(errors.New("failed to initialize storage client"), err)
	}

	key := fmt.Sprintf("imports/%s/%s/%s", username, uuid.New().String(), filename)
	if err := storageClient.UploadFile(ctx, key, data); err != nil {
		return nil, errors.Join(errors.New("failed to upload import file"), err)
	}

	result, err := r.Registry.ImportExport().CreateImport(ctx, &importexportsvc.CreateImportCommand{
		Username:      username,
		Type:          typeStr,
		Format:        importFormatCSV,
		FileURL:       key,
		Options:       map[string]string{},
		MergeStrategy: mergeStrategy,
		RequestedBy:   username,
	})
	if err != nil {
		r.Logger.Error("Failed to create import",
			zap.String("user", username),
			zap.String("type", typeStr),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to create import"), err)
	}

	return convertImportToGraphQLJob(result.Import)
}

func (r *mutationResolver) CancelImport(ctx context.Context, id string) (*model.ImportJob, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("id", id); err != nil {
		return nil, err
	}

	importRepo := r.Storage.Import()
	if importRepo == nil {
		return nil, errors.New("import repository is not available")
	}

	imp, err := importRepo.GetImport(ctx, id)
	if err != nil {
		return nil, errors.Join(errors.New("failed to get import"), err)
	}
	if imp.Username != username {
		return nil, common.ErrForbidden(errors.New("not authorized to cancel this import"))
	}

	// If already terminal, just return current state.
	switch imp.Status {
	case JobStatusCompleted, jobStatusFailed, jobStatusCancelled:
		return convertImportToGraphQLJob(imp)
	}

	if err := importRepo.UpdateImportStatus(ctx, id, jobStatusCancelled, nil, ""); err != nil {
		return nil, errors.Join(errors.New("failed to cancel import"), err)
	}

	updated, err := importRepo.GetImport(ctx, id)
	if err != nil {
		return nil, errors.Join(errors.New("failed to get import after cancellation"), err)
	}

	return convertImportToGraphQLJob(updated)
}

func mergeStrategyForImportMode(mode model.ImportMode) (string, error) {
	switch mode {
	case model.ImportModeMerge:
		return importMergeStrategyMerge, nil
	case model.ImportModeOverwrite:
		return importMergeStrategyReplace, nil
	default:
		return "", fmt.Errorf("unsupported import mode: %q", mode)
	}
}
