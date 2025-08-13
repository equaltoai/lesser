package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/services/bulk"
	"github.com/equaltoai/lesser/pkg/services/importexport"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
)

// Mock services implementing the handler interfaces
type MockBulkService struct {
	mock.Mock
}

func (m *MockBulkService) BulkFollow(ctx context.Context, cmd *bulk.FollowCommand) (*bulk.OperationResult, error) {
	args := m.Called(ctx, cmd)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*bulk.OperationResult), args.Error(1)
}

func (m *MockBulkService) BulkDeleteStatuses(ctx context.Context, cmd *bulk.DeleteStatusesCommand) (*bulk.OperationResult, error) {
	args := m.Called(ctx, cmd)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*bulk.OperationResult), args.Error(1)
}

func (m *MockBulkService) GetOperation(ctx context.Context, query *bulk.GetOperationQuery) (*bulk.OperationResult, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*bulk.OperationResult), args.Error(1)
}

type MockImportExportService struct {
	mock.Mock
}

func (m *MockImportExportService) CreateExport(ctx context.Context, cmd *importexport.CreateExportCommand) (*importexport.ExportResult, error) {
	args := m.Called(ctx, cmd)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*importexport.ExportResult), args.Error(1)
}

func (m *MockImportExportService) GetExport(ctx context.Context, query *importexport.GetExportQuery) (*importexport.ExportResult, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*importexport.ExportResult), args.Error(1)
}

func (m *MockImportExportService) ListExports(ctx context.Context, query *importexport.ListExportsQuery) (*importexport.ExportListResult, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*importexport.ExportListResult), args.Error(1)
}

func TestNewAsyncCommandHandler(t *testing.T) {
	bulkService := &MockBulkService{}
	importExportService := &MockImportExportService{}
	logger := zaptest.NewLogger(t)

	handler := NewAsyncCommandHandler(bulkService, importExportService, logger)

	assert.NotNil(t, handler)
	assert.Equal(t, bulkService, handler.bulkService)
	assert.Equal(t, importExportService, handler.importExportService)
	assert.NotNil(t, handler.BaseCommandHandler)
}

func TestAsyncCommandHandler_GetSupportedCommands(t *testing.T) {
	handler := NewAsyncCommandHandler(&MockBulkService{}, &MockImportExportService{}, zaptest.NewLogger(t))

	commands := handler.GetSupportedCommands()

	// Check that all expected commands are present
	expectedCommands := []string{
		streaming.CmdBulkFollow,
		streaming.CmdBulkUnfollow,
		streaming.CmdBulkMute,
		streaming.CmdBulkUnmute,
		streaming.CmdBulkBlock,
		streaming.CmdBulkUnblock,
		streaming.CmdBulkDeleteStatuses,
		streaming.CmdBulkListMembers,
		streaming.CmdGetBulkOperation,
		streaming.CmdCreateExport,
		streaming.CmdGetExport,
		streaming.CmdListExports,
		streaming.CmdCancelExport,
		streaming.CmdCreateImport,
		streaming.CmdGetImport,
		streaming.CmdListImports,
	}

	assert.Equal(t, len(expectedCommands), len(commands))
	for _, expectedCmd := range expectedCommands {
		assert.Contains(t, commands, expectedCmd)
	}
}

func TestAsyncCommandHandler_HandleBulkFollow_Success(t *testing.T) {
	bulkService := &MockBulkService{}
	handler := NewAsyncCommandHandler(bulkService, &MockImportExportService{}, zaptest.NewLogger(t))

	// Mock connection
	conn := &streaming.ConnectionInfo{
		ConnectionID:    "test-conn",
		UserID:          "testuser",
		Username:        "testuser",
		IsAuthenticated: true,
	}

	// Mock command
	cmd := &streaming.Command{
		ID:   "test-cmd-1",
		Type: streaming.CmdBulkFollow,
		Payload: map[string]interface{}{
			"account_ids": []interface{}{"user1", "user2", "user3"},
			"reblogs":     true,
			"notify":      false,
		},
	}

	// Mock service response
	mockOperation := &bulk.Operation{
		ID:        "bulk-op-123",
		Type:      "bulk_follow",
		Username:  "testuser",
		Status:    "processing",
		Total:     3,
		Processed: 0,
		Succeeded: 0,
		Failed:    0,
		StartedAt: time.Now(),
	}

	bulkService.On("BulkFollow", mock.Anything, mock.MatchedBy(func(cmd *bulk.FollowCommand) bool {
		return cmd.Username == "testuser" &&
			len(cmd.AccountIDs) == 3 &&
			cmd.AccountIDs[0] == "user1" &&
			cmd.Reblogs == true &&
			cmd.Notify == false
	})).Return(&bulk.OperationResult{Operation: mockOperation}, nil)

	// Execute
	response, err := handler.HandleCommand(context.Background(), conn, cmd)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "test-cmd-1", response.ID)
	assert.True(t, response.Success)
	assert.Equal(t, "command_result", response.Type)
	assert.NotNil(t, response.Data)

	// Verify service was called
	bulkService.AssertExpectations(t)
}

func TestAsyncCommandHandler_HandleBulkFollow_RequiresAuth(t *testing.T) {
	handler := NewAsyncCommandHandler(&MockBulkService{}, &MockImportExportService{}, zaptest.NewLogger(t))

	// Mock unauthenticated connection
	conn := &streaming.ConnectionInfo{
		ConnectionID:    "test-conn",
		UserID:          "",
		Username:        "",
		IsAuthenticated: false,
	}

	cmd := &streaming.Command{
		ID:   "test-cmd-1",
		Type: streaming.CmdBulkFollow,
		Payload: map[string]interface{}{
			"account_ids": []interface{}{"user1"},
		},
	}

	// Execute
	response, err := handler.HandleCommand(context.Background(), conn, cmd)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.False(t, response.Success)
	assert.Equal(t, "command_error", response.Type)
	assert.NotNil(t, response.Error)
	assert.Equal(t, "AUTHENTICATION_REQUIRED", response.Error.Code)
}

func TestAsyncCommandHandler_HandleBulkDeleteStatuses_Success(t *testing.T) {
	bulkService := &MockBulkService{}
	handler := NewAsyncCommandHandler(bulkService, &MockImportExportService{}, zaptest.NewLogger(t))

	conn := &streaming.ConnectionInfo{
		ConnectionID:    "test-conn",
		UserID:          "testuser",
		Username:        "testuser",
		IsAuthenticated: true,
	}

	cmd := &streaming.Command{
		ID:   "test-cmd-1",
		Type: streaming.CmdBulkDeleteStatuses,
		Payload: map[string]interface{}{
			"status_ids":  []interface{}{"status1", "status2"},
			"keep_pinned": true,
		},
	}

	mockOperation := &bulk.Operation{
		ID:        "bulk-op-456",
		Type:      "bulk_delete_statuses",
		Username:  "testuser",
		Status:    "processing",
		Total:     2,
		StartedAt: time.Now(),
	}

	bulkService.On("BulkDeleteStatuses", mock.Anything, mock.MatchedBy(func(cmd *bulk.DeleteStatusesCommand) bool {
		return cmd.Username == "testuser" &&
			len(cmd.StatusIDs) == 2 &&
			cmd.KeepPinned == true
	})).Return(&bulk.OperationResult{Operation: mockOperation}, nil)

	// Execute
	response, err := handler.HandleCommand(context.Background(), conn, cmd)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.True(t, response.Success)
	assert.Equal(t, "command_result", response.Type)

	bulkService.AssertExpectations(t)
}

func TestAsyncCommandHandler_HandleCreateExport_Success(t *testing.T) {
	importExportService := &MockImportExportService{}
	handler := NewAsyncCommandHandler(&MockBulkService{}, importExportService, zaptest.NewLogger(t))

	conn := &streaming.ConnectionInfo{
		ConnectionID:    "test-conn",
		UserID:          "testuser",
		Username:        "testuser",
		IsAuthenticated: true,
	}

	cmd := &streaming.Command{
		ID:   "test-cmd-1",
		Type: streaming.CmdCreateExport,
		Payload: map[string]interface{}{
			"type":          "archive",
			"format":        "activitypub",
			"include_media": true,
			"options": map[string]interface{}{
				"compress": "true",
			},
		},
	}

	mockExport := &models.Export{
		ID:           "export-123",
		Username:     "testuser",
		Type:         "archive",
		Format:       "activitypub",
		Status:       "pending",
		IncludeMedia: true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	importExportService.On("CreateExport", mock.Anything, mock.MatchedBy(func(cmd *importexport.CreateExportCommand) bool {
		return cmd.Username == "testuser" &&
			cmd.Type == "archive" &&
			cmd.Format == "activitypub" &&
			cmd.IncludeMedia == true
	})).Return(&importexport.ExportResult{Export: mockExport}, nil)

	// Execute
	response, err := handler.HandleCommand(context.Background(), conn, cmd)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.True(t, response.Success)
	assert.Equal(t, "command_result", response.Type)

	importExportService.AssertExpectations(t)
}

func TestAsyncCommandHandler_HandleGetExport_Success(t *testing.T) {
	importExportService := &MockImportExportService{}
	handler := NewAsyncCommandHandler(&MockBulkService{}, importExportService, zaptest.NewLogger(t))

	conn := &streaming.ConnectionInfo{
		ConnectionID:    "test-conn",
		UserID:          "testuser",
		Username:        "testuser",
		IsAuthenticated: true,
	}

	cmd := &streaming.Command{
		ID:   "test-cmd-1",
		Type: streaming.CmdGetExport,
		Payload: map[string]interface{}{
			"export_id": "export-123",
		},
	}

	mockExport := &models.Export{
		ID:       "export-123",
		Username: "testuser",
		Status:   "completed",
	}

	downloadURL := "https://example.com/download/export-123"
	expiresAt := time.Now().Add(24 * time.Hour)

	importExportService.On("GetExport", mock.Anything, mock.MatchedBy(func(query *importexport.GetExportQuery) bool {
		return query.ExportID == "export-123" && query.Username == "testuser"
	})).Return(&importexport.ExportResult{
		Export:      mockExport,
		DownloadURL: downloadURL,
		ExpiresAt:   &expiresAt,
	}, nil)

	// Execute
	response, err := handler.HandleCommand(context.Background(), conn, cmd)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.True(t, response.Success)
	assert.Equal(t, "command_result", response.Type)

	// Check that download URL is included
	if data, ok := response.Data["download_url"]; ok {
		assert.Equal(t, downloadURL, data)
	}

	importExportService.AssertExpectations(t)
}

func TestAsyncCommandHandler_HandleUnsupportedCommand(t *testing.T) {
	handler := NewAsyncCommandHandler(&MockBulkService{}, &MockImportExportService{}, zaptest.NewLogger(t))

	conn := &streaming.ConnectionInfo{
		ConnectionID:    "test-conn",
		UserID:          "testuser",
		Username:        "testuser",
		IsAuthenticated: true,
	}

	cmd := &streaming.Command{
		ID:      "test-cmd-1",
		Type:    "unsupported_command",
		Payload: map[string]interface{}{},
	}

	// Execute
	response, err := handler.HandleCommand(context.Background(), conn, cmd)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.False(t, response.Success)
	assert.Equal(t, "command_error", response.Type)
	assert.NotNil(t, response.Error)
	assert.Equal(t, "UNSUPPORTED_COMMAND", response.Error.Code)
}

func TestAsyncCommandHandler_HandleBulkFollow_ValidationError(t *testing.T) {
	handler := NewAsyncCommandHandler(&MockBulkService{}, &MockImportExportService{}, zaptest.NewLogger(t))

	conn := &streaming.ConnectionInfo{
		ConnectionID:    "test-conn",
		UserID:          "testuser",
		Username:        "testuser",
		IsAuthenticated: true,
	}

	// Command missing required account_ids field
	cmd := &streaming.Command{
		ID:      "test-cmd-1",
		Type:    streaming.CmdBulkFollow,
		Payload: map[string]interface{}{},
	}

	// Execute
	response, err := handler.HandleCommand(context.Background(), conn, cmd)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.False(t, response.Success)
	assert.Equal(t, "command_error", response.Type)
	assert.NotNil(t, response.Error)
	assert.Equal(t, "VALIDATION_ERROR", response.Error.Code)
}

func TestAsyncCommandHandler_HandleBulkFollow_EmptyAccountIDs(t *testing.T) {
	handler := NewAsyncCommandHandler(&MockBulkService{}, &MockImportExportService{}, zaptest.NewLogger(t))

	conn := &streaming.ConnectionInfo{
		ConnectionID:    "test-conn",
		UserID:          "testuser",
		Username:        "testuser",
		IsAuthenticated: true,
	}

	// Command with empty account_ids array
	cmd := &streaming.Command{
		ID:   "test-cmd-1",
		Type: streaming.CmdBulkFollow,
		Payload: map[string]interface{}{
			"account_ids": []interface{}{},
		},
	}

	// Execute
	response, err := handler.HandleCommand(context.Background(), conn, cmd)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.False(t, response.Success)
	assert.Equal(t, "command_error", response.Type)
	assert.NotNil(t, response.Error)
	assert.Equal(t, "VALIDATION_ERROR", response.Error.Code)
	assert.Contains(t, response.Error.Message, "No account IDs provided")
}