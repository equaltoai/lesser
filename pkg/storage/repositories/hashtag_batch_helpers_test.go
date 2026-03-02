package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestDeleteOldRecordsBatch_UnknownModelType(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)

	deleted, err := deleteOldRecordsBatch(ctx, mockDB, zap.NewNop(), time.Now(), BatchDeleteConfig{
		ModelType:   "nope",
		ErrorPrefix: "irrelevant",
		BatchSize:   2,
		QueryLimit:  10,
		FilterField: "UpdatedAt",
	})

	assert.Equal(t, 0, deleted)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrHashtagBatchUnknownModelType)
}

func TestDeleteOldRecordsBatch_TTLNoops(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	for _, modelType := range []string{"hashtag_trend", "trending_hashtag", "hashtag_usage"} {
		deleted, err := deleteOldRecordsBatch(ctx, mockDB, zap.NewNop(), time.Now(), BatchDeleteConfig{
			ModelType:   modelType,
			ErrorPrefix: "irrelevant",
			BatchSize:   2,
			QueryLimit:  10,
			FilterField: "UpdatedAt",
		})
		assert.NoError(t, err)
		assert.Equal(t, 0, deleted)
	}
}
