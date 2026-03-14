package repositories

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

type keyedPreparableModel interface {
	BeforeCreate() error
	GetPK() string
	GetSK() string
}

type keyedStoredModel interface {
	GetPK() string
	GetSK() string
}

func createPreparedModel[T keyedPreparableModel](
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	model T,
	prepareMessage string,
	createMessage string,
	logFields func(T) []zap.Field,
) error {
	if db == nil {
		return storage.ErrDatabaseConnectionFailed
	}
	if isNilModel(model) {
		return storage.ErrInvalidInput
	}

	fields := logFields(model)
	if err := model.BeforeCreate(); err != nil {
		logger.Error(prepareMessage, append(fields, zap.Error(err))...)
		return err
	}

	fields = append(fields,
		zap.String("pk", model.GetPK()),
		zap.String("sk", model.GetSK()),
	)
	if err := db.WithContext(ctx).Model(model).IfNotExists().Create(); err != nil {
		logger.Error(createMessage, append(fields, zap.Error(err))...)
		return err
	}

	return nil
}

func isNilModel[T any](model T) bool {
	value := reflect.ValueOf(model)
	if !value.IsValid() {
		return true
	}

	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func keyedUpdateBuilder[T keyedStoredModel](ctx context.Context, db core.DB, model T) (core.UpdateBuilder, error) {
	if db == nil {
		return nil, storage.ErrDatabaseConnectionFailed
	}
	if isNilModel(model) {
		return nil, storage.ErrInvalidInput
	}

	if updater, ok := any(model).(interface{ UpdateKeys() error }); ok {
		if strings.TrimSpace(model.GetPK()) == "" || strings.TrimSpace(model.GetSK()) == "" {
			if err := updater.UpdateKeys(); err != nil {
				return nil, err
			}
		}
	}

	pk := strings.TrimSpace(model.GetPK())
	sk := strings.TrimSpace(model.GetSK())
	if pk == "" || sk == "" {
		return nil, storage.ErrInvalidInput
	}

	return db.WithContext(ctx).
		Model(model).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		UpdateBuilder(), nil
}

func markChallengeUsed(
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	challengeID string,
	now time.Time,
	existing keyedStoredModel,
	warnMessage string,
) error {
	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return storage.ErrInvalidInput
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	update, err := keyedUpdateBuilder(ctx, db, existing)
	if err != nil {
		return err
	}

	if err := update.
		Set("Used", true).
		Condition("Used", "=", false).
		Condition("TTL", ">", now.Unix()).
		Execute(); err != nil {
		logger.Warn(warnMessage,
			zap.String("challenge_id", challengeID),
			zap.Error(err))
		return err
	}

	return nil
}
