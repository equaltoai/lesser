// Package streaming provides WebSocket command handling infrastructure
package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Command represents a WebSocket command message
type Command struct {
	ID      string                 `json:"id"`      // Client-provided ID for request/response matching
	Type    string                 `json:"type"`    // Command type (e.g., "create_status", "follow_user")
	Payload map[string]interface{} `json:"payload"` // Command payload data
}

// CommandResponse represents a WebSocket command response
type CommandResponse struct {
	ID      string                 `json:"id"`      // Matching client ID
	Type    string                 `json:"type"`    // Response type (e.g., "command_result", "command_error")
	Success bool                   `json:"success"` // Whether command succeeded
	Data    map[string]interface{} `json:"data"`    // Response data
	Error   *CommandError          `json:"error,omitempty"` // Error details if failed
}

// CommandError represents an error in command execution
type CommandError struct {
	Code    string `json:"code"`    // Error code
	Message string `json:"message"` // Human-readable error message
	Details string `json:"details,omitempty"` // Additional error details
}

// CommandHandler defines the interface for handling WebSocket commands
type CommandHandler interface {
	// HandleCommand processes a WebSocket command and returns a response
	HandleCommand(ctx context.Context, conn *ConnectionInfo, cmd *Command) (*CommandResponse, error)

	// GetSupportedCommands returns a list of command types this handler supports
	GetSupportedCommands() []string
}

// ConnectionInfo contains information about the WebSocket connection
type ConnectionInfo struct {
	ConnectionID string   `json:"connection_id"`
	UserID       string   `json:"user_id"`
	Username     string   `json:"username"`
	Streams      []string `json:"streams"`
	IsAuthenticated bool  `json:"is_authenticated"`
	Metadata     map[string]interface{} `json:"metadata"` // For storing connection-specific data
}

// ServiceRegistry defines the interface needed by command handlers
type ServiceRegistry interface {
	// Define the methods we need from the registry
	// This avoids circular import issues
}

// CommandRouter routes WebSocket commands to appropriate handlers
type CommandRouter struct {
	handlers map[string]CommandHandler
	logger   *zap.Logger
}

// NewCommandRouter creates a new WebSocket command router
func NewCommandRouter(logger *zap.Logger) *CommandRouter {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &CommandRouter{
		handlers: make(map[string]CommandHandler),
		logger:   logger.With(zap.String("component", "command_router")),
	}
}

// RegisterHandler registers a command handler for specific command types
func (cr *CommandRouter) RegisterHandler(handler CommandHandler) {
	supportedCommands := handler.GetSupportedCommands()
	for _, cmdType := range supportedCommands {
		cr.handlers[cmdType] = handler
		cr.logger.Debug("registered command handler",
			zap.String("command_type", cmdType),
			zap.String("handler", fmt.Sprintf("%T", handler)))
	}
}

// HandleCommand routes a command to the appropriate handler
func (cr *CommandRouter) HandleCommand(ctx context.Context, conn *ConnectionInfo, cmd *Command) (*CommandResponse, error) {
	start := time.Now()
	
	cr.logger.Info("handling WebSocket command",
		zap.String("command_id", cmd.ID),
		zap.String("command_type", cmd.Type),
		zap.String("connection_id", conn.ConnectionID),
		zap.String("user_id", conn.UserID))

	// Find handler for this command type
	handler, exists := cr.handlers[cmd.Type]
	if !exists {
		err := &CommandError{
			Code:    "UNSUPPORTED_COMMAND",
			Message: fmt.Sprintf("Command type '%s' is not supported", cmd.Type),
			Details: "Available commands: " + cr.getSupportedCommandsList(),
		}
		
		response := &CommandResponse{
			ID:      cmd.ID,
			Type:    "command_error",
			Success: false,
			Error:   err,
		}
		
		cr.logger.Warn("unsupported command type",
			zap.String("command_id", cmd.ID),
			zap.String("command_type", cmd.Type),
			zap.Duration("duration", time.Since(start)))
		
		return response, nil
	}

	// Execute command with the handler
	response, err := handler.HandleCommand(ctx, conn, cmd)
	duration := time.Since(start)
	
	if err != nil {
		cr.logger.Error("command handler failed",
			zap.String("command_id", cmd.ID),
			zap.String("command_type", cmd.Type),
			zap.String("handler", fmt.Sprintf("%T", handler)),
			zap.Error(err),
			zap.Duration("duration", duration))
		
		// Create error response if handler didn't provide one
		if response == nil {
			response = &CommandResponse{
				ID:      cmd.ID,
				Type:    "command_error",
				Success: false,
				Error: &CommandError{
					Code:    "HANDLER_ERROR",
					Message: "Command execution failed",
					Details: err.Error(),
				},
			}
		}
	} else if response != nil {
		cr.logger.Info("command executed successfully",
			zap.String("command_id", cmd.ID),
			zap.String("command_type", cmd.Type),
			zap.Bool("success", response.Success),
			zap.Duration("duration", duration))
	}

	return response, nil
}

// GetSupportedCommands returns a list of all supported command types
func (cr *CommandRouter) GetSupportedCommands() []string {
	commands := make([]string, 0, len(cr.handlers))
	for cmdType := range cr.handlers {
		commands = append(commands, cmdType)
	}
	return commands
}

// getSupportedCommandsList returns a formatted string of supported commands
func (cr *CommandRouter) getSupportedCommandsList() string {
	commands := cr.GetSupportedCommands()
	if len(commands) == 0 {
		return "none"
	}
	
	result := ""
	for i, cmd := range commands {
		if i > 0 {
			result += ", "
		}
		result += cmd
	}
	return result
}

// BaseCommandHandler provides common functionality for command handlers
type BaseCommandHandler struct {
	logger   *zap.Logger
}

// NewBaseCommandHandler creates a new base command handler
func NewBaseCommandHandler(logger *zap.Logger) *BaseCommandHandler {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &BaseCommandHandler{
		logger:   logger,
	}
}

// CreateSuccessResponse creates a successful command response
func (bch *BaseCommandHandler) CreateSuccessResponse(commandID string, data map[string]interface{}) *CommandResponse {
	return &CommandResponse{
		ID:      commandID,
		Type:    "command_result",
		Success: true,
		Data:    data,
	}
}

// CreateErrorResponse creates an error command response
func (bch *BaseCommandHandler) CreateErrorResponse(commandID string, code string, message string, details string) *CommandResponse {
	return &CommandResponse{
		ID:      commandID,
		Type:    "command_error",
		Success: false,
		Error: &CommandError{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
}

// RequireAuth checks if the connection is authenticated
func (bch *BaseCommandHandler) RequireAuth(conn *ConnectionInfo, commandID string) *CommandResponse {
	if !conn.IsAuthenticated || conn.UserID == "" {
		return bch.CreateErrorResponse(commandID, "AUTHENTICATION_REQUIRED", 
			"This command requires authentication", "Please authenticate before using this command")
	}
	return nil
}

// ValidatePayload validates that required fields are present in the command payload
func (bch *BaseCommandHandler) ValidatePayload(payload map[string]interface{}, required []string, commandID string) *CommandResponse {
	missing := make([]string, 0)
	
	for _, field := range required {
		if _, exists := payload[field]; !exists {
			missing = append(missing, field)
		}
	}
	
	if len(missing) > 0 {
		return bch.CreateErrorResponse(commandID, "VALIDATION_ERROR",
			"Required fields missing", fmt.Sprintf("Missing fields: %v", missing))
	}
	
	return nil
}

// GetString safely extracts a string value from the payload
func (bch *BaseCommandHandler) GetString(payload map[string]interface{}, key string, defaultValue string) string {
	if value, exists := payload[key]; exists {
		if strValue, ok := value.(string); ok {
			return strValue
		}
	}
	return defaultValue
}

// GetInt safely extracts an int value from the payload
func (bch *BaseCommandHandler) GetInt(payload map[string]interface{}, key string, defaultValue int) int {
	if value, exists := payload[key]; exists {
		switch v := value.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case string:
			// Try to parse string as int - but default value on failure
			return defaultValue
		}
	}
	return defaultValue
}

// GetBool safely extracts a bool value from the payload
func (bch *BaseCommandHandler) GetBool(payload map[string]interface{}, key string, defaultValue bool) bool {
	if value, exists := payload[key]; exists {
		if boolValue, ok := value.(bool); ok {
			return boolValue
		}
	}
	return defaultValue
}

// GetStringSlice safely extracts a string slice from the payload
func (bch *BaseCommandHandler) GetStringSlice(payload map[string]interface{}, key string) []string {
	if value, exists := payload[key]; exists {
		if slice, ok := value.([]interface{}); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if strItem, ok := item.(string); ok {
					result = append(result, strItem)
				}
			}
			return result
		}
	}
	return nil
}

// ConvertToJSON converts data to JSON for response
func (bch *BaseCommandHandler) ConvertToJSON(data interface{}) (map[string]interface{}, error) {
	// First marshal to JSON bytes
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}
	
	// Then unmarshal to map[string]interface{}
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}
	
	return result, nil
}