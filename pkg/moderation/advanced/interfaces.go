package advanced

import (
	"context"
)

// TextAnalyzerInterface defines the interface for text analysis
type TextAnalyzerInterface interface {
	AnalyzeText(ctx context.Context, text string, metadata ContentMetadata) (*ContentAnalysis, error)
}

// ImageAnalyzerInterface defines the interface for image analysis
type ImageAnalyzerInterface interface {
	AnalyzeImage(ctx context.Context, imageURL string, metadata ContentMetadata) (*ImageAnalysis, error)
}

// VideoAnalyzerInterface defines the interface for video analysis
type VideoAnalyzerInterface interface {
	AnalyzeVideo(ctx context.Context, videoURL string, metadata ContentMetadata) (*VideoAnalysis, error)
}

// Ensure implementations satisfy interfaces
var (
	_ TextAnalyzerInterface  = (*TextAnalyzer)(nil)
	_ TextAnalyzerInterface  = (*NoOpTextAnalyzer)(nil)
	_ ImageAnalyzerInterface = (*ImageAnalyzer)(nil)
	_ ImageAnalyzerInterface = (*NoOpImageAnalyzer)(nil)
	_ VideoAnalyzerInterface = (*NoOpVideoAnalyzer)(nil)
)
