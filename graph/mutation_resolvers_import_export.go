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

	graphQLImportMaxUploadBytes = 10 * 1024 * 1024
	graphQLExportRateWindow     = time.Hour
	graphQLImportRateWindow     = 24 * time.Hour
)

func (r *mutationResolver) CreateExport(ctx context.Context, input model.CreateExportInput) (*model.ExportJob, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
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

	if err := r.enforceGraphQLExportGates(ctx, username, typeStr, formatStr, includeMedia, input.DateRange); err != nil {
		return nil, err
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
	if input.File.Size > graphQLImportMaxUploadBytes {
		return nil, fmt.Errorf("file too large (max %d bytes)", graphQLImportMaxUploadBytes)
	}

	if _, err := input.File.File.Seek(0, io.SeekStart); err != nil {
		return nil, errors.Join(errors.New("failed to read upload"), err)
	}
	data, err := io.ReadAll(io.LimitReader(input.File.File, graphQLImportMaxUploadBytes+1))
	if err != nil {
		return nil, errors.Join(errors.New("failed to read upload"), err)
	}
	if len(data) > graphQLImportMaxUploadBytes {
		return nil, fmt.Errorf("file too large (max %d bytes)", graphQLImportMaxUploadBytes)
	}
	if err := common.ValidateSliceNotEmpty("file", data); err != nil {
		return nil, err
	}
	if err := validateGraphQLImportFile(ctx, r.graphQLImportExportLogger(), data); err != nil {
		return nil, err
	}
	if err := r.enforceGraphQLImportGates(ctx, username, typeStr, mergeStrategy, len(data)); err != nil {
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

func (r *mutationResolver) enforceGraphQLExportGates(
	ctx context.Context,
	username string,
	typeStr string,
	formatStr string,
	includeMedia bool,
	dateRange *model.DateRangeInput,
) error {
	exportRepo := r.Storage.Export()
	if exportRepo == nil {
		return errors.New("export repository is not available")
	}

	if formatStr == importExportFormatCSV && typeStr == importExportTypeArchive {
		return errors.New("CSV format not available for archive exports")
	}
	if err := validateGraphQLExportDateRange(dateRange); err != nil {
		return err
	}

	if existing, err := exportRepo.GetUserExportsByStatus(ctx, username, []string{"pending", "processing"}); err == nil {
		for _, exp := range existing {
			if exp != nil && exp.Type == typeStr {
				return errors.New("export already in progress for this type")
			}
		}
	} else {
		r.graphQLImportExportLogger().Warn("failed to check existing exports", zap.Error(err))
	}

	if existing, err := exportRepo.GetUserExportsByStatus(ctx, username, []string{"pending", "processing"}); err == nil {
		recentCount := 0
		cutoff := time.Now().Add(-graphQLExportRateWindow)
		for _, exp := range existing {
			if exp != nil && exp.Type == typeStr && exp.CreatedAt.After(cutoff) {
				recentCount++
			}
		}
		if recentCount >= 1 {
			return fmt.Errorf("rate limit exceeded: limit 1 per %d seconds", int(graphQLExportRateWindow.Seconds()))
		}
	} else {
		r.graphQLImportExportLogger().Warn("failed to check export rate limit", zap.Error(err))
	}

	estimatedCost := graphQLEstimateExportCost(typeStr, formatStr, includeMedia)
	return r.enforceGraphQLImportExportBudget(ctx, username, 0, estimatedCost)
}

func (r *mutationResolver) enforceGraphQLImportGates(
	ctx context.Context,
	username string,
	typeStr string,
	mergeStrategy string,
	fileSize int,
) error {
	importRepo := r.Storage.Import()
	if importRepo == nil {
		return errors.New("import repository is not available")
	}

	if existing, err := importRepo.GetUserImportsByStatus(ctx, username, []string{"pending", "processing"}); err == nil {
		for _, imp := range existing {
			if imp != nil && imp.Type == typeStr {
				return errors.New("import already in progress for this type")
			}
		}
	} else {
		r.graphQLImportExportLogger().Warn("failed to check existing imports", zap.Error(err))
	}

	if existing, err := importRepo.GetUserImportsByStatus(ctx, username, []string{"pending", "processing"}); err == nil {
		recentCount := 0
		cutoff := time.Now().Add(-graphQLImportRateWindow)
		for _, imp := range existing {
			if imp != nil && imp.Type == typeStr && imp.CreatedAt.After(cutoff) {
				recentCount++
			}
		}
		if recentCount >= 1 {
			return fmt.Errorf("rate limit exceeded: limit 1 per %d seconds", int(graphQLImportRateWindow.Seconds()))
		}
	} else {
		r.graphQLImportExportLogger().Warn("failed to check import rate limit", zap.Error(err))
	}

	estimatedCost := graphQLEstimateImportCost(typeStr, mergeStrategy, fileSize)
	return r.enforceGraphQLImportExportBudget(ctx, username, estimatedCost, 0)
}

func (r *mutationResolver) enforceGraphQLImportExportBudget(
	ctx context.Context,
	username string,
	importCostMicroCents int64,
	exportCostMicroCents int64,
) error {
	importRepo := r.Storage.Import()
	if importRepo == nil {
		return errors.New("import repository is not available")
	}

	budget, withinLimits, err := importRepo.CheckBudgetLimits(ctx, username, importCostMicroCents, exportCostMicroCents)
	if err != nil {
		r.graphQLImportExportLogger().Warn("failed to check import/export budget limits", zap.Error(err))
		return nil
	}
	if withinLimits {
		return nil
	}
	if budget == nil {
		return errors.New("budget limit exceeded")
	}

	limitType := "combined"
	remaining := budget.GetRemainingCombinedBudget()
	switch {
	case importCostMicroCents > 0 && budget.IsImportOverLimit(importCostMicroCents):
		limitType = "import"
		remaining = budget.GetRemainingImportBudget()
	case exportCostMicroCents > 0 && budget.IsExportOverLimit(exportCostMicroCents):
		limitType = "export"
		remaining = budget.GetRemainingExportBudget()
	case budget.IsCombinedOverLimit(importCostMicroCents, exportCostMicroCents):
		limitType = "combined"
		remaining = budget.GetRemainingCombinedBudget()
	}

	estimatedCost := importCostMicroCents + exportCostMicroCents
	return fmt.Errorf(
		"%s budget limit exceeded (estimated_cost=%.6f, remaining_budget=%.6f, budget_period=%s, budget_resets_at=%s)",
		limitType,
		float64(estimatedCost)/1_000_000.0,
		float64(remaining)/1_000_000.0,
		budget.Period,
		budget.NextResetAt.Format(time.RFC3339),
	)
}

func validateGraphQLExportDateRange(dateRange *model.DateRangeInput) error {
	if dateRange == nil {
		return nil
	}
	start := time.Time(dateRange.Start)
	end := time.Time(dateRange.End)
	if start.After(end) {
		return errors.New("invalid date range: start must be before end")
	}
	if end.After(time.Now()) {
		return errors.New("invalid date range: end must not be in the future")
	}
	return nil
}

func validateGraphQLImportFile(ctx context.Context, logger *zap.Logger, data []byte) error {
	fileValidator, err := services.NewFileValidationService(logger)
	if err != nil {
		if err := basicGraphQLImportFileValidation(data); err != nil {
			return err
		}
		return nil
	}

	result, err := fileValidator.ValidateFile(ctx, data, services.DefaultImportValidationConfig())
	if err != nil {
		return errors.Join(errors.New("file validation failed"), err)
	}
	if !result.Valid {
		return fmt.Errorf("file validation failed: %s", strings.Join(result.Errors, "; "))
	}
	return nil
}

func basicGraphQLImportFileValidation(data []byte) error {
	if !isValidGraphQLImportContentType(detectGraphQLImportContentType(data)) {
		return errors.New("unsupported file format")
	}
	return nil
}

func detectGraphQLImportContentType(data []byte) string {
	if len(data) == 0 {
		return contentTypeApplicationOctetStream
	}
	switch data[0] {
	case '{', '[':
		return "application/json"
	}
	if strings.Contains(string(data[:min(100, len(data))]), ",") {
		return "text/csv"
	}
	return contentTypeApplicationOctetStream
}

func isValidGraphQLImportContentType(contentType string) bool {
	for _, valid := range []string{"application/json", "text/csv", "text/plain"} {
		if strings.HasPrefix(contentType, valid) {
			return true
		}
	}
	return false
}

func graphQLEstimateExportCost(typeStr string, formatStr string, includeMedia bool) int64 {
	baseCost := int64(50000)
	switch typeStr {
	case importExportTypeArchive:
		baseCost *= 10
	case importExportTypeFollowers, importExportTypeFollowing:
		baseCost *= 3
	}
	switch formatStr {
	case importExportFormatActivitypub, importExportFormatMastodon:
		baseCost *= 2
	}
	if includeMedia {
		baseCost *= 5
	}
	return baseCost
}

func graphQLEstimateImportCost(typeStr string, mergeStrategy string, fileSize int) int64 {
	baseCost := int64(30000)
	fileSizeKB := int64(fileSize) / 1024
	if fileSizeKB < 1 {
		fileSizeKB = 1
	}
	sizeCost := fileSizeKB * 1000
	switch typeStr {
	case importExportTypeFollowers, importExportTypeFollowing:
		baseCost *= 3
	case importExportTypeLists, importExportTypeBookmarks:
		baseCost *= 2
	}
	if mergeStrategy == importMergeStrategyReplace {
		baseCost *= 2
	}
	return baseCost + sizeCost
}

func (r *mutationResolver) graphQLImportExportLogger() *zap.Logger {
	if r != nil && r.Logger != nil {
		return r.Logger
	}
	return zap.NewNop()
}
