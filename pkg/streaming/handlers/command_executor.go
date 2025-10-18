// Package handlers provides WebSocket command handlers for different domains
package handlers

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/streaming"
)

// CommandExecutor defines the interface for executing service commands
type CommandExecutor interface {
	// RequiresAuth returns true if the command requires authentication
	RequiresAuth() bool

	// RequiredFields returns the list of required fields for validation
	RequiredFields() []string

	// BuildCommand builds the service command from the WebSocket command payload
	BuildCommand(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{}

	// Execute executes the service command and returns the result
	Execute(ctx context.Context, serviceCmd interface{}) (interface{}, error)

	// FormatResponse formats the service result for WebSocket response
	FormatResponse(result interface{}) (map[string]interface{}, error)
}

// SimpleStatusExecutor handles simple status operations that follow a common pattern
type SimpleStatusExecutor struct {
	requiresAuth   bool
	requiredFields []string
	commandBuilder func(*streaming.ConnectionInfo, map[string]interface{}) interface{}
	executor       func(context.Context, interface{}) (interface{}, error)
	responseKey    string // Key for the response data (e.g., "Status", "Note")
}

// NewSimpleStatusExecutor creates a new simple status executor
func NewSimpleStatusExecutor(
	requiresAuth bool,
	requiredFields []string,
	commandBuilder func(*streaming.ConnectionInfo, map[string]interface{}) interface{},
	executor func(context.Context, interface{}) (interface{}, error),
	responseKey string,
) *SimpleStatusExecutor {
	return &SimpleStatusExecutor{
		requiresAuth:   requiresAuth,
		requiredFields: requiredFields,
		commandBuilder: commandBuilder,
		executor:       executor,
		responseKey:    responseKey,
	}
}

// RequiresAuth returns whether this command requires authentication
func (e *SimpleStatusExecutor) RequiresAuth() bool {
	return e.requiresAuth
}

// RequiredFields returns the list of required fields for validation
func (e *SimpleStatusExecutor) RequiredFields() []string {
	return e.requiredFields
}

// BuildCommand builds the service command from the WebSocket command payload
func (e *SimpleStatusExecutor) BuildCommand(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
	return e.commandBuilder(conn, payload)
}

// Execute executes the service command and returns the result
func (e *SimpleStatusExecutor) Execute(ctx context.Context, serviceCmd interface{}) (interface{}, error) {
	return e.executor(ctx, serviceCmd)
}

// FormatResponse formats the service result for WebSocket response
func (e *SimpleStatusExecutor) FormatResponse(result interface{}) (map[string]interface{}, error) {
	// If responseKey is empty, return the result as-is (for custom responses)
	if err := common.ValidateRequiredParam("responseKey", e.responseKey); err != nil {
		if data, ok := result.(map[string]interface{}); ok {
			return data, nil
		}
		return nil, fmt.Errorf("invalid response format")
	}

	// Otherwise, extract the specified field from the result
	if resultMap, ok := result.(map[string]interface{}); ok {
		if data, exists := resultMap[e.responseKey]; exists {
			return data.(map[string]interface{}), nil
		}
	}

	// If we can't extract, just return the whole result
	return map[string]interface{}{"result": result}, nil
}

// ExecuteGenericCommand provides a generic execution flow for commands
func ExecuteGenericCommand(
	ctx context.Context,
	handler *streaming.BaseCommandHandler,
	conn *streaming.ConnectionInfo,
	cmd *streaming.Command,
	executor CommandExecutor,
	errorCode string,
) (*streaming.CommandResponse, error) {
	// Check authentication if required
	if executor.RequiresAuth() {
		if authErr := handler.RequireAuth(conn, cmd.ID); authErr != nil {
			return authErr, nil
		}
	}

	// Validate required fields
	requiredFields := executor.RequiredFields()
	if len(requiredFields) > 0 {
		if validationErr := handler.ValidatePayload(cmd.Payload, requiredFields, cmd.ID); validationErr != nil {
			return validationErr, nil
		}
	}

	// Build the service command
	serviceCmd := executor.BuildCommand(conn, cmd.Payload)

	// Execute the service command
	result, err := executor.Execute(ctx, serviceCmd)
	if err != nil {
		return handler.CreateErrorResponse(cmd.ID, errorCode,
			fmt.Sprintf("Failed to execute %s", cmd.Type), err.Error()), nil
	}

	// Format the response
	data, err := executor.FormatResponse(result)
	if err != nil {
		return handler.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return handler.CreateSuccessResponse(cmd.ID, data), nil
}
