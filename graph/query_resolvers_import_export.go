package graph

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	importexportsvc "github.com/equaltoai/lesser/pkg/services/importexport"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

const (
	defaultImportExportLimit = 20
	maxImportExportLimit     = 100
)

func (r *queryResolver) Exports(ctx context.Context, first *int, after *model.Cursor) (*model.ExportJobConnection, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	exportRepo := r.Storage.Export()
	if exportRepo == nil {
		return nil, errors.New("export repository is not available")
	}

	exports, err := exportRepo.GetUserExportsByStatus(ctx, username, nil)
	if err != nil {
		r.Logger.Error("Failed to list exports", zap.String("user", username), zap.Error(err))
		return nil, errors.Join(errors.New("failed to list exports"), err)
	}

	sort.Slice(exports, func(i, j int) bool {
		if exports[i] == nil {
			return false
		}
		if exports[j] == nil {
			return true
		}
		return exports[i].CreatedAt.After(exports[j].CreatedAt)
	})

	limit := defaultImportExportLimit
	if first != nil && *first > 0 && *first <= maxImportExportLimit {
		limit = *first
	}

	filtered := exports
	if after != nil {
		cursorTime, parseErr := time.Parse(time.RFC3339, string(*after))
		if parseErr != nil {
			return nil, errors.Join(errors.New("invalid after cursor"), parseErr)
		}
		filtered = make([]*storageModels.Export, 0, len(exports))
		for _, exp := range exports {
			if exp == nil {
				continue
			}
			if exp.CreatedAt.Before(cursorTime) {
				filtered = append(filtered, exp)
			}
		}
	}

	hasNextPage := len(filtered) > limit
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	edges := make([]*model.ExportJobEdge, 0, len(filtered))
	for _, exp := range filtered {
		job, convErr := convertExportToGraphQLJob(exp, "", nil)
		if convErr != nil {
			return nil, convErr
		}
		cursor := model.Cursor(exp.CreatedAt.Format(time.RFC3339))
		edges = append(edges, &model.ExportJobEdge{
			Node:   job,
			Cursor: cursor,
		})
	}

	var startCursor, endCursor *model.Cursor
	if err := common.ValidateSliceNotEmpty("edges", edges); err == nil {
		startCursor = &edges[0].Cursor
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.ExportJobConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: after != nil,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(exports),
	}, nil
}

func (r *queryResolver) Export(ctx context.Context, id string) (*model.ExportJob, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("id", id); err != nil {
		return nil, err
	}

	if r.Registry != nil && r.Registry.ImportExport() != nil {
		result, err := r.Registry.ImportExport().GetExport(ctx, &importexportsvc.GetExportQuery{
			ExportID: id,
			Username: username,
		})
		if err == nil && result != nil {
			return convertExportToGraphQLJob(result.Export, result.DownloadURL, result.ExpiresAt)
		}
		if err != nil {
			r.Logger.Warn("ImportExport service GetExport failed, falling back to repository",
				zap.String("export_id", id),
				zap.Error(err))
		}
	}

	exportRepo := r.Storage.Export()
	if exportRepo == nil {
		return nil, errors.New("export repository is not available")
	}

	exp, err := exportRepo.GetExport(ctx, id)
	if err != nil {
		return nil, errors.Join(errors.New("failed to get export"), err)
	}

	if exp.Username != username {
		return nil, common.ErrForbidden(errors.New("not authorized to view this export"))
	}

	return convertExportToGraphQLJob(exp, "", nil)
}

func (r *queryResolver) Imports(ctx context.Context, first *int, after *model.Cursor) (*model.ImportJobConnection, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	importRepo := r.Storage.Import()
	if importRepo == nil {
		return nil, errors.New("import repository is not available")
	}

	imports, err := importRepo.GetUserImportsByStatus(ctx, username, nil)
	if err != nil {
		r.Logger.Error("Failed to list imports", zap.String("user", username), zap.Error(err))
		return nil, errors.Join(errors.New("failed to list imports"), err)
	}

	sort.Slice(imports, func(i, j int) bool {
		if imports[i] == nil {
			return false
		}
		if imports[j] == nil {
			return true
		}
		return imports[i].CreatedAt.After(imports[j].CreatedAt)
	})

	limit := defaultImportExportLimit
	if first != nil && *first > 0 && *first <= maxImportExportLimit {
		limit = *first
	}

	filtered := imports
	if after != nil {
		cursorTime, parseErr := time.Parse(time.RFC3339, string(*after))
		if parseErr != nil {
			return nil, errors.Join(errors.New("invalid after cursor"), parseErr)
		}
		filtered = make([]*storageModels.Import, 0, len(imports))
		for _, imp := range imports {
			if imp == nil {
				continue
			}
			if imp.CreatedAt.Before(cursorTime) {
				filtered = append(filtered, imp)
			}
		}
	}

	hasNextPage := len(filtered) > limit
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	edges := make([]*model.ImportJobEdge, 0, len(filtered))
	for _, imp := range filtered {
		job, convErr := convertImportToGraphQLJob(imp)
		if convErr != nil {
			return nil, convErr
		}
		cursor := model.Cursor(imp.CreatedAt.Format(time.RFC3339))
		edges = append(edges, &model.ImportJobEdge{
			Node:   job,
			Cursor: cursor,
		})
	}

	var startCursor, endCursor *model.Cursor
	if err := common.ValidateSliceNotEmpty("edges", edges); err == nil {
		startCursor = &edges[0].Cursor
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.ImportJobConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: after != nil,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(imports),
	}, nil
}

func (r *queryResolver) Import(ctx context.Context, id string) (*model.ImportJob, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("id", id); err != nil {
		return nil, err
	}

	if r.Registry != nil && r.Registry.ImportExport() != nil {
		result, err := r.Registry.ImportExport().GetImport(ctx, &importexportsvc.GetImportQuery{
			ImportID: id,
			Username: username,
		})
		if err == nil && result != nil {
			return convertImportToGraphQLJob(result.Import)
		}
		if err != nil {
			r.Logger.Warn("ImportExport service GetImport failed, falling back to repository",
				zap.String("import_id", id),
				zap.Error(err))
		}
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
		return nil, common.ErrForbidden(errors.New("not authorized to view this import"))
	}

	return convertImportToGraphQLJob(imp)
}
