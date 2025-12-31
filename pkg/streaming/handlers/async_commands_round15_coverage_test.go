package handlers

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/services/bulk"
	"github.com/equaltoai/lesser/pkg/services/importexport"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func authedConn() *streaming.ConnectionInfo {
	return &streaming.ConnectionInfo{
		ConnectionID:    "test-conn",
		UserID:          "testuser",
		Username:        "testuser",
		IsAuthenticated: true,
	}
}

type countingRelationshipsService struct {
	unfollowCalls atomic.Int64
	unmuteCalls   atomic.Int64

	unfollowErr error
	unmuteErr   error
}

func (s *countingRelationshipsService) Unfollow(ctx context.Context, cmd *relationships.UnfollowCommand) (*relationships.RelationshipResult, error) {
	_ = ctx
	_ = cmd
	s.unfollowCalls.Add(1)
	return &relationships.RelationshipResult{}, s.unfollowErr
}

func (s *countingRelationshipsService) Unmute(ctx context.Context, cmd *relationships.UnmuteCommand) (*relationships.RelationshipResult, error) {
	_ = ctx
	_ = cmd
	s.unmuteCalls.Add(1)
	return &relationships.RelationshipResult{}, s.unmuteErr
}

func TestAsyncCommandHandler_HandleGetBulkOperation_Round15Coverage(t *testing.T) {
	bulkService := &MockBulkService{}
	handler := NewAsyncCommandHandler(bulkService, &MockImportExportService{}, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	op := &bulk.Operation{ID: "op1", Status: "processing", Total: 1, StartedAt: time.Now()}
	bulkService.On("GetOperation", mock.Anything, mock.MatchedBy(func(q *bulk.GetOperationQuery) bool {
		return q.OperationID == "op1" && q.Username == "testuser"
	})).Return(&bulk.OperationResult{Operation: op}, nil)

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:      "cmd1",
		Type:    streaming.CmdGetBulkOperation,
		Payload: map[string]interface{}{"operation_id": "op1"},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)
	bulkService.AssertExpectations(t)
}

func TestAsyncCommandHandler_ListExportsAndCancelExport_Round15Coverage(t *testing.T) {
	importExportService := &MockImportExportService{}
	handler := NewAsyncCommandHandler(&MockBulkService{}, importExportService, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	importExportService.On("ListExports", mock.Anything, mock.MatchedBy(func(q *importexport.ListExportsQuery) bool {
		return q.Username == "testuser" && q.Status == "completed" && q.Pagination.Limit == 2 && q.Pagination.Cursor == "c1"
	})).Return(&importexport.ExportListResult{
		Exports: []*models.Export{{ID: "e1", Username: "testuser", Status: "completed"}},
	}, nil)

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdListExports,
		Payload: map[string]interface{}{
			"status": "completed",
			"limit":  2,
			"cursor": "c1",
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	importExportService.On("CancelExport", mock.Anything, mock.Anything).Return(nil, errors.New("boom"))
	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:      "cmd2",
		Type:    streaming.CmdCancelExport,
		Payload: map[string]interface{}{"export_id": "e1"},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
}

func TestAsyncCommandHandler_BulkListMembers_Round15Coverage(t *testing.T) {
	bulkService := &MockBulkService{}
	handler := NewAsyncCommandHandler(bulkService, &MockImportExportService{}, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	invalidOpResp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdBulkListMembers,
		Payload: map[string]interface{}{
			"list_id":     "list1",
			"account_ids": []interface{}{"a1"},
			"operation":   "nope",
		},
	})
	require.NoError(t, err)
	require.False(t, invalidOpResp.Success)
	require.Equal(t, "VALIDATION_ERROR", invalidOpResp.Error.Code)

	bulkService.On("BulkListMembers", mock.Anything, mock.MatchedBy(func(cmd *bulk.ListMembersCommand) bool {
		return cmd.Username == "testuser" && cmd.ListID == "list1" && cmd.Operation == "add" && len(cmd.AccountIDs) == 2
	})).Return(nil, errors.New("boom"))

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd2",
		Type: streaming.CmdBulkListMembers,
		Payload: map[string]interface{}{
			"list_id":     "list1",
			"account_ids": []interface{}{"a1", "a2"},
			"operation":   "add",
		},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
	bulkService.AssertExpectations(t)
}

func TestAsyncCommandHandler_BulkContentOperationsAndExport_Round15Coverage(t *testing.T) {
	bulkService := &MockBulkService{}
	handler := NewAsyncCommandHandler(bulkService, &MockImportExportService{}, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	start := time.Now().Add(-time.Hour).UTC()
	end := time.Now().UTC()
	dateRange := map[string]interface{}{
		"start": start.Format(time.RFC3339),
		"end":   end.Format(time.RFC3339),
	}

	op := &bulk.Operation{ID: "op1", Status: "processing", Total: 1, StartedAt: time.Now()}
	bulkService.On("BulkDelete", mock.Anything, mock.MatchedBy(func(cmd *bulk.DeleteCommand) bool {
		return cmd.Username == "testuser" && cmd.ContentType == "status" && cmd.Permanent && cmd.DateRange != nil
	})).Return(&bulk.OperationResult{Operation: op}, nil)

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdBulkDelete,
		Payload: map[string]interface{}{
			"content_ids":  []interface{}{"c1"},
			"content_type": "status",
			"permanent":    true,
			"date_range":   dateRange,
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	bulkService.On("BulkArchive", mock.Anything, mock.MatchedBy(func(cmd *bulk.ArchiveCommand) bool {
		return cmd.Username == "testuser" && len(cmd.ContentIDs) == 1 && cmd.DateRange != nil
	})).Return(&bulk.OperationResult{Operation: op}, nil)

	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd2",
		Type: streaming.CmdBulkArchive,
		Payload: map[string]interface{}{
			"content_ids": []interface{}{"c1"},
			"date_range":  dateRange,
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	bulkService.On("BulkRestore", mock.Anything, mock.Anything).Return(nil, errors.New("restore failed"))
	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd3",
		Type: streaming.CmdBulkRestore,
		Payload: map[string]interface{}{
			"content_ids": []interface{}{"c1"},
		},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "BULK_RESTORE_FAILED", resp.Error.Code)

	invalidFormatResp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd4",
		Type: streaming.CmdBulkExport,
		Payload: map[string]interface{}{
			"content_ids": []interface{}{"c1"},
			"format":      "xml",
		},
	})
	require.NoError(t, err)
	require.False(t, invalidFormatResp.Success)
	require.Equal(t, "VALIDATION_ERROR", invalidFormatResp.Error.Code)

	bulkService.On("BulkExport", mock.Anything, mock.MatchedBy(func(cmd *bulk.ExportCommand) bool {
		return cmd.Username == "testuser" && cmd.Format == "json" && cmd.DateRange != nil && cmd.IncludeMedia
	})).Return(&bulk.OperationResult{Operation: op}, nil)

	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd5",
		Type: streaming.CmdBulkExport,
		Payload: map[string]interface{}{
			"content_ids":    []interface{}{"c1"},
			"format":         "json",
			"include_media":  true,
			"content_type":   "status",
			"date_range":     dateRange,
			"unknown_field":  "ignored",
			"another_unused": 1,
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)
	bulkService.AssertExpectations(t)
}

func TestAsyncCommandHandler_DeleteStatusesAndCreateExport_DateRangeParsing_Round15Coverage(t *testing.T) {
	bulkService := &MockBulkService{}
	importExportService := &MockImportExportService{}
	handler := NewAsyncCommandHandler(bulkService, importExportService, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	start := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	end := time.Now().UTC().Truncate(time.Second)
	dateRange := map[string]interface{}{
		"start": start.Format(time.RFC3339),
		"end":   end.Format(time.RFC3339),
	}

	op := &bulk.Operation{ID: "op1", Status: "processing", Total: 1, StartedAt: time.Now()}
	bulkService.On("BulkDeleteStatuses", mock.Anything, mock.MatchedBy(func(cmd *bulk.DeleteStatusesCommand) bool {
		return cmd.Username == "testuser" && cmd.DateRange != nil && cmd.DateRange.Start.Equal(start) && cmd.DateRange.End.Equal(end)
	})).Return(&bulk.OperationResult{Operation: op}, nil)

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdBulkDeleteStatuses,
		Payload: map[string]interface{}{
			"status_ids":  []interface{}{"s1"},
			"date_range":  dateRange,
			"keep_pinned": true,
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	mockExport := &models.Export{
		ID:        "e1",
		Username:  "testuser",
		Type:      "archive",
		Format:    "activitypub",
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	importExportService.On("CreateExport", mock.Anything, mock.MatchedBy(func(cmd *importexport.CreateExportCommand) bool {
		return cmd.Username == "testuser" && cmd.DateRange != nil && cmd.DateRange.Start.Equal(start) && cmd.DateRange.End.Equal(end)
	})).Return(&importexport.ExportResult{Export: mockExport}, nil)

	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd2",
		Type: streaming.CmdCreateExport,
		Payload: map[string]interface{}{
			"type":       "archive",
			"format":     "activitypub",
			"date_range": dateRange,
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)
}

func TestAsyncCommandHandler_ImportFlows_Round15Coverage(t *testing.T) {
	importExportService := &MockImportExportService{}
	handler := NewAsyncCommandHandler(&MockBulkService{}, importExportService, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	now := time.Now()
	completedAt := now.Add(-time.Minute)
	imp := &models.Import{
		ID:          "i1",
		Username:    "testuser",
		Type:        "followers",
		Mode:        "merge",
		Status:      "processing",
		CreatedAt:   now,
		UpdatedAt:   now,
		Progress:    1,
		Total:       2,
		ErrorCount:  1,
		CompletedAt: &completedAt,
	}

	importExportService.On("CreateImport", mock.Anything, mock.MatchedBy(func(cmd *importexport.CreateImportCommand) bool {
		return cmd.Username == "testuser" && cmd.Type == "followers" && cmd.Format == "json" && cmd.FileURL != "" && cmd.MergeStrategy == "merge"
	})).Return(&importexport.ImportResult{Import: imp}, nil)

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdCreateImport,
		Payload: map[string]interface{}{
			"type":           "followers",
			"format":         "json",
			"file_url":       "https://example.com/file.json",
			"merge_strategy": "merge",
			"options": map[string]interface{}{
				"foo": "bar",
			},
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	importExportService.On("GetImport", mock.Anything, mock.MatchedBy(func(q *importexport.GetImportQuery) bool {
		return q.ImportID == "i1" && q.Username == "testuser"
	})).Return(&importexport.ImportResult{Import: imp, Processed: 1, Skipped: 0, Failed: 1}, nil)

	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:      "cmd2",
		Type:    streaming.CmdGetImport,
		Payload: map[string]interface{}{"import_id": "i1"},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	importExportService.On("ListImports", mock.Anything, mock.MatchedBy(func(q *importexport.ListImportsQuery) bool {
		return q.Username == "testuser" && q.Status == "" && q.Pagination.Limit == 50
	})).Return(&importexport.ImportListResult{
		Imports:    []*models.Import{imp},
		NextCursor: "next",
		HasMore:    false,
	}, nil).Once()

	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:      "cmd3",
		Type:    streaming.CmdListImports,
		Payload: map[string]interface{}{},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Error path with logging.
	importExportService.On("ListImports", mock.Anything, mock.Anything).Return(nil, errors.New("boom")).Once()
	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:      "cmd4",
		Type:    streaming.CmdListImports,
		Payload: map[string]interface{}{},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
}

func TestAsyncCommandHandler_BulkMuteBlockUnblock_Round15Coverage(t *testing.T) {
	bulkService := &MockBulkService{}
	handler := NewAsyncCommandHandler(bulkService, &MockImportExportService{}, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	op := &bulk.Operation{ID: "op1", Status: "processing", Total: 1, StartedAt: time.Now()}
	bulkService.On("BulkMute", mock.Anything, mock.MatchedBy(func(cmd *bulk.MuteCommand) bool {
		return cmd.Username == "testuser" && cmd.Notifications == true && len(cmd.AccountIDs) == 1
	})).Return(&bulk.OperationResult{Operation: op}, nil)

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdBulkMute,
		Payload: map[string]interface{}{
			"account_ids":    []interface{}{"a1"},
			"notifications":  true,
			"ignored_field":  "x",
			"ignored_field2": 1,
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	bulkService.On("BulkBlock", mock.Anything, mock.MatchedBy(func(cmd *bulk.BlockCommand) bool {
		return cmd.Username == "testuser" && len(cmd.AccountIDs) == 1
	})).Return(&bulk.OperationResult{Operation: op}, nil)

	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:      "cmd2",
		Type:    streaming.CmdBulkBlock,
		Payload: map[string]interface{}{"account_ids": []interface{}{"a1"}},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	bulkService.On("BulkUnblock", mock.Anything, mock.MatchedBy(func(cmd *bulk.UnblockCommand) bool {
		return cmd.Username == "testuser" && len(cmd.AccountIDs) == 1
	})).Return(&bulk.OperationResult{Operation: op}, nil)

	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:      "cmd3",
		Type:    streaming.CmdBulkUnblock,
		Payload: map[string]interface{}{"account_ids": []interface{}{"a1"}},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	bulkService.AssertExpectations(t)
}

func TestAsyncCommandHandler_BulkUnfollowAndUnmute_AsyncProcessing_Round15Coverage(t *testing.T) {
	relSvc := &countingRelationshipsService{}
	publisher := streaming.NewMockPublisher()
	pubCount := publisher.(interface{ GetPublishedEventCount() int })

	handler := NewAsyncCommandHandler(&MockBulkService{}, &MockImportExportService{}, relSvc, publisher, zaptest.NewLogger(t))

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdBulkUnfollow,
		Payload: map[string]interface{}{
			"account_ids": []interface{}{"a1", "a2"},
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	require.Eventually(t, func() bool {
		return relSvc.unfollowCalls.Load() == 2 && pubCount.GetPublishedEventCount() >= 3
	}, time.Second, 10*time.Millisecond)

	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd2",
		Type: streaming.CmdBulkUnmute,
		Payload: map[string]interface{}{
			"account_ids": []interface{}{"a1"},
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	require.Eventually(t, func() bool {
		return relSvc.unmuteCalls.Load() == 1 && pubCount.GetPublishedEventCount() >= 6
	}, time.Second, 10*time.Millisecond)
}

func TestAsyncCommandHandler_processBulkRelationshipOperation_Round15Coverage(t *testing.T) {
	publisher := streaming.NewMockPublisher()
	logger := zaptest.NewLogger(t)
	handler := NewAsyncCommandHandler(&MockBulkService{}, &MockImportExportService{}, nil, publisher, logger)

	accountIDs := make([]string, 11)
	for i := range accountIDs {
		accountIDs[i] = "acct"
	}

	callCount := 0
	handler.processBulkRelationshipOperation(context.Background(), authedConn(), "op1", accountIDs, "unfollow", func(accountID string) error {
		callCount++
		if callCount == 2 {
			return errors.New("fail")
		}
		_ = accountID
		return nil
	})

	// One initial + one progress + one final update.
	pubCount := publisher.(interface{ GetPublishedEventCount() int })
	require.GreaterOrEqual(t, pubCount.GetPublishedEventCount(), 3)
}

func TestAsyncCommandHandler_ListImportsQueryPagination_Round15Coverage(t *testing.T) {
	importExportService := &MockImportExportService{}
	handler := NewAsyncCommandHandler(&MockBulkService{}, importExportService, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	importExportService.On("ListImports", mock.Anything, mock.MatchedBy(func(q *importexport.ListImportsQuery) bool {
		return q.Username == "testuser" && q.Status == "processing" && q.Pagination == (interfaces.PaginationOptions{Limit: 7, Cursor: "c"})
	})).Return(&importexport.ImportListResult{}, nil)

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdListImports,
		Payload: map[string]interface{}{
			"status": "processing",
			"limit":  7,
			"cursor": "c",
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)
}

func TestAsyncCommandHandler_CancelExport_SuccessAndValidationError_Round15Coverage(t *testing.T) {
	importExportService := &MockImportExportService{}
	handler := NewAsyncCommandHandler(&MockBulkService{}, importExportService, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	// Validation error: field exists but empty.
	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:      "cmd1",
		Type:    streaming.CmdCancelExport,
		Payload: map[string]interface{}{"export_id": ""},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "VALIDATION_ERROR", resp.Error.Code)

	export := &models.Export{ID: "e1", Username: "testuser", Status: "cancelled"}
	importExportService.On("CancelExport", mock.Anything, mock.MatchedBy(func(cmd *importexport.CancelExportCommand) bool {
		return cmd.ExportID == "e1" && cmd.Username == "testuser"
	})).Return(&importexport.ExportResult{Export: export}, nil)

	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:      "cmd2",
		Type:    streaming.CmdCancelExport,
		Payload: map[string]interface{}{"export_id": "e1"},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)
}

func TestAsyncCommandHandler_GetImport_ServiceError_Round15Coverage(t *testing.T) {
	importExportService := &MockImportExportService{}
	handler := NewAsyncCommandHandler(&MockBulkService{}, importExportService, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	importExportService.On("GetImport", mock.Anything, mock.Anything).Return(nil, errors.New("boom"))

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:      "cmd1",
		Type:    streaming.CmdGetImport,
		Payload: map[string]interface{}{"import_id": "i1"},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
}

func TestAsyncCommandHandler_CreateImport_ValidationAndServiceError_Round15Coverage(t *testing.T) {
	importExportService := &MockImportExportService{}
	handler := NewAsyncCommandHandler(&MockBulkService{}, importExportService, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	// Validation error: merge_strategy required, but empty.
	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdCreateImport,
		Payload: map[string]interface{}{
			"type":           "followers",
			"format":         "json",
			"file_url":       "https://example.com/file.json",
			"merge_strategy": "",
		},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "VALIDATION_ERROR", resp.Error.Code)

	// Service error with options type mismatch path.
	importExportService.On("CreateImport", mock.Anything, mock.Anything).Return(nil, errors.New("boom"))

	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd2",
		Type: streaming.CmdCreateImport,
		Payload: map[string]interface{}{
			"type":           "followers",
			"format":         "json",
			"file_url":       "https://example.com/file.json",
			"merge_strategy": "merge",
			"options":        "not-a-map",
		},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
}

func TestAsyncCommandHandler_BulkListMembers_Success_Round15Coverage(t *testing.T) {
	bulkService := &MockBulkService{}
	handler := NewAsyncCommandHandler(bulkService, &MockImportExportService{}, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	op := &bulk.Operation{ID: "op1", Status: "processing", Total: 2, StartedAt: time.Now()}
	bulkService.On("BulkListMembers", mock.Anything, mock.Anything).Return(&bulk.OperationResult{Operation: op}, nil)

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdBulkListMembers,
		Payload: map[string]interface{}{
			"list_id":     "list1",
			"account_ids": []interface{}{"a1", "a2"},
			"operation":   "remove",
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)
}

func TestAsyncCommandHandler_ListExports_Error_Round15Coverage(t *testing.T) {
	importExportService := &MockImportExportService{}
	handler := NewAsyncCommandHandler(&MockBulkService{}, importExportService, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	importExportService.On("ListExports", mock.Anything, mock.Anything).Return(nil, errors.New("boom"))

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:      "cmd1",
		Type:    streaming.CmdListExports,
		Payload: map[string]interface{}{},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "LIST_EXPORTS_FAILED", resp.Error.Code)
}

func TestAsyncCommandHandler_BulkExport_ValidationErrors_Round15Coverage(t *testing.T) {
	handler := NewAsyncCommandHandler(&MockBulkService{}, &MockImportExportService{}, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	// Invalid content_ids (empty list).
	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdBulkExport,
		Payload: map[string]interface{}{
			"content_ids": []interface{}{},
			"format":      "json",
		},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "VALIDATION_ERROR", resp.Error.Code)

	// Invalid format (present but empty).
	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd2",
		Type: streaming.CmdBulkExport,
		Payload: map[string]interface{}{
			"content_ids": []interface{}{"c1"},
			"format":      "",
		},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
}

func TestAsyncCommandHandler_BulkContentOperation_ValidationErrors_Round15Coverage(t *testing.T) {
	handler := NewAsyncCommandHandler(&MockBulkService{}, &MockImportExportService{}, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdBulkArchive,
		Payload: map[string]interface{}{
			"content_ids": []interface{}{},
		},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "VALIDATION_ERROR", resp.Error.Code)

	tooMany := make([]interface{}, 101)
	for i := range tooMany {
		tooMany[i] = "c"
	}

	resp, err = handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd2",
		Type: streaming.CmdBulkRestore,
		Payload: map[string]interface{}{
			"content_ids": tooMany,
		},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
}

func TestAsyncCommandHandler_BulkDeleteStatuses_ValidationError_Round15Coverage(t *testing.T) {
	handler := NewAsyncCommandHandler(&MockBulkService{}, &MockImportExportService{}, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdBulkDeleteStatuses,
		Payload: map[string]interface{}{
			"status_ids": []interface{}{""},
		},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
}

func TestAsyncCommandHandler_BulkFollow_InvalidAccountIDs_Round15Coverage(t *testing.T) {
	handler := NewAsyncCommandHandler(&MockBulkService{}, &MockImportExportService{}, nil, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	resp, err := handler.HandleCommand(context.Background(), authedConn(), &streaming.Command{
		ID:   "cmd1",
		Type: streaming.CmdBulkFollow,
		Payload: map[string]interface{}{
			"account_ids": []interface{}{""},
		},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
}

func TestAsyncCommandHandler_AuthAndValidationEarlyReturns_Round15Coverage(t *testing.T) {
	var nilRelSvc *relationships.Service
	var relSvc RelationshipsService = nilRelSvc // typed-nil interface value

	handler := NewAsyncCommandHandler(&MockBulkService{}, &MockImportExportService{}, relSvc, streaming.NewMockPublisher(), zaptest.NewLogger(t))

	unauth := &streaming.ConnectionInfo{
		ConnectionID:    "test-conn",
		UserID:          "",
		Username:        "",
		IsAuthenticated: false,
	}

	cmdTypes := []string{
		// Direct RequireAuth branches in this file.
		streaming.CmdBulkDeleteStatuses,
		streaming.CmdCreateExport,
		streaming.CmdGetExport,
		streaming.CmdListExports,
		streaming.CmdCancelExport,
		streaming.CmdCreateImport,
		streaming.CmdGetImport,
		streaming.CmdListImports,
		streaming.CmdBulkDelete,
		streaming.CmdBulkArchive,
		streaming.CmdBulkRestore,
		streaming.CmdBulkExport,
		streaming.CmdBulkListMembers,

		// ValidateBulkAccountCommand early-return branches in this file.
		streaming.CmdBulkUnfollow,
		streaming.CmdBulkUnmute,
		streaming.CmdBulkMute,
		streaming.CmdBulkBlock,
		streaming.CmdBulkUnblock,
	}

	for i, typ := range cmdTypes {
		resp, err := handler.HandleCommand(context.Background(), unauth, &streaming.Command{
			ID:      fmt.Sprintf("cmd-%d", i),
			Type:    typ,
			Payload: map[string]interface{}{},
		})
		require.NoError(t, err)
		require.False(t, resp.Success)
		require.Equal(t, "AUTHENTICATION_REQUIRED", resp.Error.Code)
	}
}
