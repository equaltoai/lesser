package advanced

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	"github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"go.uber.org/zap"
)

// Risk level constants
const (
	riskLevelHigh     = "HIGH"
	riskLevelCritical = "CRITICAL"
)

// Moderation label constants
const (
	labelViolence = "Violence"
)

// ImageAnalyzer handles image content analysis using AWS Rekognition
type ImageAnalyzer struct {
	client      *rekognition.Client
	logger      *zap.Logger
	config      *ModerationConfig
	costTracker CostTracker

	// Cache for results
	resultCache sync.Map
	cacheTTL    time.Duration
}

// GetClient returns the Rekognition client for use by video analyzer
func (ia *ImageAnalyzer) GetClient() *rekognition.Client {
	return ia.client
}

// RekognitionCostTracker interface for tracking AWS Rekognition costs
type RekognitionCostTracker interface {
	TrackRekognitionRequest(operation string, imageCount int)
}

// NewImageAnalyzer creates a new image analyzer
func NewImageAnalyzer(client *rekognition.Client, logger *zap.Logger, config *ModerationConfig, costTracker CostTracker) *ImageAnalyzer {
	return &ImageAnalyzer{
		client:      client,
		logger:      logger,
		config:      config,
		costTracker: costTracker,
		cacheTTL:    10 * time.Minute,
	}
}

// AnalyzeImage performs comprehensive image analysis
func (ia *ImageAnalyzer) AnalyzeImage(ctx context.Context, imageURL string, _ ContentMetadata) (*ImageAnalysis, error) {
	startTime := time.Now()

	// Check cache
	cacheKey := fmt.Sprintf("img:%s", imageURL)
	if cached, ok := ia.resultCache.Load(cacheKey); ok {
		if result, ok := cached.(*cachedImageResult); ok && time.Since(result.cachedAt) < ia.cacheTTL {
			ia.logger.Debug("returning cached image analysis", zap.String("imageURL", imageURL))
			return result.analysis, nil
		}
	}

	analysis := &ImageAnalysis{
		ImageURL:   imageURL,
		AnalyzedAt: time.Now(),
	}

	// Create image input
	imageInput := &types.Image{
		S3Object: &types.S3Object{
			Bucket: aws.String(ia.config.S3Bucket),
			Name:   aws.String(extractS3Key(imageURL)),
		},
	}

	// If not S3, try to use URL directly (requires setup)
	if !isS3URL(imageURL) {
		// For non-S3 URLs, you'd need to download and upload to S3 first
		// or use DetectModerationLabels with Bytes input
		return nil, fmt.Errorf("non-S3 URLs not yet supported")
	}

	// Run analyses in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	// Moderation labels (explicit content, violence, etc.)
	wg.Add(1)
	go func() {
		defer wg.Done()
		moderation, err := ia.detectModerationLabels(ctx, imageInput)
		if err != nil {
			mu.Lock()
			errors = append(errors, fmt.Errorf("moderation detection: %w", err))
			mu.Unlock()
			return
		}
		mu.Lock()
		analysis.Explicit = moderation.explicit
		analysis.Violence = moderation.violence
		mu.Unlock()
	}()

	// Object and scene detection
	if ia.config.EnableImageAnalysis {
		wg.Add(1)
		go func() {
			defer wg.Done()
			labels, err := ia.detectLabels(ctx, imageInput)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("label detection: %w", err))
				mu.Unlock()
				return
			}
			mu.Lock()
			analysis.Objects = labels.objects
			analysis.CustomLabels = labels.customLabels
			mu.Unlock()
		}()
	}

	// Text detection
	wg.Add(1)
	go func() {
		defer wg.Done()
		textDetections, err := ia.detectText(ctx, imageInput)
		if err != nil {
			mu.Lock()
			errors = append(errors, fmt.Errorf("text detection: %w", err))
			mu.Unlock()
			return
		}
		mu.Lock()
		analysis.Text = textDetections
		mu.Unlock()
	}()

	// Face detection and analysis
	wg.Add(1)
	go func() {
		defer wg.Done()
		faces, err := ia.detectFaces(ctx, imageInput)
		if err != nil {
			mu.Lock()
			errors = append(errors, fmt.Errorf("face detection: %w", err))
			mu.Unlock()
			return
		}
		mu.Lock()
		analysis.Faces = faces
		mu.Unlock()
	}()

	// Celebrity recognition
	if ia.config.EnableImageAnalysis {
		wg.Add(1)
		go func() {
			defer wg.Done()
			celebrities, err := ia.recognizeCelebrities(ctx, imageInput)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("celebrity recognition: %w", err))
				mu.Unlock()
				return
			}
			mu.Lock()
			analysis.Celebrities = celebrities
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Check for critical errors
	if len(errors) > 0 && len(errors) == 5 {
		return nil, fmt.Errorf("all analyses failed: %v", errors)
	}

	// Log non-critical errors
	for _, err := range errors {
		ia.logger.Warn("image analysis error", zap.Error(err))
	}

	analysis.ProcessingTime = time.Since(startTime)

	// Cache the result
	ia.resultCache.Store(cacheKey, &cachedImageResult{
		analysis: analysis,
		cachedAt: time.Now(),
	})

	return analysis, nil
}

// detectModerationLabels detects inappropriate content
func (ia *ImageAnalyzer) detectModerationLabels(ctx context.Context, image *types.Image) (*moderationResult, error) {
	input := &rekognition.DetectModerationLabelsInput{
		Image:         image,
		MinConfidence: aws.Float32(float32(ia.config.ConfidenceThreshold * 100)),
	}

	result, err := ia.client.DetectModerationLabels(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("detect moderation labels: %w", err)
	}

	if ia.costTracker != nil {
		if tracker, ok := ia.costTracker.(RekognitionCostTracker); ok {
			tracker.TrackRekognitionRequest("DetectModerationLabels", 1)
		}
	}

	modResult := &moderationResult{
		explicit: ExplicitContent{},
		violence: ViolenceDetection{},
	}

	// Process moderation labels
	for _, label := range result.ModerationLabels {
		confidence := float64(*label.Confidence) / 100.0
		labelName := aws.ToString(label.Name)

		switch labelName {
		case "Explicit Nudity", "Nudity":
			modResult.explicit.IsExplicit = true
			modResult.explicit.NudityScore = maxFloat64(modResult.explicit.NudityScore, confidence)
			modResult.explicit.Confidence = maxFloat64(modResult.explicit.Confidence, confidence)

		case "Suggestive":
			modResult.explicit.SuggestiveScore = maxFloat64(modResult.explicit.SuggestiveScore, confidence)

		case labelViolence:
			modResult.violence.HasViolence = true
			modResult.violence.ViolenceScore = maxFloat64(modResult.violence.ViolenceScore, confidence)
			modResult.violence.Confidence = maxFloat64(modResult.violence.Confidence, confidence)

		case "Visually Disturbing":
			modResult.explicit.VisuallyDisturbing = maxFloat64(modResult.explicit.VisuallyDisturbing, confidence)

		case "Weapons":
			modResult.violence.WeaponsDetected = append(modResult.violence.WeaponsDetected,
				fmt.Sprintf("%s (%.2f)", aws.ToString(label.ParentName), confidence))
		}

		// Check parent labels too
		if label.ParentName != nil {
			parentName := aws.ToString(label.ParentName)
			if parentName == labelViolence {
				modResult.violence.HasViolence = true
			}
		}
	}

	return modResult, nil
}

// detectLabels detects objects and scenes
func (ia *ImageAnalyzer) detectLabels(ctx context.Context, image *types.Image) (*labelResult, error) {
	input := &rekognition.DetectLabelsInput{
		Image:         image,
		MaxLabels:     aws.Int32(50),
		MinConfidence: aws.Float32(float32(ia.config.ConfidenceThreshold * 100)),
	}

	result, err := ia.client.DetectLabels(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("detect labels: %w", err)
	}

	if ia.costTracker != nil {
		if tracker, ok := ia.costTracker.(RekognitionCostTracker); ok {
			tracker.TrackRekognitionRequest("DetectLabels", 1)
		}
	}

	labelRes := &labelResult{
		objects:      []ObjectDetection{},
		customLabels: []CustomLabel{},
	}

	for _, label := range result.Labels {
		confidence := float64(*label.Confidence) / 100.0

		// Create object detection
		obj := ObjectDetection{
			Name:       aws.ToString(label.Name),
			Confidence: confidence,
			Parents:    []string{},
		}

		// Add parent labels
		for _, parent := range label.Parents {
			obj.Parents = append(obj.Parents, aws.ToString(parent.Name))
		}

		// Add bounding box if available
		if len(label.Instances) > 0 && label.Instances[0].BoundingBox != nil {
			bbox := label.Instances[0].BoundingBox
			obj.BoundingBox = BoundingBox{
				Left:   float64(*bbox.Left),
				Top:    float64(*bbox.Top),
				Width:  float64(*bbox.Width),
				Height: float64(*bbox.Height),
			}
		}

		labelRes.objects = append(labelRes.objects, obj)

		// Also add as custom label for categorization
		labelRes.customLabels = append(labelRes.customLabels, CustomLabel{
			Name:       obj.Name,
			Confidence: confidence,
			Parents:    obj.Parents,
		})
	}

	return labelRes, nil
}

// detectText detects text in images
func (ia *ImageAnalyzer) detectText(ctx context.Context, image *types.Image) ([]TextInImage, error) {
	input := &rekognition.DetectTextInput{
		Image: image,
	}

	result, err := ia.client.DetectText(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("detect text: %w", err)
	}

	if ia.costTracker != nil {
		if tracker, ok := ia.costTracker.(RekognitionCostTracker); ok {
			tracker.TrackRekognitionRequest("DetectText", 1)
		}
	}

	textDetections := []TextInImage{}

	for _, detection := range result.TextDetections {
		if detection.Type == types.TextTypesLine || detection.Type == types.TextTypesWord {
			confidence := float64(*detection.Confidence) / 100.0

			textDetection := TextInImage{
				Text:       aws.ToString(detection.DetectedText),
				Confidence: confidence,
			}

			// Add bounding box
			if detection.Geometry != nil && detection.Geometry.BoundingBox != nil {
				bbox := detection.Geometry.BoundingBox
				textDetection.BoundingBox = BoundingBox{
					Left:   float64(*bbox.Left),
					Top:    float64(*bbox.Top),
					Width:  float64(*bbox.Width),
					Height: float64(*bbox.Height),
				}
			}

			textDetections = append(textDetections, textDetection)
		}
	}

	return textDetections, nil
}

// detectFaces detects and analyzes faces
func (ia *ImageAnalyzer) detectFaces(ctx context.Context, image *types.Image) ([]FaceAnalysis, error) {
	input := &rekognition.DetectFacesInput{
		Image:      image,
		Attributes: []types.Attribute{types.AttributeAll},
	}

	result, err := ia.client.DetectFaces(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("detect faces: %w", err)
	}

	if ia.costTracker != nil {
		if tracker, ok := ia.costTracker.(RekognitionCostTracker); ok {
			tracker.TrackRekognitionRequest("DetectFaces", 1)
		}
	}

	faces := []FaceAnalysis{}

	for _, face := range result.FaceDetails {
		faceAnalysis := FaceAnalysis{
			Confidence: float64(*face.Confidence) / 100.0,
			Emotions:   []Emotion{},
		}

		// Add bounding box
		if face.BoundingBox != nil {
			faceAnalysis.BoundingBox = BoundingBox{
				Left:   float64(*face.BoundingBox.Left),
				Top:    float64(*face.BoundingBox.Top),
				Width:  float64(*face.BoundingBox.Width),
				Height: float64(*face.BoundingBox.Height),
			}
		}

		// Add emotions
		for _, emotion := range face.Emotions {
			faceAnalysis.Emotions = append(faceAnalysis.Emotions, Emotion{
				Type:       string(emotion.Type),
				Confidence: float64(*emotion.Confidence) / 100.0,
			})
		}

		// Add age range
		if face.AgeRange != nil {
			faceAnalysis.AgeRange = AgeRange{
				Low:  int(*face.AgeRange.Low),
				High: int(*face.AgeRange.High),
			}
		}

		// Add gender
		if face.Gender != nil {
			faceAnalysis.Gender = Gender{
				Value:      string(face.Gender.Value),
				Confidence: float64(*face.Gender.Confidence) / 100.0,
			}
		}

		faces = append(faces, faceAnalysis)
	}

	return faces, nil
}

// recognizeCelebrities recognizes celebrities in images
func (ia *ImageAnalyzer) recognizeCelebrities(ctx context.Context, image *types.Image) ([]CelebrityMatch, error) {
	input := &rekognition.RecognizeCelebritiesInput{
		Image: image,
	}

	result, err := ia.client.RecognizeCelebrities(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("recognize celebrities: %w", err)
	}

	if ia.costTracker != nil {
		if tracker, ok := ia.costTracker.(RekognitionCostTracker); ok {
			tracker.TrackRekognitionRequest("RecognizeCelebrities", 1)
		}
	}

	celebrities := []CelebrityMatch{}

	for _, celeb := range result.CelebrityFaces {
		match := CelebrityMatch{
			Name:       aws.ToString(celeb.Name),
			Confidence: float64(*celeb.MatchConfidence) / 100.0,
			URLs:       celeb.Urls,
		}

		// Add bounding box
		if celeb.Face != nil && celeb.Face.BoundingBox != nil {
			bbox := celeb.Face.BoundingBox
			match.BoundingBox = BoundingBox{
				Left:   float64(*bbox.Left),
				Top:    float64(*bbox.Top),
				Width:  float64(*bbox.Width),
				Height: float64(*bbox.Height),
			}
		}

		celebrities = append(celebrities, match)
	}

	return celebrities, nil
}

// AnalyzeImageContent performs content-specific analysis
func (ia *ImageAnalyzer) AnalyzeImageContent(ctx context.Context, imageURL string, textContent string) (*CombinedAnalysis, error) {
	// This method combines image and text analysis for better context
	imageAnalysis, err := ia.AnalyzeImage(ctx, imageURL, ContentMetadata{})
	if err != nil {
		return nil, err
	}

	combined := &CombinedAnalysis{
		ImageAnalysis: imageAnalysis,
		Flags:         []string{},
	}

	// Check for concerning combinations
	if imageAnalysis.Violence.HasViolence && containsThreatLanguage(textContent) {
		combined.Flags = append(combined.Flags, "VIOLENT_THREAT")
		combined.RiskLevel = riskLevelHigh
	}

	if imageAnalysis.Explicit.IsExplicit && isTargetedAtMinors(textContent) {
		combined.Flags = append(combined.Flags, "CHILD_SAFETY_CONCERN")
		combined.RiskLevel = riskLevelCritical
	}

	// Check for doxxing
	if len(imageAnalysis.Text) > 0 && containsPersonalInfo(textContent) {
		for _, text := range imageAnalysis.Text {
			if looksLikeAddress(text.Text) || looksLikePhoneNumber(text.Text) {
				combined.Flags = append(combined.Flags, "POSSIBLE_DOXXING")
				combined.RiskLevel = riskLevelHigh
				break
			}
		}
	}

	// Check for harassment
	if len(imageAnalysis.Faces) > 0 && containsHarassmentLanguage(textContent) {
		combined.Flags = append(combined.Flags, "TARGETED_HARASSMENT")
		combined.RiskLevel = riskLevelHigh
	}

	return combined, nil
}

// Helper types and functions

type cachedImageResult struct {
	analysis *ImageAnalysis
	cachedAt time.Time
}

type moderationResult struct {
	explicit ExplicitContent
	violence ViolenceDetection
}

type labelResult struct {
	objects      []ObjectDetection
	customLabels []CustomLabel
}

// CombinedAnalysis represents combined image and text analysis results
type CombinedAnalysis struct {
	ImageAnalysis *ImageAnalysis
	TextAnalysis  *ContentAnalysis
	Flags         []string
	RiskLevel     string
}

func extractS3Key(url string) string {
	// Extract S3 key from URL
	// This is a simplified version - in production, use proper URL parsing
	if idx := strings.Index(url, ".amazonaws.com/"); idx > 0 {
		return url[idx+15:]
	}
	return url
}

func isS3URL(url string) bool {
	return strings.Contains(url, ".s3.") && strings.Contains(url, ".amazonaws.com")
}

func containsThreatLanguage(text string) bool {
	threatWords := []string{"kill", "hurt", "attack", "destroy"}
	lowerText := strings.ToLower(text)
	for _, word := range threatWords {
		if strings.Contains(lowerText, word) {
			return true
		}
	}
	return false
}

func isTargetedAtMinors(text string) bool {
	minorWords := []string{"child", "kid", "minor", "underage", "young"}
	lowerText := strings.ToLower(text)
	for _, word := range minorWords {
		if strings.Contains(lowerText, word) {
			return true
		}
	}
	return false
}

func containsHarassmentLanguage(text string) bool {
	harassmentWords := []string{"ugly", "fat", "stupid", "kill yourself", "die"}
	lowerText := strings.ToLower(text)
	for _, word := range harassmentWords {
		if strings.Contains(lowerText, word) {
			return true
		}
	}
	return false
}

func looksLikeAddress(text string) bool {
	// Simple check for address patterns
	addressKeywords := []string{"street", "st", "avenue", "ave", "road", "rd", "drive", "dr"}
	lowerText := strings.ToLower(text)
	for _, keyword := range addressKeywords {
		if strings.Contains(lowerText, keyword) {
			return true
		}
	}
	return false
}

func looksLikePhoneNumber(text string) bool {
	// Simple check for phone number patterns
	digitCount := 0
	for _, char := range text {
		if char >= '0' && char <= '9' {
			digitCount++
		}
	}
	return digitCount >= 10 && digitCount <= 15
}
