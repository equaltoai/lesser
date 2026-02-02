package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaMetadata_LifecycleValidationAndHelpers(t *testing.T) {
	t.Run("BeforeCreate sets defaults, keys, and validates", func(t *testing.T) {
		m := &MediaMetadata{
			MediaID: "m1",
		}

		require.NoError(t, m.BeforeCreate())
		assert.Equal(t, StatusPending, m.Status)
		assert.False(t, m.ProcessedAt.IsZero())
		assert.False(t, m.CreatedAt.IsZero())
		assert.True(t, m.CreatedAt.Equal(m.UpdatedAt))

		assert.Equal(t, "MEDIA#m1", m.PK)
		assert.Equal(t, SKMetadata, m.SK)
		assert.Equal(t, "STATUS#"+StatusPending, m.GSI1PK)
		assert.Contains(t, m.GSI1SK, "PROCESSED#")

		assert.Equal(t, MainTableName, m.TableName())
		assert.Equal(t, MainTableName, (QualityCodecInfo{}).TableName())
		assert.Equal(t, m.PK, m.GetPK())
		assert.Equal(t, m.SK, m.GetSK())
	})

	t.Run("BeforeUpdate sets ProcessedAt when completing and missing timestamp", func(t *testing.T) {
		m := &MediaMetadata{
			MediaID:     "m1",
			Status:      StatusComplete,
			ProcessedAt: time.Time{},
		}
		require.NoError(t, m.BeforeUpdate())
		assert.False(t, m.ProcessedAt.IsZero())
		assert.True(t, m.ProcessedAt.Equal(m.UpdatedAt))
	})

	t.Run("Validate rejects invalid values", func(t *testing.T) {
		m := &MediaMetadata{
			MediaID: "m1",
			Status:  "nope",
		}
		assert.ErrorIs(t, m.Validate(), ErrMediaMetadataInvalidStatus)

		m = &MediaMetadata{MediaID: "m1", Status: StatusPending, Width: -1}
		assert.ErrorIs(t, m.Validate(), ErrMediaMetadataWidthNegative)

		m = &MediaMetadata{MediaID: "m1", Status: StatusPending, Height: -1}
		assert.ErrorIs(t, m.Validate(), ErrMediaMetadataHeightNegative)

		m = &MediaMetadata{MediaID: "m1", Status: StatusPending, Duration: -1}
		assert.ErrorIs(t, m.Validate(), ErrMediaMetadataDurationNegative)

		m = &MediaMetadata{MediaID: "m1", Status: StatusPending, FileSize: -1}
		assert.ErrorIs(t, m.Validate(), ErrMediaMetadataFileSizeNegative)
	})

	t.Run("Status helpers and setters", func(t *testing.T) {
		m := &MediaMetadata{MediaID: "m1", Status: StatusPending}
		assert.True(t, m.IsPending())
		assert.False(t, m.IsProcessing())

		m.SetProcessing()
		assert.True(t, m.IsProcessing())

		m.SetComplete()
		assert.True(t, m.IsComplete())
		assert.False(t, m.ProcessedAt.IsZero())

		before := time.Now()
		m.SetFailed()
		assert.True(t, m.IsFailed())
		assert.True(t, time.Unix(m.TTL, 0).After(before.Add(6*24*time.Hour)))
	})

	t.Run("Quality helpers", func(t *testing.T) {
		m := &MediaMetadata{}
		assert.False(t, m.HasQuality("720p"))
		m.AddQuality("720p")
		m.AddQuality("720p")
		assert.True(t, m.HasQuality("720p"))
		assert.Len(t, m.AvailableQualities, 1)
	})

	t.Run("Codec info helpers", func(t *testing.T) {
		m := &MediaMetadata{}
		_, ok := m.GetCodecInfo("720p")
		assert.False(t, ok)

		info := QualityCodecInfo{VideoCodec: "avc1", AudioCodec: "mp4a", Bandwidth: 1}
		m.SetCodecInfo("720p", info)
		got, ok := m.GetCodecInfo("720p")
		assert.True(t, ok)
		assert.Equal(t, info, got)
	})
}
