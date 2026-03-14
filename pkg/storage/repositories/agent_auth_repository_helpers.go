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

func markChallengeUsed(
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	challengeID string,
	now time.Time,
	existing interface{},
	warnMessage string,
) error {
	if db == nil {
		return storage.ErrDatabaseConnectionFailed
	}

	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return storage.ErrInvalidInput
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if err := db.Model(existing).WithContext(ctx).UpdateBuilder().
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
