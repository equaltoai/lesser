package streaming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStreamingError_Error(t *testing.T) {
	err := &StreamingError{Message: "boom"}
	assert.Equal(t, "boom", err.Error())
}

func TestGetQualityInfo_AndQualitiesByBandwidth(t *testing.T) {
	assert.Equal(t, "1920x1080", GetQualityInfo(Quality1080p).Resolution)
	assert.Equal(t, Quality480p, GetQualityInfo("unknown").Quality)

	assert.Empty(t, GetQualitiesByBandwidth(100))
	assert.Equal(t, []Quality{Quality240p}, GetQualitiesByBandwidth(500))
	assert.Equal(t, []Quality{Quality240p, Quality360p, Quality480p}, GetQualitiesByBandwidth(2500))
	assert.Contains(t, GetQualitiesByBandwidth(25000), Quality4K)
}
