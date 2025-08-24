// Package handlers provides WebSocket command handlers for async operations
package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/bulk"
	"github.com/equaltoai/lesser/pkg/services/importexport"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// Constants for operation statuses
const (
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
)

// BulkService defines the interface for bulk operations
type BulkService interface {
	BulkFollow(ctx context.Context, cmd *bulk.FollowCommand) (*bulk.OperationResult, error)
	BulkDeleteStatuses(ctx context.Context, cmd *bulk.DeleteStatusesCommand) (*bulk.OperationResult, error)
	GetOperation(ctx context.Context, query *bulk.GetOperationQuery) (*bulk.OperationResult, error)

	// Bulk Content Management Operations
	BulkDelete(ctx context.Context, cmd *bulk.DeleteCommand) (*bulk.OperationResult, error)
	BulkArchive(ctx context.Context, cmd *bulk.ArchiveCommand) (*bulk.OperationResult, error)
	BulkRestore(ctx context.Context, cmd *bulk.RestoreCommand) (*bulk.OperationResult, error)
	BulkExport(ctx context.Context, cmd *bulk.ExportCommand) (*bulk.OperationResult, error)
	BulkListMembers(ctx context.Context, cmd *bulk.ListMembersCommand) (*bulk.OperationResult, error)

	// Bulk Moderation Operations
	BulkUnblock(ctx context.Context, cmd *bulk.UnblockCommand) (*bulk.OperationResult, error)
	BulkMute(ctx context.Context, cmd *bulk.MuteCommand) (*bulk.OperationResult, error)
	BulkBlock(ctx context.Context, cmd *bulk.BlockCommand) (*bulk.OperationResult, error)
}

// ImportExportService defines the interface for import/export operations
type ImportExportService interface {
	CreateExport(ctx context.Context, cmd *importexport.CreateExportCommand) (*importexport.ExportResult, error)
	GetExport(ctx context.Context, query *importexport.GetExportQuery) (*importexport.ExportResult, error)
	ListExports(ctx context.Context, query *importexport.ListExportsQuery) (*importexport.ExportListResult, error)
	CancelExport(ctx context.Context, cmd *importexport.CancelExportCommand) (*importexport.ExportResult, error)
	CreateImport(ctx context.Context, cmd *importexport.CreateImportCommand) (*importexport.ImportResult, error)
	GetImport(ctx context.Context, query *importexport.GetImportQuery) (*importexport.ImportResult, error)
	ListImports(ctx context.Context, query *importexport.ListImportsQuery) (*importexport.ImportListResult, error)
}

// AsyncCommandHandler handles WebSocket commands for async operations (bulk ops, import/export)
type AsyncCommandHandler struct {
	*streaming.BaseCommandHandler
	bulkService          BulkService
	importExportService  ImportExportService
	relationshipsService *relationships.Service
	publisher            streaming.Publisher
	logger               *zap.Logger
}

// NewAsyncCommandHandler creates a new async command handler
func NewAsyncCommandHandler(
	bulkService BulkService,
	importExportService ImportExportService,
	relationshipsService *relationships.Service,
	publisher streaming.Publisher,
	logger *zap.Logger,
) *AsyncCommandHandler {
	return &AsyncCommandHandler{
		BaseCommandHandler:   streaming.NewBaseCommandHandler(logger),
		bulkService:          bulkService,
		importExportService:  importExportService,
		relationshipsService: relationshipsService,
		publisher:            publisher,
		logger:               logger,
	}
}

// GetSupportedCommands returns the list of commands this handler supports
func (ach *AsyncCommandHandler) GetSupportedCommands() []string {
	return []string{
		// Bulk operation commands
		streaming.CmdBulkFollow,
		streaming.CmdBulkUnfollow,
		streaming.CmdBulkMute,
		streaming.CmdBulkUnmute,
		streaming.CmdBulkBlock,
		streaming.CmdBulkUnblock,
		streaming.CmdBulkDeleteStatuses,
		streaming.CmdBulkListMembers,
		streaming.CmdGetBulkOperation,
		// Bulk Content Management commands
		streaming.CmdBulkDelete,
		streaming.CmdBulkArchive,
		streaming.CmdBulkRestore,
		streaming.CmdBulkExport,
		// Import/Export commands
		streaming.CmdCreateExport,
		streaming.CmdGetExport,
		streaming.CmdListExports,
		streaming.CmdCancelExport,
		streaming.CmdCreateImport,
		streaming.CmdGetImport,
		streaming.CmdListImports,
	}
}

// HandleCommand processes async-related WebSocket commands
func (ach *AsyncCommandHandler) HandleCommand(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	switch cmd.Type {
	// Bulk operation commands
	case streaming.CmdBulkFollow:
		return ach.handleBulkFollow(ctx, conn, cmd)
	case streaming.CmdBulkUnfollow:
		return ach.handleBulkUnfollow(ctx, conn, cmd)
	case streaming.CmdBulkMute:
		return ach.handleBulkMute(ctx, conn, cmd)
	case streaming.CmdBulkUnmute:
		return ach.handleBulkUnmute(ctx, conn, cmd)
	case streaming.CmdBulkBlock:
		return ach.handleBulkBlock(ctx, conn, cmd)
	case streaming.CmdBulkUnblock:
		return ach.handleBulkUnblock(ctx, conn, cmd)
	case streaming.CmdBulkDeleteStatuses:
		return ach.handleBulkDeleteStatuses(ctx, conn, cmd)
	case streaming.CmdBulkListMembers:
		return ach.handleBulkListMembers(ctx, conn, cmd)
	case streaming.CmdGetBulkOperation:
		return ach.handleGetBulkOperation(ctx, conn, cmd)
	// Bulk Content Management commands
	case streaming.CmdBulkDelete:
		return ach.handleBulkDelete(ctx, conn, cmd)
	case streaming.CmdBulkArchive:
		return ach.handleBulkArchive(ctx, conn, cmd)
	case streaming.CmdBulkRestore:
		return ach.handleBulkRestore(ctx, conn, cmd)
	case streaming.CmdBulkExport:
		return ach.handleBulkExport(ctx, conn, cmd)
	// Import/Export commands
	case streaming.CmdCreateExport:
		return ach.handleCreateExport(ctx, conn, cmd)
	case streaming.CmdGetExport:
		return ach.handleGetExport(ctx, conn, cmd)
	case streaming.CmdListExports:
		return ach.handleListExports(ctx, conn, cmd)
	case streaming.CmdCancelExport:
		return ach.handleCancelExport(ctx, conn, cmd)
	case streaming.CmdCreateImport:
		return ach.handleCreateImport(ctx, conn, cmd)
	case streaming.CmdGetImport:
		return ach.handleGetImport(ctx, conn, cmd)
	case streaming.CmdListImports:
		return ach.handleListImports(ctx, conn, cmd)
	default:
		return ach.CreateErrorResponse(cmd.ID, "UNSUPPORTED_COMMAND",
			"Unsupported async command", fmt.Sprintf("Command %s not supported by async handler", cmd.Type)), nil
	}
}

// Bulk Operation Command Handlers

// handleBulkFollow handles bulk follow operations
func (ach *AsyncCommandHandler) handleBulkFollow(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"account_ids"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	accountIDs := ach.GetStringSlice(cmd.Payload, "account_ids")
	if err := common.ValidateEntityIDsList(accountIDs, "account"); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid account_ids", err.Error()), nil
	}

	reblogs := ach.GetBool(cmd.Payload, "reblogs", true)
	notify := ach.GetBool(cmd.Payload, "notify", false)
	languages := ach.GetStringSlice(cmd.Payload, "languages")

	bulkCmd := &bulk.FollowCommand{
		Username:   conn.Username,
		AccountIDs: accountIDs,
		Reblogs:    reblogs,
		Notify:     notify,
		Languages:  languages,
	}

	result, err := ach.bulkService.BulkFollow(ctx, bulkCmd)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "BULK_FOLLOW_FAILED",
			"Failed to start bulk follow operation", err.Error()), nil
	}

	data, err := ach.ConvertToJSON(result.Operation)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

// handleBulkUnfollow handles bulk unfollow operations
func (ach *AsyncCommandHandler) handleBulkUnfollow(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	accountIDs, validationErr := ach.ValidateBulkAccountCommand(conn, cmd, streaming.DefaultBulkAccountConfig())
	if validationErr != nil {
		return validationErr, nil
	}

	// Start async processing
	go ach.processBulkUnfollow(ctx, conn, cmd.ID, accountIDs)

	// Return immediate response indicating processing started
	return ach.CreateBulkOperationResponse(cmd.ID, cmd.ID, "processing", len(accountIDs), "Bulk unfollow operation started"), nil
}

// handleBulkDeleteStatuses handles bulk status deletion
func (ach *AsyncCommandHandler) handleBulkDeleteStatuses(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"status_ids"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	statusIDs := ach.GetStringSlice(cmd.Payload, "status_ids")
	if err := common.ValidateEntityIDsList(statusIDs, "status"); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid status_ids", err.Error()), nil
	}

	keepPinned := ach.GetBool(cmd.Payload, "keep_pinned", true)

	// Parse date range if provided
	var dateRange *bulk.DateRange
	if dateRangeData, exists := cmd.Payload["date_range"].(map[string]interface{}); exists {
		if startStr := ach.GetString(dateRangeData, "start", ""); startStr != "" {
			if start, err := time.Parse(time.RFC3339, startStr); err == nil {
				if endStr := ach.GetString(dateRangeData, "end", ""); endStr != "" {
					if end, err := time.Parse(time.RFC3339, endStr); err == nil {
						dateRange = &bulk.DateRange{Start: start, End: end}
					}
				}
			}
		}
	}

	bulkCmd := &bulk.DeleteStatusesCommand{
		Username:   conn.Username,
		StatusIDs:  statusIDs,
		DateRange:  dateRange,
		KeepPinned: keepPinned,
	}

	result, err := ach.bulkService.BulkDeleteStatuses(ctx, bulkCmd)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "BULK_DELETE_FAILED",
			"Failed to start bulk delete operation", err.Error()), nil
	}

	data, err := ach.ConvertToJSON(result.Operation)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

// handleGetBulkOperation handles retrieving bulk operation status
func (ach *AsyncCommandHandler) handleGetBulkOperation(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	config := &streaming.CommandHandlerConfig{
		RequiredFields:  []string{"operation_id"},
		ParameterName:   "operation_id",
		ErrorCodePrefix: "GET_OPERATION",
		OperationName:   "get bulk operation",
		ResultExtractor: func(result interface{}) interface{} {
			return result.(*bulk.OperationResult).Operation
		},
		ServiceCall: func(ctx context.Context, conn *streaming.ConnectionInfo, operationID string) (interface{}, error) {
			query := &bulk.GetOperationQuery{
				OperationID: operationID,
				Username:    conn.Username,
			}
			return ach.bulkService.GetOperation(ctx, query)
		},
	}

	return ach.ExecuteStandardCommandFlow(ctx, conn, cmd, config)
}

// Import/Export Command Handlers

// handleCreateExport handles creating a new export
func (ach *AsyncCommandHandler) handleCreateExport(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"type", "format"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	exportType := ach.GetString(cmd.Payload, "type", "")
	format := ach.GetString(cmd.Payload, "format", "")
	includeMedia := ach.GetBool(cmd.Payload, "include_media", false)

	// Parse options map
	options := make(map[string]string)
	if optionsData, exists := cmd.Payload["options"].(map[string]interface{}); exists {
		for k, v := range optionsData {
			if strValue, ok := v.(string); ok {
				options[k] = strValue
			}
		}
	}

	// Parse date range if provided
	var dateRange *importexport.DateRange
	if dateRangeData, exists := cmd.Payload["date_range"].(map[string]interface{}); exists {
		if startStr := ach.GetString(dateRangeData, "start", ""); startStr != "" {
			if start, err := time.Parse(time.RFC3339, startStr); err == nil {
				if endStr := ach.GetString(dateRangeData, "end", ""); endStr != "" {
					if end, err := time.Parse(time.RFC3339, endStr); err == nil {
						dateRange = &importexport.DateRange{Start: start, End: end}
					}
				}
			}
		}
	}

	createCmd := &importexport.CreateExportCommand{
		Username:     conn.Username,
		Type:         exportType,
		Format:       format,
		IncludeMedia: includeMedia,
		DateRange:    dateRange,
		Options:      options,
		RequestedBy:  conn.Username,
	}

	result, err := ach.importExportService.CreateExport(ctx, createCmd)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "CREATE_EXPORT_FAILED",
			"Failed to create export", err.Error()), nil
	}

	data, err := ach.ConvertToJSON(result.Export)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

// handleGetExport handles retrieving export status
func (ach *AsyncCommandHandler) handleGetExport(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"export_id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	exportID := ach.GetString(cmd.Payload, "export_id", "")

	query := &importexport.GetExportQuery{
		ExportID: exportID,
		Username: conn.Username,
	}

	result, err := ach.importExportService.GetExport(ctx, query)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "GET_EXPORT_FAILED",
			"Failed to get export", err.Error()), nil
	}

	// Include download URL and expiry if available
	data := map[string]interface{}{
		"export": result.Export,
	}
	if result.DownloadURL != "" {
		data["download_url"] = result.DownloadURL
	}
	if result.ExpiresAt != nil {
		data["expires_at"] = result.ExpiresAt
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

// handleListExports handles listing exports for a user
func (ach *AsyncCommandHandler) handleListExports(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	status := ach.GetString(cmd.Payload, "status", "")
	limit := ach.GetInt(cmd.Payload, "limit", 20)
	cursor := ach.GetString(cmd.Payload, "cursor", "")

	query := &importexport.ListExportsQuery{
		Username: conn.Username,
		Status:   status,
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: cursor,
		},
	}

	result, err := ach.importExportService.ListExports(ctx, query)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "LIST_EXPORTS_FAILED",
			"Failed to list exports", err.Error()), nil
	}

	data, err := ach.ConvertToJSON(result)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

// Bulk Social Action Command Handlers

func (ach *AsyncCommandHandler) handleBulkMute(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	accountIDs, validationErr := ach.ValidateBulkAccountCommand(conn, cmd, streaming.DefaultBulkAccountConfig())
	if validationErr != nil {
		return validationErr, nil
	}

	// Optional parameters
	hideNotifications := ach.GetBool(cmd.Payload, "notifications", false)

	// Create bulk mute command
	bulkCmd := &bulk.MuteCommand{
		Username:      conn.UserID,
		AccountIDs:    accountIDs,
		Notifications: hideNotifications,
	}

	// Execute bulk mute operation using the bulk service
	result, err := ach.bulkService.BulkMute(ctx, bulkCmd)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "BULK_OPERATION_ERROR", "Failed to start bulk mute operation", err.Error()), nil
	}

	// Return operation started response
	data := map[string]interface{}{
		"operation_id": result.Operation.ID,
		"status":       result.Operation.Status,
		"total":        result.Operation.Total,
		"message":      "Bulk mute operation started",
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

func (ach *AsyncCommandHandler) handleBulkUnmute(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	accountIDs, validationErr := ach.ValidateBulkAccountCommand(conn, cmd, streaming.DefaultBulkAccountConfig())
	if validationErr != nil {
		return validationErr, nil
	}

	// Start async processing
	go ach.processBulkUnmute(ctx, conn, cmd.ID, accountIDs)

	// Return immediate response indicating processing started
	return ach.CreateBulkOperationResponse(cmd.ID, cmd.ID, "processing", len(accountIDs), "Bulk unmute operation started"), nil
}

func (ach *AsyncCommandHandler) handleBulkBlock(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	accountIDs, validationErr := ach.ValidateBulkAccountCommand(conn, cmd, streaming.DefaultBulkAccountConfig())
	if validationErr != nil {
		return validationErr, nil
	}

	// Create bulk block command
	bulkCmd := &bulk.BlockCommand{
		Username:   conn.UserID,
		AccountIDs: accountIDs,
	}

	// Execute bulk block operation using the bulk service
	result, err := ach.bulkService.BulkBlock(ctx, bulkCmd)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "BULK_OPERATION_ERROR", "Failed to start bulk block operation", err.Error()), nil
	}

	// Return operation started response
	data := map[string]interface{}{
		"operation_id": result.Operation.ID,
		"status":       result.Operation.Status,
		"total":        result.Operation.Total,
		"message":      "Bulk block operation started",
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

func (ach *AsyncCommandHandler) handleBulkUnblock(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	accountIDs, validationErr := ach.ValidateBulkAccountCommand(conn, cmd, streaming.DefaultBulkAccountConfig())
	if validationErr != nil {
		return validationErr, nil
	}

	// Create bulk unblock command
	bulkCmd := &bulk.UnblockCommand{
		Username:   conn.UserID,
		AccountIDs: accountIDs,
	}

	// Execute bulk unblock operation using the bulk service
	result, err := ach.bulkService.BulkUnblock(ctx, bulkCmd)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "BULK_OPERATION_ERROR", "Failed to start bulk unblock operation", err.Error()), nil
	}

	// Return operation started response
	data := map[string]interface{}{
		"operation_id": result.Operation.ID,
		"status":       result.Operation.Status,
		"total":        result.Operation.Total,
		"message":      "Bulk unblock operation started",
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

func (ach *AsyncCommandHandler) handleBulkListMembers(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"list_id", "account_ids", "operation"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	// Extract parameters from payload
	listID := ach.GetString(cmd.Payload, "list_id", "")
	accountIDs := ach.GetStringSlice(cmd.Payload, "account_ids")
	operation := ach.GetString(cmd.Payload, "operation", "")

	// Validate operation type
	if operation != "add" && operation != "remove" {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid operation", "Operation must be 'add' or 'remove'"), nil
	}

	// Validate account IDs
	if err := common.ValidateEntityIDsList(accountIDs, "account"); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid account_ids", err.Error()), nil
	}
	if err := common.ValidateIntRange("account_ids_count", len(accountIDs), 1, 100); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid account_ids count", err.Error()), nil
	}

	// Create command for bulk service
	listMembersCmd := &bulk.ListMembersCommand{
		Username:   conn.Username, // Using Username as required by the command
		ListID:     listID,
		AccountIDs: accountIDs,
		Operation:  operation,
	}

	// Call the bulk service
	result, err := ach.bulkService.BulkListMembers(ctx, listMembersCmd)
	if err != nil {
		ach.logger.Error("bulk list members operation failed",
			zap.String("list_id", listID),
			zap.String("operation", operation),
			zap.Strings("account_ids", accountIDs),
			zap.Error(err))
		return ach.CreateErrorResponse(cmd.ID, "INTERNAL_ERROR", "Bulk operation failed", err.Error()), nil
	}

	// Return operation result
	data := map[string]interface{}{
		"operation_id": result.Operation.ID,
		"status":       result.Operation.Status,
		"total":        result.Operation.Total,
		"processed":    result.Operation.Processed,
		"successful":   result.Operation.Succeeded,
		"failed":       result.Operation.Failed,
		"operation":    operation,
		"list_id":      listID,
		"message":      fmt.Sprintf("Bulk %s list members operation started", operation),
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

func (ach *AsyncCommandHandler) handleCancelExport(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"export_id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	// Extract export ID from payload
	exportID := ach.GetString(cmd.Payload, "export_id", "")
	if err := common.ValidateRequiredParam("export_id", exportID); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid export_id", err.Error()), nil
	}

	// Create cancel command for import/export service
	cancelCmd := &importexport.CancelExportCommand{
		ExportID: exportID,
		Username: conn.Username,
	}

	// Call the import/export service
	result, err := ach.importExportService.CancelExport(ctx, cancelCmd)
	if err != nil {
		ach.logger.Error("cancel export operation failed",
			zap.String("export_id", exportID),
			zap.String("username", conn.Username),
			zap.Error(err))
		return ach.CreateErrorResponse(cmd.ID, "INTERNAL_ERROR", "Cancel export failed", err.Error()), nil
	}

	// Return cancellation result
	data := map[string]interface{}{
		"export_id": exportID,
		"status":    result.Export.Status,
		"message":   "Export cancelled successfully",
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

func (ach *AsyncCommandHandler) handleCreateImport(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"type", "format", "file_url", "merge_strategy"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	// Extract parameters from payload
	importType := ach.GetString(cmd.Payload, "type", "")
	format := ach.GetString(cmd.Payload, "format", "")
	fileURL := ach.GetString(cmd.Payload, "file_url", "")
	mergeStrategy := ach.GetString(cmd.Payload, "merge_strategy", "")

	// Get optional options
	optionsInterface, hasOptions := cmd.Payload["options"]
	options := make(map[string]string)
	if hasOptions {
		if optionsMap, ok := optionsInterface.(map[string]interface{}); ok {
			for k, v := range optionsMap {
				if strVal, ok := v.(string); ok {
					options[k] = strVal
				}
			}
		}
	}

	// Validate parameters
	if err := common.ValidateRequiredParam("import_type", importType); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid import_type", err.Error()), nil
	}
	if err := common.ValidateRequiredParam("format", format); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid format", err.Error()), nil
	}
	if err := common.ValidateRequiredParam("file_url", fileURL); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid file_url", err.Error()), nil
	}
	if err := common.ValidateRequiredParam("merge_strategy", mergeStrategy); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid merge_strategy", err.Error()), nil
	}

	// Create import command for import/export service
	importCmd := &importexport.CreateImportCommand{
		Username:      conn.Username,
		Type:          importType,
		Format:        format,
		FileURL:       fileURL,
		Options:       options,
		MergeStrategy: mergeStrategy,
		RequestedBy:   conn.Username,
	}

	// Call the import/export service
	result, err := ach.importExportService.CreateImport(ctx, importCmd)
	if err != nil {
		ach.logger.Error("create import operation failed",
			zap.String("type", importType),
			zap.String("format", format),
			zap.String("username", conn.Username),
			zap.Error(err))
		return ach.CreateErrorResponse(cmd.ID, "INTERNAL_ERROR", "Create import failed", err.Error()), nil
	}

	// Return import creation result
	data := map[string]interface{}{
		"import_id":      result.Import.ID,
		"type":           result.Import.Type,
		"format":         format, // Use format from command since model doesn't store it
		"status":         result.Import.Status,
		"merge_strategy": result.Import.Mode, // Use Mode field
		"message":        "Import created successfully",
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

func (ach *AsyncCommandHandler) handleGetImport(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"import_id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	// Extract import ID from payload
	importID := ach.GetString(cmd.Payload, "import_id", "")
	if err := common.ValidateRequiredParam("import_id", importID); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid import_id", err.Error()), nil
	}

	// Create get query for import/export service
	getQuery := &importexport.GetImportQuery{
		ImportID: importID,
		Username: conn.Username,
	}

	// Call the import/export service
	result, err := ach.importExportService.GetImport(ctx, getQuery)
	if err != nil {
		ach.logger.Error("get import operation failed",
			zap.String("import_id", importID),
			zap.String("username", conn.Username),
			zap.Error(err))
		return ach.CreateErrorResponse(cmd.ID, "INTERNAL_ERROR", "Get import failed", err.Error()), nil
	}

	// Return import details
	data := map[string]interface{}{
		"import_id":  result.Import.ID,
		"type":       result.Import.Type,
		"status":     result.Import.Status,
		"mode":       result.Import.Mode,
		"processed":  result.Processed,
		"skipped":    result.Skipped,
		"failed":     result.Failed,
		"created_at": result.Import.CreatedAt,
		"updated_at": result.Import.UpdatedAt,
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

func (ach *AsyncCommandHandler) handleListImports(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Extract optional parameters from payload
	status := ach.GetString(cmd.Payload, "status", "")
	limit := ach.GetInt(cmd.Payload, "limit", 50) // Default limit
	cursor := ach.GetString(cmd.Payload, "cursor", "")

	// Create list query for import/export service
	listQuery := &importexport.ListImportsQuery{
		Username: conn.Username,
		Status:   status,
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: cursor,
		},
	}

	// Call the import/export service
	result, err := ach.importExportService.ListImports(ctx, listQuery)
	if err != nil {
		ach.logger.Error("list imports operation failed",
			zap.String("username", conn.Username),
			zap.String("status", status),
			zap.Error(err))
		return ach.CreateErrorResponse(cmd.ID, "INTERNAL_ERROR", "List imports failed", err.Error()), nil
	}

	// Convert imports to response format
	imports := make([]map[string]interface{}, len(result.Imports))
	for i, imp := range result.Imports {
		imports[i] = map[string]interface{}{
			"import_id":   imp.ID,
			"type":        imp.Type,
			"status":      imp.Status,
			"mode":        imp.Mode,
			"created_at":  imp.CreatedAt,
			"updated_at":  imp.UpdatedAt,
			"progress":    imp.Progress,
			"total":       imp.Total,
			"error_count": imp.ErrorCount,
		}
		if imp.CompletedAt != nil {
			imports[i]["completed_at"] = *imp.CompletedAt
		}
	}

	// Return import list
	data := map[string]interface{}{
		"imports":     imports,
		"next_cursor": result.NextCursor,
		"has_more":    result.HasMore,
		"total":       len(imports),
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

// ===== Bulk Social Action Processing Functions =====

// processBulkRelationshipOperation processes bulk relationship operations with progress tracking
func (ach *AsyncCommandHandler) processBulkRelationshipOperation(
	_ context.Context,
	conn *streaming.ConnectionInfo,
	operationID string,
	accountIDs []string,
	operationName string,
	processAccount func(accountID string) error,
) {
	tracker := streaming.NewBulkProcessingTracker(len(accountIDs))
	config := streaming.DefaultBulkProcessingConfig()
	progressHelper := streaming.NewProgressUpdateHelper(ach.publisher, ach.logger)

	// Send initial progress update
	initialMessage := fmt.Sprintf("Starting bulk %s operation", operationName)
	progressHelper.SendProgressUpdate(conn, operationID, tracker, initialMessage)

	// Process in batches to manage load
	for i := 0; i < len(accountIDs); i += config.BatchSize {
		end := i + config.BatchSize
		if end > len(accountIDs) {
			end = len(accountIDs)
		}

		// Process batch
		for j := i; j < end; j++ {
			accountID := accountIDs[j]

			// Execute operation
			err := processAccount(accountID)
			if err != nil {
				tracker.AddFailure(err, accountID)
			} else {
				tracker.AddSuccess()
			}

			// Send progress update if needed
			if tracker.ShouldSendProgress(config) {
				message := fmt.Sprintf("Processed %d/%d %ss (%d successful, %d failed)", tracker.Processed, tracker.Total, operationName, tracker.Successful, tracker.Failed)
				progressHelper.SendProgressUpdate(conn, operationID, tracker, message)
			}
		}

		// Small delay between batches to avoid overwhelming the system
		if end < len(accountIDs) {
			time.Sleep(config.BatchDelay)
		}
	}

	// Send final completion update
	finalMessage := fmt.Sprintf("Bulk %s completed: %d successful, %d failed out of %d accounts", operationName, tracker.Successful, tracker.Failed, tracker.Total)
	progressHelper.SendFinalUpdate(conn, operationID, tracker, finalMessage)
}

// processBulkUnfollow processes bulk unfollow operations with progress tracking
func (ach *AsyncCommandHandler) processBulkUnfollow(ctx context.Context, conn *streaming.ConnectionInfo, operationID string, accountIDs []string) {
	ach.processBulkRelationshipOperation(ctx, conn, operationID, accountIDs, "unfollow", func(accountID string) error {
		unfollowCmd := &relationships.UnfollowCommand{
			FollowerID:  conn.Username,
			FollowingID: accountID,
		}
		_, err := ach.relationshipsService.Unfollow(ctx, unfollowCmd)
		return err
	})
}

// processBulkUnmute processes bulk unmute operations with progress tracking
func (ach *AsyncCommandHandler) processBulkUnmute(ctx context.Context, conn *streaming.ConnectionInfo, operationID string, accountIDs []string) {
	ach.processBulkRelationshipOperation(ctx, conn, operationID, accountIDs, "unmute", func(accountID string) error {
		unmuteCmd := &relationships.UnmuteCommand{
			MuterID: conn.Username,
			MutedID: accountID,
		}
		_, err := ach.relationshipsService.Unmute(ctx, unmuteCmd)
		return err
	})
}

// ===== Helper Functions for Progress Updates =====

// Note: sendProgressUpdate and sendFinalUpdate functions have been replaced by shared helpers in command_helpers.go

// handleBulkDelete handles bulk deletion of multiple posts/content
func (ach *AsyncCommandHandler) handleBulkDelete(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"content_ids"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	contentIDs := ach.GetStringSlice(cmd.Payload, "content_ids")
	if err := common.ValidateEntityIDsList(contentIDs, "content"); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid content_ids", err.Error()), nil
	}
	if err := common.ValidateIntRange("content_ids_count", len(contentIDs), 1, 100); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid content_ids count", err.Error()), nil
	}

	// Optional parameters
	contentType := ach.GetString(cmd.Payload, "content_type", "status") // Default to status
	permanent := ach.GetBool(cmd.Payload, "permanent", false)           // Default to soft delete

	// Parse date range if provided
	var dateRange *bulk.DateRange
	if dateRangeData, exists := cmd.Payload["date_range"].(map[string]interface{}); exists {
		if startStr := ach.GetString(dateRangeData, "start", ""); startStr != "" {
			if start, err := time.Parse(time.RFC3339, startStr); err == nil {
				if endStr := ach.GetString(dateRangeData, "end", ""); endStr != "" {
					if end, err := time.Parse(time.RFC3339, endStr); err == nil {
						dateRange = &bulk.DateRange{Start: start, End: end}
					}
				}
			}
		}
	}

	bulkCmd := &bulk.DeleteCommand{
		Username:    conn.Username,
		ContentIDs:  contentIDs,
		ContentType: contentType,
		DateRange:   dateRange,
		Permanent:   permanent,
	}

	result, err := ach.bulkService.BulkDelete(ctx, bulkCmd)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "BULK_DELETE_FAILED",
			"Failed to start bulk delete operation", err.Error()), nil
	}

	data, err := ach.ConvertToJSON(result.Operation)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

// handleBulkContentOperation handles bulk content operations (archive/restore) with shared logic
func (ach *AsyncCommandHandler) handleBulkContentOperation(
	ctx context.Context,
	conn *streaming.ConnectionInfo,
	cmd *streaming.Command,
	operationType string,
	operationFunc func(context.Context, interface{}) (*bulk.OperationResult, error),
	createCommand func(username string, contentIDs []string, contentType string, dateRange *bulk.DateRange) interface{},
) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"content_ids"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	contentIDs := ach.GetStringSlice(cmd.Payload, "content_ids")
	if err := common.ValidateSliceNotEmpty("content_ids", contentIDs); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid content_ids", err.Error()), nil
	}

	// Limit batch size to prevent timeouts
	if err := common.ValidateSliceLength("content_ids", contentIDs, 100); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid content_ids count", err.Error()), nil
	}

	// Optional parameters
	contentType := ach.GetString(cmd.Payload, "content_type", "status") // Default to status

	// Parse date range if provided
	var dateRange *bulk.DateRange
	if dateRangeData, exists := cmd.Payload["date_range"].(map[string]interface{}); exists {
		if startStr := ach.GetString(dateRangeData, "start", ""); startStr != "" {
			if start, err := time.Parse(time.RFC3339, startStr); err == nil {
				if endStr := ach.GetString(dateRangeData, "end", ""); endStr != "" {
					if end, err := time.Parse(time.RFC3339, endStr); err == nil {
						dateRange = &bulk.DateRange{Start: start, End: end}
					}
				}
			}
		}
	}

	bulkCmd := createCommand(conn.Username, contentIDs, contentType, dateRange)

	result, err := operationFunc(ctx, bulkCmd)
	if err != nil {
		errorCode := fmt.Sprintf("BULK_%s_FAILED", strings.ToUpper(operationType))
		errorMessage := fmt.Sprintf("Failed to start bulk %s operation", operationType)
		return ach.CreateErrorResponse(cmd.ID, errorCode, errorMessage, err.Error()), nil
	}

	data, err := ach.ConvertToJSON(result.Operation)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

// handleBulkArchive handles bulk archiving of multiple posts
func (ach *AsyncCommandHandler) handleBulkArchive(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return ach.handleBulkContentOperation(ctx, conn, cmd, "archive",
		func(ctx context.Context, bulkCmd interface{}) (*bulk.OperationResult, error) {
			return ach.bulkService.BulkArchive(ctx, bulkCmd.(*bulk.ArchiveCommand))
		},
		func(username string, contentIDs []string, contentType string, dateRange *bulk.DateRange) interface{} {
			return &bulk.ArchiveCommand{
				Username:    username,
				ContentIDs:  contentIDs,
				ContentType: contentType,
				DateRange:   dateRange,
			}
		},
	)
}

// handleBulkRestore handles bulk restoration of archived posts
func (ach *AsyncCommandHandler) handleBulkRestore(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return ach.handleBulkContentOperation(ctx, conn, cmd, "restore",
		func(ctx context.Context, bulkCmd interface{}) (*bulk.OperationResult, error) {
			return ach.bulkService.BulkRestore(ctx, bulkCmd.(*bulk.RestoreCommand))
		},
		func(username string, contentIDs []string, contentType string, dateRange *bulk.DateRange) interface{} {
			return &bulk.RestoreCommand{
				Username:    username,
				ContentIDs:  contentIDs,
				ContentType: contentType,
				DateRange:   dateRange,
			}
		},
	)
}

// handleBulkExport handles bulk export of user content
func (ach *AsyncCommandHandler) handleBulkExport(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"content_ids", "format"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	contentIDs := ach.GetStringSlice(cmd.Payload, "content_ids")
	if err := common.ValidateSliceNotEmpty("content_ids", contentIDs); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid content_ids", err.Error()), nil
	}

	// Limit batch size to prevent timeouts
	if err := common.ValidateSliceLength("content_ids", contentIDs, 100); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid content_ids count", err.Error()), nil
	}

	format := ach.GetString(cmd.Payload, "format", "")
	if err := common.ValidateRequiredParam("format", format); err != nil {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid format", err.Error()), nil
	}

	// Validate format
	validFormats := []string{"json", "csv", "activitypub"}
	isValidFormat := false
	for _, validFormat := range validFormats {
		if format == validFormat {
			isValidFormat = true
			break
		}
	}
	if !isValidFormat {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR",
			"Invalid export format", "format must be one of: json, csv, activitypub"), nil
	}

	// Optional parameters
	contentType := ach.GetString(cmd.Payload, "content_type", "status") // Default to status
	includeMedia := ach.GetBool(cmd.Payload, "include_media", false)

	// Parse date range if provided
	var dateRange *bulk.DateRange
	if dateRangeData, exists := cmd.Payload["date_range"].(map[string]interface{}); exists {
		if startStr := ach.GetString(dateRangeData, "start", ""); startStr != "" {
			if start, err := time.Parse(time.RFC3339, startStr); err == nil {
				if endStr := ach.GetString(dateRangeData, "end", ""); endStr != "" {
					if end, err := time.Parse(time.RFC3339, endStr); err == nil {
						dateRange = &bulk.DateRange{Start: start, End: end}
					}
				}
			}
		}
	}

	bulkCmd := &bulk.ExportCommand{
		Username:     conn.Username,
		ContentIDs:   contentIDs,
		Format:       format,
		ContentType:  contentType,
		IncludeMedia: includeMedia,
		DateRange:    dateRange,
	}

	result, err := ach.bulkService.BulkExport(ctx, bulkCmd)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "BULK_EXPORT_FAILED",
			"Failed to start bulk export operation", err.Error()), nil
	}

	data, err := ach.ConvertToJSON(result.Operation)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}
