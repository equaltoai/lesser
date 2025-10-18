package ai

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// AI processing errors - consolidated to use centralized error system
var (
	// ErrContentExtractionFailed is returned when content extraction from stream fails
	ErrContentExtractionFailed = errors.ProcessingFailed("content extraction", stdErrors.New("content extraction failed"))

	// ErrAnalysisFailed is returned when AI content analysis fails
	ErrAnalysisFailed = errors.ProcessingFailed("AI content analysis", stdErrors.New("AI content analysis failed"))

	// ErrAnalysisSaveFailed is returned when saving AI analysis fails
	ErrAnalysisSaveFailed = errors.FailedToSave("AI analysis", stdErrors.New("AI analysis save failed"))

	// ErrStreamUnmarshalFailed is returned when unmarshaling stream data fails
	ErrStreamUnmarshalFailed = errors.UnmarshalingFailed("stream image", stdErrors.New("stream image unmarshaling failed"))

	// ErrInvalidObjectPK is returned when object primary key is invalid
	ErrInvalidObjectPK = errors.IDInvalidFormat("object")

	// ErrNotAnalyzableType is returned when object type cannot be analyzed
	ErrNotAnalyzableType = errors.InvalidValue("object_type", []string{"image", "video", "audio", "text"}, "")

	// ErrNoJSONFoundInResponse is returned when no JSON is found in AI model response
	ErrNoJSONFoundInResponse = errors.ParsingFailed("JSON in AI response", stdErrors.New("JSON parsing failed"))

	// ErrLocalNetworkAccess is returned when attempting to access local networks
	ErrLocalNetworkAccess = errors.URLHostNotAllowed("", "local network")

	// ErrInvalidURLScheme is returned when URL has an invalid scheme
	ErrInvalidURLScheme = errors.URLSchemeNotAllowed("", "")

	// ErrImageDownloadHTTP is returned when image download fails with HTTP error
	ErrImageDownloadHTTP = errors.NetworkError("image download", stdErrors.New("image download failed"))

	// ErrInvalidEmbeddingResponse is returned when embedding response format is invalid
	ErrInvalidEmbeddingResponse = errors.ParsingFailed("embedding response", stdErrors.New("embedding response parsing failed"))

	// ErrGetAnalysisDeprecated is returned when deprecated GetAnalysis method is called
	ErrGetAnalysisDeprecated = errors.ProcessingFailed("GetAnalysis is deprecated", stdErrors.New("GetAnalysis is deprecated"))

	// ErrGetAnalysisStatsDeprecated is returned when deprecated GetAnalysisStats method is called
	ErrGetAnalysisStatsDeprecated = errors.ProcessingFailed("GetAnalysisStats is deprecated", stdErrors.New("GetAnalysisStats is deprecated"))
)
