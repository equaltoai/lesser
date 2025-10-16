package graph

import (
	"strings"

	"github.com/equaltoai/lesser/graph/model"
)

const (
	quality480p  = "480p"
	quality720p  = "720p"
	quality1080p = "1080p"
	quality2160p = "2160p"
	qualityAuto  = "auto"
)

// mapQualityToEnum maps quality string to StreamQuality enum
func mapQualityToEnum(quality string) model.StreamQuality {
	quality = strings.ToLower(quality)
	switch quality {
	case "240p", "low":
		return model.StreamQualityLow
	case "360p", quality480p, "medium":
		return model.StreamQualityMedium
	case quality720p, "high":
		return model.StreamQualityHigh
	case quality1080p, quality2160p, "4k", "ultra":
		return model.StreamQualityUltra
	case qualityAuto:
		return model.StreamQualityAuto
	default:
		return model.StreamQualityMedium
	}
}
