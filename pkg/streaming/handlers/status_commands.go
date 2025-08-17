// Package handlers provides WebSocket command handlers for different domains
package handlers

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// StatusCommandHandlerV2 handles WebSocket commands related to statuses/notes with reduced duplication
type StatusCommandHandlerV2 struct {
	*streaming.BaseCommandHandler
	notesService *notes.Service
	executors    map[string]CommandExecutor
}

// NewStatusCommandHandlerV2 creates a new status command handler with reduced duplication
func NewStatusCommandHandlerV2(notesService *notes.Service, logger *zap.Logger) *StatusCommandHandlerV2 {
	handler := &StatusCommandHandlerV2{
		BaseCommandHandler: streaming.NewBaseCommandHandler(logger),
		notesService:       notesService,
		executors:          make(map[string]CommandExecutor),
	}
	
	// Initialize executors for each command type
	handler.initializeExecutors()
	
	return handler
}

// initializeExecutors sets up the command executors for each command type
func (sch *StatusCommandHandlerV2) initializeExecutors() {
	// Helper function to get string from payload
	getString := func(payload map[string]interface{}, key, defaultVal string) string {
		if val, ok := payload[key].(string); ok {
			return val
		}
		return defaultVal
	}
	
	// Helper function to get bool from payload
	getBool := func(payload map[string]interface{}, key string, defaultVal bool) bool {
		if val, ok := payload[key].(bool); ok {
			return val
		}
		return defaultVal
	}
	
	// Helper function to get string slice from payload
	getStringSlice := func(payload map[string]interface{}, key string) []string {
		if val, ok := payload[key].([]interface{}); ok {
			result := make([]string, 0, len(val))
			for _, v := range val {
				if s, ok := v.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
		return []string{}
	}
	
	// Create Status executor
	sch.executors[streaming.CmdCreateStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"status"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.CreateNoteCommand{
				AuthorID:    conn.UserID,
				Content:     getString(payload, "status", ""),
				InReplyToID: getString(payload, "in_reply_to_id", ""),
				MediaIDs:    getStringSlice(payload, "media_ids"),
				Sensitive:   getBool(payload, "sensitive", false),
				Visibility:  getString(payload, "visibility", "public"),
				Language:    getString(payload, "language", ""),
			}
		},
		executor: func(ctx context.Context, cmd interface{}) (interface{}, error) {
			result, err := sch.notesService.CreateNote(ctx, cmd.(*notes.CreateNoteCommand))
			if err != nil {
				return nil, err
			}
			return result.Note, nil
		},
		responseKey: "",
	}
	
	// Delete Status executor
	sch.executors[streaming.CmdDeleteStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.DeleteNoteCommand{
				StatusID:  getString(payload, "id", ""),
				DeleterID: conn.UserID,
			}
		},
		executor: func(ctx context.Context, cmd interface{}) (interface{}, error) {
			err := sch.notesService.DeleteNote(ctx, cmd.(*notes.DeleteNoteCommand))
			if err != nil {
				return nil, err
			}
			statusID := cmd.(*notes.DeleteNoteCommand).StatusID
			return map[string]interface{}{
				"deleted": true,
				"id":      statusID,
			}, nil
		},
		responseKey: "",
	}
	
	// Favorite Status executor
	sch.executors[streaming.CmdFavoriteStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.LikeNoteCommand{
				StatusID: getString(payload, "id", ""),
				LikerID:  conn.UserID,
			}
		},
		executor: func(ctx context.Context, cmd interface{}) (interface{}, error) {
			result, err := sch.notesService.LikeNote(ctx, cmd.(*notes.LikeNoteCommand))
			if err != nil {
				return nil, err
			}
			return result.Status, nil
		},
		responseKey: "",
	}
	
	// Unfavorite Status executor
	sch.executors[streaming.CmdUnfavoriteStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.UnlikeNoteCommand{
				StatusID:  getString(payload, "id", ""),
				UnlikerID: conn.UserID,
			}
		},
		executor: func(ctx context.Context, cmd interface{}) (interface{}, error) {
			result, err := sch.notesService.UnlikeNote(ctx, cmd.(*notes.UnlikeNoteCommand))
			if err != nil {
				return nil, err
			}
			return result.Status, nil
		},
		responseKey: "",
	}
	
	// Reblog Status executor
	sch.executors[streaming.CmdReblogStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.ReblogNoteCommand{
				StatusID:    getString(payload, "id", ""),
				RebloggerID: conn.UserID,
			}
		},
		executor: func(ctx context.Context, cmd interface{}) (interface{}, error) {
			result, err := sch.notesService.ReblogNote(ctx, cmd.(*notes.ReblogNoteCommand))
			if err != nil {
				return nil, err
			}
			return result.Status, nil
		},
		responseKey: "",
	}
	
	// Unreblog Status executor
	sch.executors[streaming.CmdUnreblogStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.UnreblogNoteCommand{
				StatusID:      getString(payload, "id", ""),
				UnrebloggerID: conn.UserID,
			}
		},
		executor: func(ctx context.Context, cmd interface{}) (interface{}, error) {
			result, err := sch.notesService.UnreblogNote(ctx, cmd.(*notes.UnreblogNoteCommand))
			if err != nil {
				return nil, err
			}
			return result.Status, nil
		},
		responseKey: "",
	}
	
	// Bookmark Status executor
	sch.executors[streaming.CmdBookmarkStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.BookmarkNoteCommand{
				StatusID:     getString(payload, "id", ""),
				BookmarkerID: conn.UserID,
			}
		},
		executor: func(ctx context.Context, cmd interface{}) (interface{}, error) {
			result, err := sch.notesService.BookmarkNote(ctx, cmd.(*notes.BookmarkNoteCommand))
			if err != nil {
				return nil, err
			}
			return result.Status, nil
		},
		responseKey: "",
	}
	
	// Unbookmark Status executor
	sch.executors[streaming.CmdUnbookmarkStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.UnbookmarkNoteCommand{
				StatusID:       getString(payload, "id", ""),
				UnbookmarkerID: conn.UserID,
			}
		},
		executor: func(ctx context.Context, cmd interface{}) (interface{}, error) {
			result, err := sch.notesService.UnbookmarkNote(ctx, cmd.(*notes.UnbookmarkNoteCommand))
			if err != nil {
				return nil, err
			}
			return result.Status, nil
		},
		responseKey: "",
	}
	
	// Mute Status executor
	sch.executors[streaming.CmdMuteStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.MuteNoteCommand{
				StatusID: getString(payload, "id", ""),
				MuterID:  conn.UserID,
			}
		},
		executor: func(ctx context.Context, cmd interface{}) (interface{}, error) {
			result, err := sch.notesService.MuteNote(ctx, cmd.(*notes.MuteNoteCommand))
			if err != nil {
				return nil, err
			}
			return result.Status, nil
		},
		responseKey: "",
	}
	
	// Unmute Status executor
	sch.executors[streaming.CmdUnmuteStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.UnmuteNoteCommand{
				StatusID: getString(payload, "id", ""),
				MuterID:  conn.UserID,
			}
		},
		executor: func(ctx context.Context, cmd interface{}) (interface{}, error) {
			result, err := sch.notesService.UnmuteNote(ctx, cmd.(*notes.UnmuteNoteCommand))
			if err != nil {
				return nil, err
			}
			return result.Status, nil
		},
		responseKey: "",
	}
	
	// Pin Status executor
	sch.executors[streaming.CmdPinStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.PinNoteCommand{
				StatusID: getString(payload, "id", ""),
				PinnerID: conn.UserID,
			}
		},
		executor: func(ctx context.Context, cmd interface{}) (interface{}, error) {
			result, err := sch.notesService.PinNote(ctx, cmd.(*notes.PinNoteCommand))
			if err != nil {
				return nil, err
			}
			return result.Status, nil
		},
		responseKey: "",
	}
	
	// Unpin Status executor
	sch.executors[streaming.CmdUnpinStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.UnpinNoteCommand{
				StatusID: getString(payload, "id", ""),
				PinnerID: conn.UserID,
			}
		},
		executor: func(ctx context.Context, cmd interface{}) (interface{}, error) {
			result, err := sch.notesService.UnpinNote(ctx, cmd.(*notes.UnpinNoteCommand))
			if err != nil {
				return nil, err
			}
			return result.Status, nil
		},
		responseKey: "",
	}
}

// GetSupportedCommands returns the list of commands this handler supports
func (sch *StatusCommandHandlerV2) GetSupportedCommands() []string {
	commands := make([]string, 0, len(sch.executors))
	for cmd := range sch.executors {
		commands = append(commands, cmd)
	}
	return commands
}

// HandleCommand processes status-related WebSocket commands with reduced duplication
func (sch *StatusCommandHandlerV2) HandleCommand(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	executor, exists := sch.executors[cmd.Type]
	if !exists {
		return sch.CreateErrorResponse(cmd.ID, "UNSUPPORTED_COMMAND",
			"Unsupported status command", fmt.Sprintf("Command %s not supported by status handler", cmd.Type)), nil
	}
	
	// Map command type to error code
	errorCode := getErrorCodeForCommand(cmd.Type)
	
	// Use the generic execution flow
	return ExecuteGenericCommand(ctx, sch.BaseCommandHandler, conn, cmd, executor, errorCode)
}

// getErrorCodeForCommand returns the appropriate error code for a command type
func getErrorCodeForCommand(cmdType string) string {
	errorCodes := map[string]string{
		streaming.CmdCreateStatus:      "CREATE_FAILED",
		streaming.CmdDeleteStatus:      "DELETE_FAILED",
		streaming.CmdFavoriteStatus:    "FAVORITE_FAILED",
		streaming.CmdUnfavoriteStatus:  "UNFAVORITE_FAILED",
		streaming.CmdReblogStatus:      "REBLOG_FAILED",
		streaming.CmdUnreblogStatus:    "UNREBLOG_FAILED",
		streaming.CmdBookmarkStatus:    "BOOKMARK_FAILED",
		streaming.CmdUnbookmarkStatus:  "UNBOOKMARK_FAILED",
		streaming.CmdMuteStatus:        "MUTE_FAILED",
		streaming.CmdUnmuteStatus:      "UNMUTE_FAILED",
		streaming.CmdPinStatus:         "PIN_FAILED",
		streaming.CmdUnpinStatus:       "UNPIN_FAILED",
	}
	
	if code, exists := errorCodes[cmdType]; exists {
		return code
	}
	return "COMMAND_FAILED"
}