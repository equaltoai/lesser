package repositories

import "github.com/equaltoai/lesser/pkg/storage/models"

// NewWebSocketConnectionValidationService returns a validation service for WebSocket connection records.
func NewWebSocketConnectionValidationService() ValidationService {
	return newUsernameOptionalValidationService(func(model BaseModel) (string, *string, bool) {
		conn, ok := model.(*models.WebSocketConnection)
		if !ok || conn == nil {
			return "", nil, false
		}
		return conn.UserID, &conn.Username, true
	})
}
