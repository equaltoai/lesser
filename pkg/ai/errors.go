package ai

import "errors"

// AI processing errors
var (
	// ErrContentExtractionFailed is returned when content extraction from stream fails
	ErrContentExtractionFailed = errors.New("failed to extract content")

	// ErrAnalysisFailed is returned when AI content analysis fails
	ErrAnalysisFailed = errors.New("failed to analyze content")

	// ErrAnalysisSaveFailed is returned when saving AI analysis fails
	ErrAnalysisSaveFailed = errors.New("failed to save analysis")

	// ErrStreamUnmarshalFailed is returned when unmarshaling stream data fails
	ErrStreamUnmarshalFailed = errors.New("failed to unmarshal stream image")

	// ErrInvalidObjectPK is returned when object primary key is invalid
	ErrInvalidObjectPK = errors.New("invalid object PK")

	// ErrNotAnalyzableType is returned when object type cannot be analyzed
	ErrNotAnalyzableType = errors.New("not an analyzable type")

	// ErrNoJSONFoundInResponse is returned when no JSON is found in AI model response
	ErrNoJSONFoundInResponse = errors.New("no JSON found in response")

	// ErrLocalNetworkAccess is returned when attempting to access local networks
	ErrLocalNetworkAccess = errors.New("access to local networks not allowed")

	// ErrInvalidURLScheme is returned when URL has an invalid scheme
	ErrInvalidURLScheme = errors.New("invalid URL scheme")

	// ErrImageDownloadHTTP is returned when image download fails with HTTP error
	ErrImageDownloadHTTP = errors.New("failed to download image")

	// ErrInvalidEmbeddingResponse is returned when embedding response format is invalid
	ErrInvalidEmbeddingResponse = errors.New("invalid embedding response format")

	// ErrGetAnalysisDeprecated is returned when deprecated GetAnalysis method is called
	ErrGetAnalysisDeprecated = errors.New("GetAnalysis is deprecated - use service layer")

	// ErrGetAnalysisStatsDeprecated is returned when deprecated GetAnalysisStats method is called
	ErrGetAnalysisStatsDeprecated = errors.New("GetAnalysisStats is deprecated - use service layer")
)
