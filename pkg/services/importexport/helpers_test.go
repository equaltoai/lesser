package importexport

import (
	stderrors "errors"
	"testing"
	"time"

	serviceerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_validateCreateExportCommand(t *testing.T) {
	t.Run("user_not_found", func(t *testing.T) {
		accountRepo := &MockAccountRepository{}
		accountRepo.On("GetAccount", mock.Anything, "missing").Return((*storage.Account)(nil), stderrors.New("not found"))

		svc := newTestService(nil, nil, accountRepo, nil, nil, nil, zap.NewNop(), "example.com")
		err := svc.validateCreateExportCommand(&CreateExportCommand{Username: "missing"})
		require.ErrorIs(t, err, serviceerrors.ErrUserNotFound)
	})

	t.Run("invalid_date_range_order", func(t *testing.T) {
		accountRepo := &MockAccountRepository{}
		accountRepo.On("GetAccount", mock.Anything, "alice").Return(&storage.Account{}, nil)

		svc := newTestService(nil, nil, accountRepo, nil, nil, nil, zap.NewNop(), "example.com")
		err := svc.validateCreateExportCommand(&CreateExportCommand{
			Username: "alice",
			DateRange: &DateRange{
				Start: time.Now(),
				End:   time.Now().Add(-1 * time.Hour),
			},
		})
		require.ErrorIs(t, err, serviceerrors.ErrInvalidDateRangeOrder)
	})

	t.Run("invalid_date_range_future_end", func(t *testing.T) {
		accountRepo := &MockAccountRepository{}
		accountRepo.On("GetAccount", mock.Anything, "alice").Return(&storage.Account{}, nil)

		svc := newTestService(nil, nil, accountRepo, nil, nil, nil, zap.NewNop(), "example.com")
		err := svc.validateCreateExportCommand(&CreateExportCommand{
			Username: "alice",
			DateRange: &DateRange{
				Start: time.Now().Add(-24 * time.Hour),
				End:   time.Now().Add(1 * time.Hour),
			},
		})
		require.ErrorIs(t, err, serviceerrors.ErrInvalidDateRangeFuture)
	})

	t.Run("ok", func(t *testing.T) {
		accountRepo := &MockAccountRepository{}
		accountRepo.On("GetAccount", mock.Anything, "alice").Return(&storage.Account{}, nil)

		svc := newTestService(nil, nil, accountRepo, nil, nil, nil, zap.NewNop(), "example.com")
		require.NoError(t, svc.validateCreateExportCommand(&CreateExportCommand{Username: "alice"}))
	})
}

func TestService_createExportEvent(t *testing.T) {
	svc := &Service{logger: zap.NewNop()}
	export := &models.Export{ID: "exp1", Username: "alice"}

	event := svc.createExportEvent("export.created", export)
	assert.Equal(t, "export.created", event.Type)
	assert.Equal(t, "user", event.Stream)
	assert.False(t, event.Timestamp.IsZero())
	assert.Equal(t, export, event.Payload["export"])
}

func TestService_createProgressEvent(t *testing.T) {
	svc := &Service{logger: zap.NewNop()}
	export := &models.Export{ID: "exp1", Username: "alice"}

	event := svc.createProgressEvent("export.progress", export, 50, 200)
	assert.Equal(t, "export.progress", event.Type)
	assert.Equal(t, "user", event.Stream)
	assert.Equal(t, "exp1", event.Payload["export_id"])
	assert.Equal(t, 50, event.Payload["processed"])
	assert.Equal(t, 200, event.Payload["total"])
	assert.Equal(t, 25.0, event.Payload["percent"])
}

func TestConvertStringMapToAny(t *testing.T) {
	out := convertStringMapToAny(map[string]string{"a": "1", "b": "2"})
	assert.Equal(t, map[string]any{"a": "1", "b": "2"}, out)
}
