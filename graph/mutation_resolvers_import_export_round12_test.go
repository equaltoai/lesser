package graph

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/equaltoai/lesser/graph/model"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

type round12ErrReadSeeker struct {
	readErr error
	seekErr error
	data    []byte
	offset  int64
}

func (r *round12ErrReadSeeker) Read(p []byte) (int, error) {
	if r.readErr != nil {
		return 0, r.readErr
	}
	if r.offset >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += int64(n)
	return n, nil
}

func (r *round12ErrReadSeeker) Seek(offset int64, whence int) (int64, error) {
	if r.seekErr != nil {
		return 0, r.seekErr
	}

	var base int64
	switch whence {
	case io.SeekStart:
		base = 0
	case io.SeekCurrent:
		base = r.offset
	case io.SeekEnd:
		base = int64(len(r.data))
	default:
		return 0, errors.New("invalid whence")
	}

	next := base + offset
	if next < 0 {
		return 0, errors.New("negative position")
	}
	r.offset = next
	return r.offset, nil
}

func TestRound12MutationResolvers_ImportExport_HelperFunctions(t *testing.T) {
	strategy, err := mergeStrategyForImportMode(model.ImportModeMerge)
	require.NoError(t, err)
	require.Equal(t, "merge", strategy)

	strategy, err = mergeStrategyForImportMode(model.ImportModeOverwrite)
	require.NoError(t, err)
	require.Equal(t, "replace", strategy)

	_, err = mergeStrategyForImportMode(model.ImportMode("nope"))
	require.Error(t, err)

	require.Equal(t, int64(5_000_000), graphQLEstimateExportCost("archive", "activitypub", true))
	require.Greater(
		t,
		graphQLEstimateImportCost("followers", importMergeStrategyReplace, 1),
		graphQLEstimateImportCost("followers", importMergeStrategyMerge, 1),
	)
	require.NoError(t, basicGraphQLImportFileValidation([]byte("account,show_reblogs\n@bob@example.com,true\n")))
	require.Error(t, basicGraphQLImportFileValidation([]byte("no-commas-and-not-json")))
}

func TestRound12MutationResolvers_ImportExport_CreateExport_DuplicateAndValidation(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	mut := &mutationResolver{resolver}

	// Duplicate export path uses existing pending/processing exports from repo.
	state.autoPopulateAll = true
	state.autoPopulateCount = 1

	_, err := mut.CreateExport(round12AuthContext("alice"), model.CreateExportInput{
		Type:   model.ExportTypeArchive,
		Format: model.ExportFormatCSV,
	})
	require.Error(t, err)

	// Registry not available error.
	state.autoPopulateAll = false
	resolver.Registry = nil
	_, err = mut.CreateExport(round12AuthContext("alice"), model.CreateExportInput{
		Type:   model.ExportTypeArchive,
		Format: model.ExportFormatCSV,
	})
	require.Error(t, err)

	// Invalid type validation.
	resolver, _ = newRound12GraphResolver(t)
	mut = &mutationResolver{resolver}
	_, err = mut.CreateExport(round12AuthContext("alice"), model.CreateExportInput{
		Type:   model.ExportType("NOPE"),
		Format: model.ExportFormatCSV,
	})
	require.Error(t, err)
}

func TestRound12MutationResolvers_ImportExport_CreateExport_RESTGates(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)
	mut := &mutationResolver{resolver}

	_, err := mut.CreateExport(round12AuthContext("alice"), model.CreateExportInput{
		Type:   model.ExportTypeArchive,
		Format: model.ExportFormatCSV,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "CSV format")

	futureEnd := model.Time(time.Now().Add(time.Hour))
	pastStart := model.Time(time.Now().Add(-time.Hour))
	_, err = mut.CreateExport(round12AuthContext("alice"), model.CreateExportInput{
		Type:   model.ExportTypeFollowers,
		Format: model.ExportFormatCSV,
		DateRange: &model.DateRangeInput{
			Start: pastStart,
			End:   futureEnd,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "date range")

	now := time.Now()
	budget := &storageModels.ImportBudget{
		Username:              "alice",
		Period:                storageModels.PeriodDaily,
		IsActive:              true,
		ExportLimitMicroCents: 1,
		NextResetAt:           now.Add(24 * time.Hour),
	}
	budget.UpdateKeys()
	storage.queryState.seededImportBudgets = map[string]*storageModels.ImportBudget{
		budget.PK + "#" + budget.SK: budget,
	}

	_, err = mut.CreateExport(round12AuthContext("alice"), model.CreateExportInput{
		Type:   model.ExportTypeArchive,
		Format: model.ExportFormatActivitypub,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "export budget limit exceeded")
}

func TestRound12MutationResolvers_ImportExport_CreateImport_EarlyFailures(t *testing.T) {
	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)
	mut := &mutationResolver{resolver}

	// Service not available.
	resolver.Registry = nil
	_, err := mut.CreateImport(round12AuthContext("alice"), model.CreateImportInput{
		Type: model.ImportTypeFollowers,
		File: graphql.Upload{Filename: "x.csv", File: bytes.NewReader([]byte("a")), Size: 1},
	})
	require.Error(t, err)

	// Restore registry for remaining validation-only paths.
	resolver, _ = newRound12GraphResolver(t)
	mut = &mutationResolver{resolver}

	// Missing file.
	_, err = mut.CreateImport(round12AuthContext("alice"), model.CreateImportInput{
		Type: model.ImportTypeFollowers,
		File: graphql.Upload{Filename: "x.csv", File: nil, Size: 0},
	})
	require.Error(t, err)

	// Too large (size-only check; doesn't require allocating bytes).
	tooLarge := int64(10*1024*1024 + 1)
	_, err = mut.CreateImport(round12AuthContext("alice"), model.CreateImportInput{
		Type: model.ImportTypeFollowers,
		File: graphql.Upload{Filename: "x.csv", File: bytes.NewReader([]byte("a")), Size: tooLarge},
	})
	require.Error(t, err)

	// Seek failure.
	_, err = mut.CreateImport(round12AuthContext("alice"), model.CreateImportInput{
		Type: model.ImportTypeFollowers,
		File: graphql.Upload{Filename: "x.csv", File: &round12ErrReadSeeker{seekErr: errors.New("seek")}, Size: 1},
	})
	require.Error(t, err)

	// Read failure.
	_, err = mut.CreateImport(round12AuthContext("alice"), model.CreateImportInput{
		Type: model.ImportTypeFollowers,
		File: graphql.Upload{Filename: "x.csv", File: &round12ErrReadSeeker{data: []byte("a"), readErr: errors.New("read")}, Size: 1},
	})
	require.Error(t, err)

	// Empty data.
	_, err = mut.CreateImport(round12AuthContext("alice"), model.CreateImportInput{
		Type: model.ImportTypeFollowers,
		File: graphql.Upload{Filename: "x.csv", File: bytes.NewReader(nil), Size: 0},
	})
	require.Error(t, err)
}

func TestRound12MutationResolvers_ImportExport_CreateImport_RESTGates(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")

	resolver, storage := newRound12GraphResolver(t)
	mut := &mutationResolver{resolver}

	_, err := mut.CreateImport(round12AuthContext("alice"), model.CreateImportInput{
		Type: model.ImportTypeFollowers,
		File: graphql.Upload{
			Filename: "followers.txt",
			File:     bytes.NewReader([]byte("no-commas-and-not-json")),
			Size:     int64(len("no-commas-and-not-json")),
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "file validation failed")

	data := []byte(`{"account":"@bob@example.com"}`)
	estimated := graphQLEstimateImportCost(importExportTypeFollowers, importMergeStrategyMerge, len(data))
	budget := &storageModels.ImportBudget{
		Username:              "alice",
		Period:                storageModels.PeriodDaily,
		IsActive:              true,
		ImportLimitMicroCents: estimated - 1,
		NextResetAt:           time.Now().Add(24 * time.Hour),
	}
	budget.UpdateKeys()
	storage.queryState.seededImportBudgets = map[string]*storageModels.ImportBudget{
		budget.PK + "#" + budget.SK: budget,
	}

	_, err = mut.CreateImport(round12AuthContext("alice"), model.CreateImportInput{
		Type: model.ImportTypeFollowers,
		File: graphql.Upload{
			Filename: "followers.json",
			File:     bytes.NewReader(data),
			Size:     int64(len(data)),
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "import budget limit exceeded")
}

func TestRound12MutationResolvers_ImportExport_CancelImport_Paths(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := &mutationResolver{resolver}

	// Auth required.
	_, err := mut.CancelImport(context.Background(), "import-1")
	require.Error(t, err)

	// Missing id.
	_, err = mut.CancelImport(round12AuthContext("alice"), "")
	require.Error(t, err)

	// Unauthorized cancellation (import belongs to alice per harness default).
	_, err = mut.CancelImport(round12AuthContext("bob"), "import-1")
	require.Error(t, err)

	// Terminal status returns current state.
	job, err := mut.CancelImport(round12AuthContext("alice"), "import-1")
	require.NoError(t, err)
	require.NotNil(t, job)

	// Non-terminal status triggers update flow (status controlled by harness).
	job, err = mut.CancelImport(round12AuthContext("alice"), "processing")
	require.NoError(t, err)
	require.NotNil(t, job)
}
