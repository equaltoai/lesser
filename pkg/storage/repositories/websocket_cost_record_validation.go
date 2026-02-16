package repositories

import "github.com/equaltoai/lesser/pkg/storage/models"

func newWebSocketCostRecordValidationService() ValidationService {
	return newUsernameOptionalValidationService(func(model BaseModel) (string, *string, bool) {
		record, ok := model.(*models.WebSocketCostRecord)
		if !ok || record == nil {
			return "", nil, false
		}
		return record.UserID, &record.Username, true
	})
}
