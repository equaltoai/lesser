package advanced

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type rekognitionStubTransport struct {
	mu sync.Mutex

	responses map[string]map[string]any
	status    map[string]int
	err       map[string]error

	calls map[string]int
}

func newRekognitionStubTransport() *rekognitionStubTransport {
	return &rekognitionStubTransport{
		responses: make(map[string]map[string]any),
		status:    make(map[string]int),
		err:       make(map[string]error),
		calls:     make(map[string]int),
	}
}

func (t *rekognitionStubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target := req.Header.Get("X-Amz-Target")
	if !strings.HasPrefix(target, "RekognitionService.") {
		return jsonResponse(http.StatusBadRequest, map[string]any{"Message": "unexpected target"})
	}

	op := strings.TrimPrefix(target, "RekognitionService.")

	t.mu.Lock()
	t.calls[op]++
	resp := t.responses[op]
	status := t.status[op]
	err := t.err[op]
	t.mu.Unlock()

	if err != nil {
		return nil, err
	}
	if status == 0 {
		status = http.StatusOK
	}
	if resp == nil {
		resp = map[string]any{}
	}

	return jsonResponse(status, resp)
}

func (t *rekognitionStubTransport) callCount(op string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls[op]
}

func TestImageAnalyzer_AnalyzeImage_ProcessesResultsAndCaches(t *testing.T) {
	ctx := context.Background()

	transport := newRekognitionStubTransport()
	transport.responses["DetectModerationLabels"] = map[string]any{
		"ModerationLabels": []any{
			map[string]any{"Name": "Explicit Nudity", "Confidence": 99.0},
			map[string]any{"Name": "Suggestive", "Confidence": 80.0},
			map[string]any{"Name": labelViolence, "Confidence": 70.0},
			map[string]any{"Name": "Visually Disturbing", "Confidence": 60.0},
			map[string]any{"Name": "Weapons", "ParentName": labelViolence, "Confidence": 50.0},
		},
	}
	transport.responses["DetectLabels"] = map[string]any{
		"Labels": []any{
			map[string]any{
				"Name":       "Person",
				"Confidence": 99.0,
				"Parents": []any{
					map[string]any{"Name": "Human"},
				},
				"Instances": []any{
					map[string]any{"BoundingBox": map[string]any{"Left": 0.1, "Top": 0.2, "Width": 0.3, "Height": 0.4}},
				},
			},
		},
	}
	transport.responses["DetectText"] = map[string]any{
		"TextDetections": []any{
			map[string]any{
				"DetectedText": "123 Street",
				"Confidence":   99.0,
				"Type":         "LINE",
				"Geometry":     map[string]any{"BoundingBox": map[string]any{"Left": 0.1, "Top": 0.2, "Width": 0.3, "Height": 0.4}},
			},
			map[string]any{"DetectedText": "ignored", "Confidence": 99.0, "Type": "OTHER"},
		},
	}
	transport.responses["DetectFaces"] = map[string]any{
		"FaceDetails": []any{
			map[string]any{
				"Confidence":  99.0,
				"BoundingBox": map[string]any{"Left": 0.1, "Top": 0.2, "Width": 0.3, "Height": 0.4},
				"Emotions": []any{
					map[string]any{"Type": "HAPPY", "Confidence": 98.0},
				},
				"AgeRange": map[string]any{"Low": 10, "High": 20},
				"Gender":   map[string]any{"Value": "Male", "Confidence": 99.0},
			},
		},
	}
	transport.responses["RecognizeCelebrities"] = map[string]any{
		"CelebrityFaces": []any{
			map[string]any{
				"Name":            "Famous",
				"MatchConfidence": 97.0,
				"Urls":            []any{"https://example.com"},
				"Face":            map[string]any{"BoundingBox": map[string]any{"Left": 0.1, "Top": 0.2, "Width": 0.3, "Height": 0.4}},
			},
		},
	}

	cfg := DefaultModerationConfig()
	cfg.EnableImageAnalysis = true
	cfg.S3Bucket = "test-bucket"

	client := rekognition.NewFromConfig(awsConfigForStub(transport))
	ia := NewImageAnalyzer(client, zap.NewNop(), cfg, nil)
	ia.fetchImageBytes = func(context.Context, string) ([]byte, error) { return []byte{1, 2, 3}, nil }

	analysis, err := ia.AnalyzeImage(ctx, "https://example.com/image.png", ContentMetadata{})
	require.NoError(t, err)
	require.NotNil(t, analysis)

	assert.True(t, analysis.Explicit.IsExplicit)
	assert.True(t, analysis.Violence.HasViolence)
	assert.NotEmpty(t, analysis.Violence.WeaponsDetected)
	require.Len(t, analysis.Objects, 1)
	assert.Equal(t, "Person", analysis.Objects[0].Name)
	require.Len(t, analysis.Text, 1)
	assert.Equal(t, "123 Street", analysis.Text[0].Text)
	require.Len(t, analysis.Faces, 1)
	require.Len(t, analysis.Celebrities, 1)

	// Cached call should not invoke Rekognition again.
	before := transport.callCount("DetectModerationLabels") +
		transport.callCount("DetectLabels") +
		transport.callCount("DetectText") +
		transport.callCount("DetectFaces") +
		transport.callCount("RecognizeCelebrities")
	analysis2, err := ia.AnalyzeImage(ctx, "https://example.com/image.png", ContentMetadata{})
	require.NoError(t, err)
	require.NotNil(t, analysis2)
	after := transport.callCount("DetectModerationLabels") +
		transport.callCount("DetectLabels") +
		transport.callCount("DetectText") +
		transport.callCount("DetectFaces") +
		transport.callCount("RecognizeCelebrities")
	assert.Equal(t, before, after)
}

func TestImageAnalyzer_AnalyzeImage_AllFailuresReturnsError(t *testing.T) {
	ctx := context.Background()

	transport := newRekognitionStubTransport()
	transport.err["DetectModerationLabels"] = errors.New("boom")
	transport.err["DetectLabels"] = errors.New("boom")
	transport.err["DetectText"] = errors.New("boom")
	transport.err["DetectFaces"] = errors.New("boom")
	transport.err["RecognizeCelebrities"] = errors.New("boom")

	cfg := DefaultModerationConfig()
	cfg.EnableImageAnalysis = true

	client := rekognition.NewFromConfig(awsConfigForStub(transport))
	ia := NewImageAnalyzer(client, zap.NewNop(), cfg, nil)
	ia.fetchImageBytes = func(context.Context, string) ([]byte, error) { return []byte{1}, nil }

	_, err := ia.AnalyzeImage(ctx, "https://example.com/image.png", ContentMetadata{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all analyses failed")
}

func TestImageAnalyzer_AnalyzeImageContent_FlagsCombinations(t *testing.T) {
	ctx := context.Background()

	transport := newRekognitionStubTransport()
	transport.responses["DetectModerationLabels"] = map[string]any{
		"ModerationLabels": []any{
			map[string]any{"Name": "Explicit Nudity", "Confidence": 99.0},
			map[string]any{"Name": labelViolence, "Confidence": 90.0},
		},
	}
	transport.responses["DetectText"] = map[string]any{
		"TextDetections": []any{
			map[string]any{"DetectedText": "5551234567", "Confidence": 99.0, "Type": "LINE"},
		},
	}
	transport.responses["DetectFaces"] = map[string]any{
		"FaceDetails": []any{
			map[string]any{"Confidence": 99.0},
		},
	}
	transport.responses["DetectLabels"] = map[string]any{"Labels": []any{}}
	transport.responses["RecognizeCelebrities"] = map[string]any{"CelebrityFaces": []any{}}

	cfg := DefaultModerationConfig()
	cfg.EnableImageAnalysis = true

	client := rekognition.NewFromConfig(awsConfigForStub(transport))
	ia := NewImageAnalyzer(client, zap.NewNop(), cfg, nil)
	ia.fetchImageBytes = func(context.Context, string) ([]byte, error) { return []byte{1, 2, 3}, nil }

	combined, err := ia.AnalyzeImageContent(ctx, "https://example.com/image.png", "kill that kid my address is 123 street you are ugly")
	require.NoError(t, err)
	require.NotNil(t, combined)
	assert.Contains(t, combined.Flags, "VIOLENT_THREAT")
	assert.Contains(t, combined.Flags, "CHILD_SAFETY_CONCERN")
	assert.Contains(t, combined.Flags, "POSSIBLE_DOXXING")
	assert.Contains(t, combined.Flags, "TARGETED_HARASSMENT")
	assert.NotEmpty(t, combined.RiskLevel)
}

func TestImageAnalyzer_Helpers(t *testing.T) {
	assert.True(t, isS3URL("s3://bucket/key"))
	assert.True(t, isS3URL("https://bucket.s3.us-east-1.amazonaws.com/key"))
	assert.Equal(t, "path/to/key.jpg", extractS3Key("https://example.com/path/to/key.jpg?X-Amz-Signature=abc"))

	assert.True(t, containsThreatLanguage("I will KILL you"))
	assert.True(t, isTargetedAtMinors("underage"))
	assert.True(t, containsHarassmentLanguage("you are ugly"))
	assert.True(t, looksLikeAddress("123 street"))
	assert.True(t, looksLikePhoneNumber("5551234567"))
}
