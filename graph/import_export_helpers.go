package graph

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
)

const (
	importExportTypeArchive   = "archive"
	importExportTypeFollowers = "followers"
	importExportTypeFollowing = "following"
	importExportTypeBlocks    = "blocks"
	importExportTypeMutes     = "mutes"
	importExportTypeLists     = "lists"
	importExportTypeBookmarks = "bookmarks"

	importExportFormatActivitypub = "activitypub"
	importExportFormatMastodon    = "mastodon"
	importExportFormatCSV         = "csv"
)

func exportTypeFromString(value string) (model.ExportType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case importExportTypeArchive:
		return model.ExportTypeArchive, nil
	case importExportTypeFollowers:
		return model.ExportTypeFollowers, nil
	case importExportTypeFollowing:
		return model.ExportTypeFollowing, nil
	case importExportTypeBlocks:
		return model.ExportTypeBlocks, nil
	case importExportTypeMutes:
		return model.ExportTypeMutes, nil
	case importExportTypeLists:
		return model.ExportTypeLists, nil
	case importExportTypeBookmarks:
		return model.ExportTypeBookmarks, nil
	default:
		return "", fmt.Errorf("unknown export type: %q", value)
	}
}

func exportTypeToString(value model.ExportType) (string, error) {
	switch value {
	case model.ExportTypeArchive:
		return importExportTypeArchive, nil
	case model.ExportTypeFollowers:
		return importExportTypeFollowers, nil
	case model.ExportTypeFollowing:
		return importExportTypeFollowing, nil
	case model.ExportTypeBlocks:
		return importExportTypeBlocks, nil
	case model.ExportTypeMutes:
		return importExportTypeMutes, nil
	case model.ExportTypeLists:
		return importExportTypeLists, nil
	case model.ExportTypeBookmarks:
		return importExportTypeBookmarks, nil
	default:
		return "", fmt.Errorf("unknown export type: %q", value)
	}
}

func exportFormatFromString(value string) (model.ExportFormat, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case importExportFormatActivitypub:
		return model.ExportFormatActivitypub, nil
	case importExportFormatMastodon:
		return model.ExportFormatMastodon, nil
	case importExportFormatCSV:
		return model.ExportFormatCSV, nil
	default:
		return "", fmt.Errorf("unknown export format: %q", value)
	}
}

func exportFormatToString(value model.ExportFormat) (string, error) {
	switch value {
	case model.ExportFormatActivitypub:
		return importExportFormatActivitypub, nil
	case model.ExportFormatMastodon:
		return importExportFormatMastodon, nil
	case model.ExportFormatCSV:
		return importExportFormatCSV, nil
	default:
		return "", fmt.Errorf("unknown export format: %q", value)
	}
}

func importTypeFromString(value string) (model.ImportType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case importExportTypeFollowers:
		return model.ImportTypeFollowers, nil
	case importExportTypeFollowing:
		return model.ImportTypeFollowing, nil
	case importExportTypeBlocks:
		return model.ImportTypeBlocks, nil
	case importExportTypeMutes:
		return model.ImportTypeMutes, nil
	case importExportTypeLists:
		return model.ImportTypeLists, nil
	case importExportTypeBookmarks:
		return model.ImportTypeBookmarks, nil
	default:
		return "", fmt.Errorf("unknown import type: %q", value)
	}
}

func importTypeToString(value model.ImportType) (string, error) {
	switch value {
	case model.ImportTypeFollowers:
		return importExportTypeFollowers, nil
	case model.ImportTypeFollowing:
		return importExportTypeFollowing, nil
	case model.ImportTypeBlocks:
		return importExportTypeBlocks, nil
	case model.ImportTypeMutes:
		return importExportTypeMutes, nil
	case model.ImportTypeLists:
		return importExportTypeLists, nil
	case model.ImportTypeBookmarks:
		return importExportTypeBookmarks, nil
	default:
		return "", fmt.Errorf("unknown import type: %q", value)
	}
}

func convertExportToGraphQLJob(export *storageModels.Export, downloadURL string, expiresAt *time.Time) (*model.ExportJob, error) {
	if export == nil {
		return nil, nil
	}

	exportType, err := exportTypeFromString(export.Type)
	if err != nil {
		return nil, err
	}

	exportFormat, err := exportFormatFromString(export.Format)
	if err != nil {
		return nil, err
	}

	var downloadURLPtr *string
	if downloadURL != "" {
		downloadURLPtr = &downloadURL
	} else if export.DownloadURL != "" {
		downloadURLPtr = &export.DownloadURL
	}

	var expiresAtPtr *model.Time
	if expiresAt != nil {
		t := model.Time(*expiresAt)
		expiresAtPtr = &t
	} else if export.ExpiresAt != nil {
		t := model.Time(*export.ExpiresAt)
		expiresAtPtr = &t
	}

	var fileSizePtr *int
	if export.FileSize > 0 {
		fileSize := int(export.FileSize)
		fileSizePtr = &fileSize
	}

	var recordCountPtr *int
	if export.RecordCount > 0 {
		recordCount := int(export.RecordCount)
		recordCountPtr = &recordCount
	}

	var errorPtr *string
	if export.Error != "" {
		errorPtr = &export.Error
	}

	return &model.ExportJob{
		ID:          export.ID,
		Status:      export.Status,
		Type:        exportType,
		Format:      exportFormat,
		CreatedAt:   model.Time(export.CreatedAt),
		DownloadURL: downloadURLPtr,
		ExpiresAt:   expiresAtPtr,
		FileSize:    fileSizePtr,
		RecordCount: recordCountPtr,
		Error:       errorPtr,
	}, nil
}

func convertImportToGraphQLJob(imp *storageModels.Import) (*model.ImportJob, error) {
	if imp == nil {
		return nil, nil
	}

	importType, err := importTypeFromString(imp.Type)
	if err != nil {
		return nil, err
	}

	var totalPtr *int
	if imp.Total > 0 {
		total := imp.Total
		totalPtr = &total
	}

	errorsList := imp.Errors
	if errorsList == nil {
		errorsList = []string{}
	}

	var resultsPtr *model.ImportResults
	if imp.SuccessCount != 0 || imp.SkipCount != 0 || imp.ErrorCount != 0 {
		resultsPtr = &model.ImportResults{
			Success: imp.SuccessCount,
			Skipped: imp.SkipCount,
			Failed:  imp.ErrorCount,
		}
	}

	return &model.ImportJob{
		ID:        imp.ID,
		Status:    imp.Status,
		Type:      importType,
		CreatedAt: model.Time(imp.CreatedAt),
		Processed: imp.Progress,
		Total:     totalPtr,
		Errors:    errorsList,
		Results:   resultsPtr,
	}, nil
}
