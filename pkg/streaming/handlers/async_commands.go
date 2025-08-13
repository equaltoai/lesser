// Package handlers provides WebSocket command handlers for async operations
package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/services/bulk"
	"github.com/equaltoai/lesser/pkg/services/importexport"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// BulkService defines the interface for bulk operations
type BulkService interface {
	BulkFollow(ctx context.Context, cmd *bulk.FollowCommand) (*bulk.OperationResult, error)
	BulkDeleteStatuses(ctx context.Context, cmd *bulk.DeleteStatusesCommand) (*bulk.OperationResult, error)
	GetOperation(ctx context.Context, query *bulk.GetOperationQuery) (*bulk.OperationResult, error)
}

// ImportExportService defines the interface for import/export operations
type ImportExportService interface {
	CreateExport(ctx context.Context, cmd *importexport.CreateExportCommand) (*importexport.ExportResult, error)
	GetExport(ctx context.Context, query *importexport.GetExportQuery) (*importexport.ExportResult, error)
	ListExports(ctx context.Context, query *importexport.ListExportsQuery) (*importexport.ExportListResult, error)
}

// AsyncCommandHandler handles WebSocket commands for async operations (bulk ops, import/export)
type AsyncCommandHandler struct {
	*streaming.BaseCommandHandler
	bulkService         BulkService
	importExportService ImportExportService
}

// NewAsyncCommandHandler creates a new async command handler
func NewAsyncCommandHandler(
	bulkService BulkService,
	importExportService ImportExportService,
	logger *zap.Logger,
) *AsyncCommandHandler {
	return &AsyncCommandHandler{
		BaseCommandHandler:  streaming.NewBaseCommandHandler(logger),
		bulkService:         bulkService,
		importExportService: importExportService,
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
	if len(accountIDs) == 0 {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR",
			"No account IDs provided", "account_ids array cannot be empty"), nil
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
func (ach *AsyncCommandHandler) handleBulkUnfollow(_ context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"account_ids"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	accountIDs := ach.GetStringSlice(cmd.Payload, "account_ids")
	if len(accountIDs) == 0 {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR",
			"No account IDs provided", "account_ids array cannot be empty"), nil
	}

	// Note: BulkUnfollow is not implemented in the bulk service yet
	// This would need to be added to the bulk service
	_ = accountIDs // Avoid unused variable warning
	return ach.CreateErrorResponse(cmd.ID, "NOT_IMPLEMENTED",
		"Bulk unfollow not yet implemented", "This feature is coming soon"), nil
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
	if len(statusIDs) == 0 {
		return ach.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR",
			"No status IDs provided", "status_ids array cannot be empty"), nil
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
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := ach.ValidatePayload(cmd.Payload, []string{"operation_id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	operationID := ach.GetString(cmd.Payload, "operation_id", "")

	query := &bulk.GetOperationQuery{
		OperationID: operationID,
		Username:    conn.Username,
	}

	result, err := ach.bulkService.GetOperation(ctx, query)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "GET_OPERATION_FAILED",
			"Failed to get bulk operation", err.Error()), nil
	}

	data, err := ach.ConvertToJSON(result.Operation)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
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

// Placeholder handlers for commands not yet implemented

func (ach *AsyncCommandHandler) handleBulkMute(_ context.Context, _ *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return ach.CreateErrorResponse(cmd.ID, "NOT_IMPLEMENTED", "Bulk mute not yet implemented", "This feature is coming soon"), nil
}

func (ach *AsyncCommandHandler) handleBulkUnmute(_ context.Context, _ *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return ach.CreateErrorResponse(cmd.ID, "NOT_IMPLEMENTED", "Bulk unmute not yet implemented", "This feature is coming soon"), nil
}

func (ach *AsyncCommandHandler) handleBulkBlock(_ context.Context, _ *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return ach.CreateErrorResponse(cmd.ID, "NOT_IMPLEMENTED", "Bulk block not yet implemented", "This feature is coming soon"), nil
}

func (ach *AsyncCommandHandler) handleBulkUnblock(_ context.Context, _ *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return ach.CreateErrorResponse(cmd.ID, "NOT_IMPLEMENTED", "Bulk unblock not yet implemented", "This feature is coming soon"), nil
}

func (ach *AsyncCommandHandler) handleBulkListMembers(_ context.Context, _ *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return ach.CreateErrorResponse(cmd.ID, "NOT_IMPLEMENTED", "Bulk list members not yet implemented", "This feature is coming soon"), nil
}

func (ach *AsyncCommandHandler) handleCancelExport(_ context.Context, _ *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return ach.CreateErrorResponse(cmd.ID, "NOT_IMPLEMENTED", "Cancel export not yet implemented", "This feature is coming soon"), nil
}

func (ach *AsyncCommandHandler) handleCreateImport(_ context.Context, _ *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return ach.CreateErrorResponse(cmd.ID, "NOT_IMPLEMENTED", "Create import not yet implemented", "This feature is coming soon"), nil
}

func (ach *AsyncCommandHandler) handleGetImport(_ context.Context, _ *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return ach.CreateErrorResponse(cmd.ID, "NOT_IMPLEMENTED", "Get import not yet implemented", "This feature is coming soon"), nil
}

func (ach *AsyncCommandHandler) handleListImports(_ context.Context, _ *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return ach.CreateErrorResponse(cmd.ID, "NOT_IMPLEMENTED", "List imports not yet implemented", "This feature is coming soon"), nil
}