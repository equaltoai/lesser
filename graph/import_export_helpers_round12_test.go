package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12ImportExportHelpers_TypeAndFormatParsing(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want model.ExportType
	}{
		{"archive", model.ExportTypeArchive},
		{"followers", model.ExportTypeFollowers},
		{"following", model.ExportTypeFollowing},
		{"blocks", model.ExportTypeBlocks},
		{"mutes", model.ExportTypeMutes},
		{"lists", model.ExportTypeLists},
		{"bookmarks", model.ExportTypeBookmarks},
	} {
		got, err := exportTypeFromString(tc.in)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)

		roundTrip, err := exportTypeToString(got)
		require.NoError(t, err)
		require.Equal(t, tc.in, roundTrip)
	}

	_, err := exportTypeFromString("nope")
	require.Error(t, err)
	_, err = exportTypeToString("nope")
	require.Error(t, err)

	for _, tc := range []struct {
		in   string
		want model.ExportFormat
	}{
		{"activitypub", model.ExportFormatActivitypub},
		{"mastodon", model.ExportFormatMastodon},
		{"csv", model.ExportFormatCSV},
	} {
		got, err := exportFormatFromString(tc.in)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)

		roundTrip, err := exportFormatToString(got)
		require.NoError(t, err)
		require.Equal(t, tc.in, roundTrip)
	}

	_, err = exportFormatFromString("nope")
	require.Error(t, err)
	_, err = exportFormatToString("nope")
	require.Error(t, err)
}

func TestRound12ImportExportHelpers_ImportTypeParsing(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want model.ImportType
	}{
		{"followers", model.ImportTypeFollowers},
		{"following", model.ImportTypeFollowing},
		{"blocks", model.ImportTypeBlocks},
		{"mutes", model.ImportTypeMutes},
		{"lists", model.ImportTypeLists},
		{"bookmarks", model.ImportTypeBookmarks},
	} {
		got, err := importTypeFromString(tc.in)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)

		roundTrip, err := importTypeToString(got)
		require.NoError(t, err)
		require.Equal(t, tc.in, roundTrip)
	}

	_, err := importTypeFromString("nope")
	require.Error(t, err)
	_, err = importTypeToString("nope")
	require.Error(t, err)
}

func TestRound12ImportExportHelpers_ConvertJobs(t *testing.T) {
	job, err := convertExportToGraphQLJob(nil, "", nil)
	require.NoError(t, err)
	require.Nil(t, job)

	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(time.Hour)
	export := &storageModels.Export{
		ID:          "exp-1",
		Username:    "alice",
		Status:      "completed",
		Type:        "archive",
		Format:      "csv",
		CreatedAt:   now,
		DownloadURL: "https://cdn.local/exports/exp-1.csv",
		FileSize:    12,
		RecordCount: 3,
		Error:       "warn",
		ExpiresAt:   &expires,
	}

	job, err = convertExportToGraphQLJob(export, "", nil)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.Equal(t, "exp-1", job.ID)
	require.NotNil(t, job.DownloadURL)
	require.NotNil(t, job.ExpiresAt)
	require.NotNil(t, job.FileSize)
	require.NotNil(t, job.RecordCount)
	require.NotNil(t, job.Error)

	overrideURL := "https://override.local/exp-1.csv"
	job, err = convertExportToGraphQLJob(export, overrideURL, nil)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NotNil(t, job.DownloadURL)
	require.Equal(t, overrideURL, *job.DownloadURL)

	overrideExpiry := now.Add(2 * time.Hour)
	job, err = convertExportToGraphQLJob(export, "", &overrideExpiry)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NotNil(t, job.ExpiresAt)
	require.Equal(t, overrideExpiry.Unix(), time.Time(*job.ExpiresAt).Unix())

	_, err = convertExportToGraphQLJob(&storageModels.Export{ID: "exp-2", Type: "nope", Format: "csv"}, "", nil)
	require.Error(t, err)
	_, err = convertExportToGraphQLJob(&storageModels.Export{ID: "exp-3", Type: "archive", Format: "nope"}, "", nil)
	require.Error(t, err)

	imp, err := convertImportToGraphQLJob(nil)
	require.NoError(t, err)
	require.Nil(t, imp)

	importJob, err := convertImportToGraphQLJob(&storageModels.Import{
		ID:           "imp-1",
		Username:     "alice",
		Status:       "completed",
		Type:         "followers",
		CreatedAt:    now,
		Progress:     5,
		Total:        10,
		Errors:       nil,
		SuccessCount: 1,
		SkipCount:    2,
		ErrorCount:   3,
	})
	require.NoError(t, err)
	require.NotNil(t, importJob)
	require.NotNil(t, importJob.Total)
	require.NotNil(t, importJob.Results)
	require.NotNil(t, importJob.Errors)
	require.Len(t, importJob.Errors, 0)

	_, err = convertImportToGraphQLJob(&storageModels.Import{ID: "imp-2", Type: "nope"})
	require.Error(t, err)
}
