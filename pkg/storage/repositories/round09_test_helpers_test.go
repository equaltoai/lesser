package repositories

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

func newMockDBQuery() (*mocks.MockDB, *mocks.MockQuery) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()

	return mockDB, mockQuery
}

func requireNoMockExpectations(t *testing.T, mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
	t.Helper()
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
