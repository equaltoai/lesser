package repositories

import (
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
)

// permitInstanceCountMaintenance registers permissive mock expectations for
// the best-effort O(1) instance-count maintenance paths (see
// instance_counts.go): TOTAL_USERS/TOTAL_DOMAINS bumps, per-domain counters,
// and the activity-day rollup. Mock-based tests exercising write paths that
// now maintain counters call this so they keep pinning their own behavior
// without enumerating counter ops.
func permitInstanceCountMaintenance(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
	ub := new(mocks.MockUpdateBuilder)
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("IfNotExists").Return(mockQuery).Maybe()
	mockQuery.On("UpdateBuilder").Return(ub).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	ub.On("Add", mock.Anything, mock.Anything).Return(ub).Maybe()
	ub.On("Set", mock.Anything, mock.Anything).Return(ub).Maybe()
	ub.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(ub).Maybe()
	ub.On("Execute").Return(nil).Maybe()
	ub.On("ExecuteWithResult", mock.Anything).Return(nil).Maybe()
}
