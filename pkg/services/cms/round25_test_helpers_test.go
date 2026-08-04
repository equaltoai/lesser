package cms

import (
	"testing"

	"github.com/stretchr/testify/mock"
	dynamormMocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
)

func newCMSMockDB(t *testing.T) (*dynamormMocks.MockDB, *dynamormMocks.MockQuery) {
	t.Helper()

	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()

	q.On("IfNotExists").Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Index", mock.Anything).Return(q).Maybe()
	q.On("Limit", mock.Anything).Return(q).Maybe()
	q.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Cursor", mock.Anything).Return(q).Maybe()

	return db, q
}
