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
	// Initialize payload helpers
	payloadHelpers := createPayloadHelpers()
	
	// Initialize all status command executors
	sch.initializeCreateExecutor(payloadHelpers)
	sch.initializeDeleteExecutor(payloadHelpers)
	sch.initializeFavoriteExecutors(payloadHelpers)
	sch.initializeReblogExecutors(payloadHelpers)
	sch.initializeBookmarkExecutors(payloadHelpers)
	sch.initializeMuteExecutors(payloadHelpers)
	sch.initializePinExecutors(payloadHelpers)
}

// payloadHelpers contains helper functions for extracting data from payloads
type payloadHelpers struct {
	getString      func(map[string]interface{}, string, string) string
	getBool        func(map[string]interface{}, string, bool) bool
	getStringSlice func(map[string]interface{}, string) []string
}

// createPayloadHelpers creates the payload helper functions
func createPayloadHelpers() payloadHelpers {
	return payloadHelpers{
		getString: func(payload map[string]interface{}, key, defaultVal string) string {
			if val, ok := payload[key].(string); ok {
				return val
			}
			return defaultVal
		},
		getBool: func(payload map[string]interface{}, key string, defaultVal bool) bool {
			if val, ok := payload[key].(bool); ok {
				return val
			}
			return defaultVal
		},
		getStringSlice: func(payload map[string]interface{}, key string) []string {
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
		},
	}
}

// initializeCreateExecutor initializes the create status executor
func (sch *StatusCommandHandlerV2) initializeCreateExecutor(h payloadHelpers) {
	sch.executors[streaming.CmdCreateStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"status"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.CreateNoteCommand{
				AuthorID:    conn.UserID,
				Content:     h.getString(payload, "status", ""),
				InReplyToID: h.getString(payload, "in_reply_to_id", ""),
				MediaIDs:    h.getStringSlice(payload, "media_ids"),
				Sensitive:   h.getBool(payload, "sensitive", false),
				SpoilerText: h.getString(payload, "spoiler_text", ""),
				Visibility:  h.getString(payload, "visibility", "public"),
				Language:    h.getString(payload, "language", ""),
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
}

// initializeDeleteExecutor initializes the delete status executor
func (sch *StatusCommandHandlerV2) initializeDeleteExecutor(h payloadHelpers) {
	sch.executors[streaming.CmdDeleteStatus] = &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		commandBuilder: func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return &notes.DeleteNoteCommand{
				StatusID:  h.getString(payload, "id", ""),
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
}

// initializeFavoriteExecutors initializes favorite and unfavorite executors
func (sch *StatusCommandHandlerV2) initializeFavoriteExecutors(h payloadHelpers) {
	// Favorite Status executor
	sch.executors[streaming.CmdFavoriteStatus] = sch.createSimpleStatusExecutor(
		h, "id", "LikeNoteCommand", sch.notesService.LikeNote)
	
	// Unfavorite Status executor
	sch.executors[streaming.CmdUnfavoriteStatus] = sch.createSimpleStatusExecutor(
		h, "id", "UnlikeNoteCommand", sch.notesService.UnlikeNote)
}

// initializeReblogExecutors initializes reblog and unreblog executors
func (sch *StatusCommandHandlerV2) initializeReblogExecutors(h payloadHelpers) {
	// Reblog Status executor
	sch.executors[streaming.CmdReblogStatus] = sch.createSimpleStatusExecutor(
		h, "id", "ReblogNoteCommand", sch.notesService.ReblogNote)
	
	// Unreblog Status executor
	sch.executors[streaming.CmdUnreblogStatus] = sch.createSimpleStatusExecutor(
		h, "id", "UnreblogNoteCommand", sch.notesService.UnreblogNote)
}

// initializeBookmarkExecutors initializes bookmark and unbookmark executors
func (sch *StatusCommandHandlerV2) initializeBookmarkExecutors(h payloadHelpers) {
	// Bookmark Status executor
	sch.executors[streaming.CmdBookmarkStatus] = sch.createSimpleStatusExecutor(
		h, "id", "BookmarkNoteCommand", sch.notesService.BookmarkNote)
	
	// Unbookmark Status executor
	sch.executors[streaming.CmdUnbookmarkStatus] = sch.createSimpleStatusExecutor(
		h, "id", "UnbookmarkNoteCommand", sch.notesService.UnbookmarkNote)
}

// initializeMuteExecutors initializes mute and unmute executors
func (sch *StatusCommandHandlerV2) initializeMuteExecutors(h payloadHelpers) {
	// Mute Status executor
	sch.executors[streaming.CmdMuteStatus] = sch.createSimpleStatusExecutor(
		h, "id", "MuteNoteCommand", sch.notesService.MuteNote)
	
	// Unmute Status executor
	sch.executors[streaming.CmdUnmuteStatus] = sch.createSimpleStatusExecutor(
		h, "id", "UnmuteNoteCommand", sch.notesService.UnmuteNote)
}

// initializePinExecutors initializes pin and unpin executors
func (sch *StatusCommandHandlerV2) initializePinExecutors(h payloadHelpers) {
	// Pin Status executor
	sch.executors[streaming.CmdPinStatus] = sch.createSimpleStatusExecutor(
		h, "id", "PinNoteCommand", sch.notesService.PinNote)
	
	// Unpin Status executor
	sch.executors[streaming.CmdUnpinStatus] = sch.createSimpleStatusExecutor(
		h, "id", "UnpinNoteCommand", sch.notesService.UnpinNote)
}

// createSimpleStatusExecutor is a factory function for creating standard status executors
func (sch *StatusCommandHandlerV2) createSimpleStatusExecutor(
	h payloadHelpers,
	idField, cmdType string,
	serviceMethod interface{},
) *SimpleStatusExecutor {
	return &SimpleStatusExecutor{
		requiresAuth:   true,
		requiredFields: []string{idField},
		commandBuilder: sch.createCommandBuilder(h, idField, cmdType),
		executor:       sch.createServiceExecutor(serviceMethod),
		responseKey:    "",
	}
}

// createCommandBuilder creates a command builder function for the given command type
func (sch *StatusCommandHandlerV2) createCommandBuilder(
	h payloadHelpers,
	idField, cmdType string,
) func(*streaming.ConnectionInfo, map[string]interface{}) interface{} {
	return func(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
		return sch.buildCommand(h, conn, payload, idField, cmdType)
	}
}

// buildCommand creates the appropriate command struct based on the command type
func (sch *StatusCommandHandlerV2) buildCommand(
	h payloadHelpers,
	conn *streaming.ConnectionInfo,
	payload map[string]interface{},
	idField, cmdType string,
) interface{} {
	statusID := h.getString(payload, idField, "")
	userID := conn.UserID
	
	switch cmdType {
	case "LikeNoteCommand":
		return &notes.LikeNoteCommand{StatusID: statusID, LikerID: userID}
	case "UnlikeNoteCommand":
		return &notes.UnlikeNoteCommand{StatusID: statusID, UnlikerID: userID}
	case "ReblogNoteCommand":
		return &notes.ReblogNoteCommand{StatusID: statusID, RebloggerID: userID}
	case "UnreblogNoteCommand":
		return &notes.UnreblogNoteCommand{StatusID: statusID, UnrebloggerID: userID}
	case "BookmarkNoteCommand":
		return &notes.BookmarkNoteCommand{StatusID: statusID, BookmarkerID: userID}
	case "UnbookmarkNoteCommand":
		return &notes.UnbookmarkNoteCommand{StatusID: statusID, UnbookmarkerID: userID}
	case "MuteNoteCommand":
		return &notes.MuteNoteCommand{StatusID: statusID, MuterID: userID}
	case "UnmuteNoteCommand":
		return &notes.UnmuteNoteCommand{StatusID: statusID, MuterID: userID}
	case "PinNoteCommand":
		return &notes.PinNoteCommand{StatusID: statusID, PinnerID: userID}
	case "UnpinNoteCommand":
		return &notes.UnpinNoteCommand{StatusID: statusID, PinnerID: userID}
	default:
		return nil
	}
}

// createServiceExecutor creates an executor function that calls the appropriate service method
func (sch *StatusCommandHandlerV2) createServiceExecutor(
	serviceMethod interface{},
) func(context.Context, interface{}) (interface{}, error) {
	return func(ctx context.Context, cmd interface{}) (interface{}, error) {
		result, err := sch.callServiceMethod(ctx, cmd, serviceMethod)
		if err != nil {
			return nil, err
		}
		return sch.extractResponseFromResult(result), nil
	}
}

// callServiceMethod dynamically calls the appropriate service method
func (sch *StatusCommandHandlerV2) callServiceMethod(
	ctx context.Context,
	cmd interface{},
	serviceMethod interface{},
) (interface{}, error) {
	switch method := serviceMethod.(type) {
	case func(context.Context, *notes.LikeNoteCommand) (*notes.LikeResult, error):
		return method(ctx, cmd.(*notes.LikeNoteCommand))
	case func(context.Context, *notes.UnlikeNoteCommand) (*notes.LikeResult, error):
		return method(ctx, cmd.(*notes.UnlikeNoteCommand))
	case func(context.Context, *notes.ReblogNoteCommand) (*notes.LikeResult, error):
		return method(ctx, cmd.(*notes.ReblogNoteCommand))
	case func(context.Context, *notes.UnreblogNoteCommand) (*notes.LikeResult, error):
		return method(ctx, cmd.(*notes.UnreblogNoteCommand))
	case func(context.Context, *notes.BookmarkNoteCommand) (*notes.BookmarkResult, error):
		return method(ctx, cmd.(*notes.BookmarkNoteCommand))
	case func(context.Context, *notes.UnbookmarkNoteCommand) (*notes.BookmarkResult, error):
		return method(ctx, cmd.(*notes.UnbookmarkNoteCommand))
	case func(context.Context, *notes.MuteNoteCommand) (*notes.LikeResult, error):
		return method(ctx, cmd.(*notes.MuteNoteCommand))
	case func(context.Context, *notes.UnmuteNoteCommand) (*notes.LikeResult, error):
		return method(ctx, cmd.(*notes.UnmuteNoteCommand))
	case func(context.Context, *notes.PinNoteCommand) (*notes.LikeResult, error):
		return method(ctx, cmd.(*notes.PinNoteCommand))
	case func(context.Context, *notes.UnpinNoteCommand) (*notes.LikeResult, error):
		return method(ctx, cmd.(*notes.UnpinNoteCommand))
	default:
		return nil, fmt.Errorf("unsupported service method type")
	}
}

// extractResponseFromResult extracts the Status field from service results
func (sch *StatusCommandHandlerV2) extractResponseFromResult(result interface{}) interface{} {
	switch res := result.(type) {
	case *notes.LikeResult:
		return res.Status
	case *notes.BookmarkResult:
		return res.Status
	default:
		return result
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