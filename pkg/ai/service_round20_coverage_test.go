package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	comprehendtypes "github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rekognitiontypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type round20Comprehend struct {
	detectDominantLanguageOut *comprehend.DetectDominantLanguageOutput
	detectDominantLanguageErr error

	detectSentimentOut *comprehend.DetectSentimentOutput
	detectSentimentErr error

	classifyDocumentOut *comprehend.ClassifyDocumentOutput
	classifyDocumentErr error

	detectPiiEntitiesOut *comprehend.DetectPiiEntitiesOutput
	detectPiiEntitiesErr error

	detectEntitiesOut *comprehend.DetectEntitiesOutput
	detectEntitiesErr error

	detectKeyPhrasesOut *comprehend.DetectKeyPhrasesOutput
	detectKeyPhrasesErr error
}

func (c *round20Comprehend) DetectDominantLanguage(ctx context.Context, params *comprehend.DetectDominantLanguageInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectDominantLanguageOutput, error) {
	_ = ctx
	_ = params
	_ = optFns
	if c.detectDominantLanguageErr != nil {
		return nil, c.detectDominantLanguageErr
	}
	if c.detectDominantLanguageOut == nil {
		return &comprehend.DetectDominantLanguageOutput{}, nil
	}
	return c.detectDominantLanguageOut, nil
}

func (c *round20Comprehend) DetectSentiment(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
	_ = ctx
	_ = params
	_ = optFns
	if c.detectSentimentErr != nil {
		return nil, c.detectSentimentErr
	}
	if c.detectSentimentOut == nil {
		return nil, errors.New("no DetectSentiment stub configured")
	}
	return c.detectSentimentOut, nil
}

func (c *round20Comprehend) ClassifyDocument(ctx context.Context, params *comprehend.ClassifyDocumentInput, optFns ...func(*comprehend.Options)) (*comprehend.ClassifyDocumentOutput, error) {
	_ = ctx
	_ = params
	_ = optFns
	if c.classifyDocumentErr != nil {
		return nil, c.classifyDocumentErr
	}
	if c.classifyDocumentOut == nil {
		return &comprehend.ClassifyDocumentOutput{}, nil
	}
	return c.classifyDocumentOut, nil
}

func (c *round20Comprehend) DetectPiiEntities(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
	_ = ctx
	_ = params
	_ = optFns
	if c.detectPiiEntitiesErr != nil {
		return nil, c.detectPiiEntitiesErr
	}
	if c.detectPiiEntitiesOut == nil {
		return &comprehend.DetectPiiEntitiesOutput{}, nil
	}
	return c.detectPiiEntitiesOut, nil
}

func (c *round20Comprehend) DetectEntities(ctx context.Context, params *comprehend.DetectEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectEntitiesOutput, error) {
	_ = ctx
	_ = params
	_ = optFns
	if c.detectEntitiesErr != nil {
		return nil, c.detectEntitiesErr
	}
	if c.detectEntitiesOut == nil {
		return &comprehend.DetectEntitiesOutput{}, nil
	}
	return c.detectEntitiesOut, nil
}

func (c *round20Comprehend) DetectKeyPhrases(ctx context.Context, params *comprehend.DetectKeyPhrasesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectKeyPhrasesOutput, error) {
	_ = ctx
	_ = params
	_ = optFns
	if c.detectKeyPhrasesErr != nil {
		return nil, c.detectKeyPhrasesErr
	}
	if c.detectKeyPhrasesOut == nil {
		return &comprehend.DetectKeyPhrasesOutput{}, nil
	}
	return c.detectKeyPhrasesOut, nil
}

type round20BedrockRuntime struct {
	errByModelID map[string]error
	outByModelID map[string]*bedrockruntime.InvokeModelOutput
}

func (b *round20BedrockRuntime) InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	_ = ctx
	_ = optFns
	modelID := aws.ToString(params.ModelId)
	if b.errByModelID != nil {
		if err, ok := b.errByModelID[modelID]; ok {
			return nil, err
		}
	}
	if b.outByModelID != nil {
		if out, ok := b.outByModelID[modelID]; ok {
			return out, nil
		}
	}
	return &bedrockruntime.InvokeModelOutput{Body: []byte(`{}`)}, nil
}

type round20Rekognition struct {
	detectModerationLabelsOut *rekognition.DetectModerationLabelsOutput
	detectModerationLabelsErr error

	detectTextOut *rekognition.DetectTextOutput
	detectTextErr error

	recognizeCelebritiesOut *rekognition.RecognizeCelebritiesOutput
	recognizeCelebritiesErr error
}

func (r *round20Rekognition) DetectModerationLabels(ctx context.Context, params *rekognition.DetectModerationLabelsInput, optFns ...func(*rekognition.Options)) (*rekognition.DetectModerationLabelsOutput, error) {
	_ = ctx
	_ = params
	_ = optFns
	if r.detectModerationLabelsErr != nil {
		return nil, r.detectModerationLabelsErr
	}
	if r.detectModerationLabelsOut == nil {
		return &rekognition.DetectModerationLabelsOutput{}, nil
	}
	return r.detectModerationLabelsOut, nil
}

func (r *round20Rekognition) DetectText(ctx context.Context, params *rekognition.DetectTextInput, optFns ...func(*rekognition.Options)) (*rekognition.DetectTextOutput, error) {
	_ = ctx
	_ = params
	_ = optFns
	if r.detectTextErr != nil {
		return nil, r.detectTextErr
	}
	if r.detectTextOut == nil {
		return &rekognition.DetectTextOutput{}, nil
	}
	return r.detectTextOut, nil
}

func (r *round20Rekognition) RecognizeCelebrities(ctx context.Context, params *rekognition.RecognizeCelebritiesInput, optFns ...func(*rekognition.Options)) (*rekognition.RecognizeCelebritiesOutput, error) {
	_ = ctx
	_ = params
	_ = optFns
	if r.recognizeCelebritiesErr != nil {
		return nil, r.recognizeCelebritiesErr
	}
	if r.recognizeCelebritiesOut == nil {
		return &rekognition.RecognizeCelebritiesOutput{}, nil
	}
	return r.recognizeCelebritiesOut, nil
}

type round20S3 struct {
	putObjectErr error
	putObjectIn  *s3.PutObjectInput
}

func (s *round20S3) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	_ = ctx
	_ = optFns
	s.putObjectIn = params
	if s.putObjectErr != nil {
		return nil, s.putObjectErr
	}
	_, _ = io.ReadAll(params.Body)
	return &s3.PutObjectOutput{}, nil
}

type round20SQS struct {
	sendMessageErr error
	sendMessageIn  *sqs.SendMessageInput
	calls          int
}

func (c *round20SQS) SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	_ = ctx
	_ = optFns
	c.calls++
	c.sendMessageIn = params
	if c.sendMessageErr != nil {
		return nil, c.sendMessageErr
	}
	return &sqs.SendMessageOutput{MessageId: aws.String("mid")}, nil
}

type round20HTTPClient struct {
	respByURL map[string]*http.Response
	errByURL  map[string]error
}

func (c *round20HTTPClient) Do(req *http.Request) (*http.Response, error) {
	if c.errByURL != nil {
		if err, ok := c.errByURL[req.URL.String()]; ok {
			return nil, err
		}
	}
	if c.respByURL != nil {
		if resp, ok := c.respByURL[req.URL.String()]; ok {
			return resp, nil
		}
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("not found")),
	}, nil
}

type round20ErrCloseReadCloser struct {
	io.Reader
	closeErr error
}

func (r *round20ErrCloseReadCloser) Close() error { return r.closeErr }

func TestAIService_Round20_AnalyzeText_ComprehendPaths(t *testing.T) {
	t.Run("happy_path_with_toxicity_and_pii", func(t *testing.T) {
		text := "hello world"
		comprehendClient := &round20Comprehend{
			detectDominantLanguageOut: &comprehend.DetectDominantLanguageOutput{
				Languages: []comprehendtypes.DominantLanguage{
					{LanguageCode: aws.String("es"), Score: aws.Float32(0.9)},
					{LanguageCode: aws.String("en"), Score: aws.Float32(0.1)},
				},
			},
			detectSentimentOut: &comprehend.DetectSentimentOutput{
				Sentiment: comprehendtypes.SentimentTypePositive,
				SentimentScore: &comprehendtypes.SentimentScore{
					Positive: aws.Float32(0.9),
					Negative: aws.Float32(0.05),
					Neutral:  aws.Float32(0.03),
					Mixed:    aws.Float32(0.02),
				},
			},
			classifyDocumentOut: &comprehend.ClassifyDocumentOutput{
				Classes: []comprehendtypes.DocumentClass{
					{Name: aws.String("TOXIC"), Score: aws.Float32(0.75)},
					{Name: aws.String("OFFENSIVE"), Score: aws.Float32(0.85)},
					{Name: aws.String("NEUTRAL"), Score: aws.Float32(0.95)},
				},
			},
			detectPiiEntitiesOut: &comprehend.DetectPiiEntitiesOutput{
				Entities: []comprehendtypes.PiiEntity{
					{
						Type:        comprehendtypes.PiiEntityTypeSsn,
						Score:       aws.Float32(0.99),
						BeginOffset: aws.Int32(0),
						EndOffset:   aws.Int32(5),
					},
				},
			},
			detectEntitiesOut: &comprehend.DetectEntitiesOutput{
				Entities: []comprehendtypes.Entity{
					{Type: comprehendtypes.EntityTypePerson, Text: aws.String("Alice"), Score: aws.Float32(0.88)},
				},
			},
			detectKeyPhrasesOut: &comprehend.DetectKeyPhrasesOutput{
				KeyPhrases: []comprehendtypes.KeyPhrase{
					{Text: aws.String("hello"), Score: aws.Float32(0.9)},
					{Text: aws.String("skip"), Score: aws.Float32(0.7)},
				},
			},
		}

		svc := &AIService{
			comprehend: comprehendClient,
			logger:     zap.NewNop(),
			config: &AIConfig{
				EnablePIIDetection: true,
				ToxicityModelARN:   "arn:aws:comprehend:toxicity",
			},
		}

		analysis, err := svc.analyzeText(context.Background(), text)
		require.NoError(t, err)
		require.Equal(t, "es", analysis.DominantLanguage)
		require.NotEmpty(t, analysis.LanguageScores)
		require.Equal(t, SentimentPositive, analysis.Sentiment)
		require.NotEmpty(t, analysis.SentimentScores)
		require.True(t, analysis.ContainsPII)
		require.Len(t, analysis.PIIEntities, 1)
		require.Equal(t, "hello", analysis.PIIEntities[0].Text)
		require.Len(t, analysis.Entities, 1)
		require.Equal(t, "Alice", analysis.Entities[0].Text)
		require.InDelta(t, 0.85, analysis.ToxicityScore, 0.0001)
		require.NotEmpty(t, analysis.ToxicityLabels)
		require.Equal(t, []string{"hello"}, analysis.KeyPhrases)
	})

	t.Run("language_detection_failure_and_sentiment_toxicity_fallback", func(t *testing.T) {
		comprehendClient := &round20Comprehend{
			detectDominantLanguageErr: errors.New("language fail"),
			detectSentimentOut: &comprehend.DetectSentimentOutput{
				Sentiment: comprehendtypes.SentimentTypeNegative,
				SentimentScore: &comprehendtypes.SentimentScore{
					Positive: aws.Float32(0.1),
					Negative: aws.Float32(0.8),
					Neutral:  aws.Float32(0.05),
					Mixed:    aws.Float32(0.05),
				},
			},
			detectEntitiesErr:   errors.New("entities fail"),
			detectKeyPhrasesErr: errors.New("phrases fail"),
		}

		svc := &AIService{
			comprehend: comprehendClient,
			logger:     zap.NewNop(),
			config: &AIConfig{
				EnablePIIDetection: false,
				ToxicityModelARN:   "",
			},
		}

		analysis, err := svc.analyzeText(context.Background(), "some text")
		require.NoError(t, err)
		require.Equal(t, "en", analysis.DominantLanguage)
		require.Equal(t, SentimentNegative, analysis.Sentiment)
		require.InDelta(t, 0.4, analysis.ToxicityScore, 0.0001)
	})
}

func TestAIService_Round20_ImageAnalysis_Paths(t *testing.T) {
	t.Run("analyzeImage_returns_empty_when_rekognition_nil", func(t *testing.T) {
		svc := &AIService{config: &AIConfig{S3Bucket: "bucket"}}
		analysis, err := svc.analyzeImage(context.Background(), "ignored", "key")
		require.NoError(t, err)
		require.NotNil(t, analysis)
	})

	t.Run("analyzeImages_merges_labels_and_text_toxicity", func(t *testing.T) {
		textClient := &round20Comprehend{
			detectDominantLanguageErr: errors.New("skip language"),
			detectSentimentOut: &comprehend.DetectSentimentOutput{
				Sentiment: comprehendtypes.SentimentTypeNegative,
				SentimentScore: &comprehendtypes.SentimentScore{
					Positive: aws.Float32(0.1),
					Negative: aws.Float32(0.6),
					Neutral:  aws.Float32(0.2),
					Mixed:    aws.Float32(0.1),
				},
			},
		}

		parent := "Adult"
		line := "click here"
		word := "word"
		rekognitionClient := &round20Rekognition{
			detectModerationLabelsOut: &rekognition.DetectModerationLabelsOutput{
				ModerationLabels: []rekognitiontypes.ModerationLabel{
					{Name: aws.String("Explicit Nudity"), Confidence: aws.Float32(90), ParentName: aws.String(parent)},
					{Name: aws.String("Weapon"), Confidence: aws.Float32(80)},
					{Name: aws.String("Hate Symbols"), Confidence: aws.Float32(70)},
				},
			},
			detectTextOut: &rekognition.DetectTextOutput{
				TextDetections: []rekognitiontypes.TextDetection{
					{Type: rekognitiontypes.TextTypesLine, DetectedText: aws.String(line)},
					{Type: rekognitiontypes.TextTypesWord, DetectedText: aws.String(word)},
				},
			},
			recognizeCelebritiesOut: &rekognition.RecognizeCelebritiesOutput{
				CelebrityFaces: []rekognitiontypes.Celebrity{
					{
						Name: aws.String("Famous"),
						Urls: []string{"https://example.com"},
						Face: &rekognitiontypes.ComparedFace{Confidence: aws.Float32(99)},
					},
				},
			},
		}

		svc := &AIService{
			comprehend:  textClient,
			rekognition: rekognitionClient,
			logger:      zap.NewNop(),
			config: &AIConfig{
				S3Bucket: "bucket",
			},
		}

		analysis, err := svc.analyzeImages(context.Background(), []string{
			"http://localhost:1234/",                              // should be blocked and skipped
			"https://d123abc.cloudfront.net/media/images/a.jpg",   // processed
			"https://bucket.s3.us-east-1.amazonaws.com/b.jpg",     // processed
			"https://d123abc.cloudfront.net/media/images/c.jpg",   // processed
			"https://bucket.s3.us-east-1.amazonaws.com/dir/d.jpg", // processed
		})
		require.NoError(t, err)
		require.NotNil(t, analysis)
		require.NotEmpty(t, analysis.ModerationLabels)
		require.True(t, analysis.IsNSFW)
		require.Greater(t, analysis.NSFWConfidence, 0.0)
		require.True(t, analysis.WeaponsDetected)
		require.Greater(t, analysis.ViolenceScore, 0.0)
		require.Contains(t, analysis.DetectedText, line)
		require.Greater(t, analysis.TextToxicity, 0.0)
		require.NotEmpty(t, analysis.CelebrityFaces)
	})

	t.Run("rekognition_errors_short_circuit_detectors", func(t *testing.T) {
		rekognitionClient := &round20Rekognition{
			detectModerationLabelsErr: errors.New("moderr"),
			detectTextErr:             errors.New("texterr"),
			recognizeCelebritiesErr:   errors.New("celeberr"),
		}
		svc := &AIService{rekognition: rekognitionClient, config: &AIConfig{S3Bucket: "bucket"}}

		analysis, err := svc.analyzeImage(context.Background(), "ignored", "key")
		require.NoError(t, err)
		require.NotNil(t, analysis)
		require.Empty(t, analysis.ModerationLabels)
		require.Empty(t, analysis.DetectedText)
		require.Empty(t, analysis.CelebrityFaces)
	})
}

func TestAIService_Round20_DetectAIContent_GenerateEmbedding_AndRequests(t *testing.T) {
	t.Run("detectAIContent_bedrock_error_and_valid_completion_json", func(t *testing.T) {
		aiModelID := "anthropic.claude-v2"
		embedModelID := "amazon.titan-embed-text-v1"
		bedrockClient := &round20BedrockRuntime{
			errByModelID: map[string]error{
				aiModelID: errors.New("bedrock down"),
			},
			outByModelID: map[string]*bedrockruntime.InvokeModelOutput{
				embedModelID: {Body: []byte(`{"embedding":[1.1,2.2,3.3]}`)},
			},
		}

		svc := &AIService{
			bedrock: bedrockClient,
			logger:  zap.NewNop(),
			config: &AIConfig{
				BedrockModelID: aiModelID,
			},
		}

		detection, err := svc.detectAIContent(context.Background(), &Content{Text: "As an AI, I cannot do that. My training data suggests otherwise."})
		require.NoError(t, err)
		require.NotNil(t, detection)
		require.NotEmpty(t, detection.SuspiciousPatterns)

		embedding, err := svc.GenerateEmbedding(context.Background(), "hello")
		require.NoError(t, err)
		require.Equal(t, []float32{1.1, 2.2, 3.3}, embedding)

		_, err = svc.GetAnalysis(context.Background(), "id")
		require.ErrorIs(t, err, ErrGetAnalysisDeprecated)
		_, err = svc.GetAnalysisStats(context.Background(), "id")
		require.ErrorIs(t, err, ErrGetAnalysisStatsDeprecated)
	})

	t.Run("detectAIContent_parses_completion_json_when_available", func(t *testing.T) {
		aiModelID := "anthropic.claude-v2"
		completionJSON := `{"ai_generated_probability":0.91,"generation_model":"test","pattern_consistency":0.2,"style_deviation":0.3,"semantic_coherence":0.4,"suspicious_patterns":["x"]}`
		body, err := json.Marshal(map[string]any{"completion": completionJSON})
		require.NoError(t, err)

		bedrockClient := &round20BedrockRuntime{
			outByModelID: map[string]*bedrockruntime.InvokeModelOutput{
				aiModelID: {Body: body},
			},
		}

		svc := &AIService{
			bedrock: bedrockClient,
			logger:  zap.NewNop(),
			config: &AIConfig{
				BedrockModelID: aiModelID,
			},
		}

		detection, err := svc.detectAIContent(context.Background(), &Content{Text: "One sentence. Two sentence."})
		require.NoError(t, err)
		require.NotNil(t, detection)
		require.InDelta(t, 0.91, detection.AIGeneratedProbability, 0.0001)
		require.Equal(t, "test", detection.GenerationModel)
		require.Greater(t, detection.TopicConsistency, 0.0)
	})

	t.Run("GenerateEmbedding_unmarshal_and_invalid_embedding_errors", func(t *testing.T) {
		embedModelID := "amazon.titan-embed-text-v1"
		bedrockClient := &round20BedrockRuntime{
			outByModelID: map[string]*bedrockruntime.InvokeModelOutput{
				embedModelID: {Body: []byte("not-json")},
			},
		}

		svc := &AIService{bedrock: bedrockClient, config: &AIConfig{}}
		_, err := svc.GenerateEmbedding(context.Background(), "hello")
		require.Error(t, err)

		bedrockClient.outByModelID[embedModelID] = &bedrockruntime.InvokeModelOutput{Body: []byte(`{"embedding":"nope"}`)}
		_, err = svc.GenerateEmbedding(context.Background(), "hello")
		require.ErrorIs(t, err, ErrInvalidEmbeddingResponse)
	})

	t.Run("QueueAnalysisRequest_success_skip_and_failure_paths", func(t *testing.T) {
		sqsClient := &round20SQS{}

		svc := &AIService{
			sqsClient: sqsClient,
			logger:    zap.NewNop(),
			config:    &AIConfig{AIQueueURL: ""},
		}
		reqID, err := svc.QueueAnalysisRequest(context.Background(), "obj", "note", false)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(reqID, "ai-req-"))
		require.Equal(t, 0, sqsClient.calls)

		svc.config.AIQueueURL = "https://sqs.example.com/queue"
		reqID, err = svc.QueueAnalysisRequest(context.Background(), "obj2", "note", true)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(reqID, "ai-req-"))
		require.Equal(t, 1, sqsClient.calls)
		require.NotNil(t, sqsClient.sendMessageIn)
		require.Equal(t, svc.config.AIQueueURL, aws.ToString(sqsClient.sendMessageIn.QueueUrl))
		require.NotEmpty(t, aws.ToString(sqsClient.sendMessageIn.MessageBody))
		require.Contains(t, sqsClient.sendMessageIn.MessageAttributes, "RequestID")

		sqsClient.sendMessageErr = errors.New("sqs down")
		_, err = svc.QueueAnalysisRequest(context.Background(), "obj3", "note", false)
		require.Error(t, err)
	})
}

func TestAIService_Round20_UploadImageToS3_Paths(t *testing.T) {
	t.Run("existing_key_short_circuit", func(t *testing.T) {
		svc := &AIService{config: &AIConfig{S3Bucket: "bucket"}}
		key, err := svc.uploadImageToS3(context.Background(), "https://bucket.s3.us-east-1.amazonaws.com/media/images/photo.jpg")
		require.NoError(t, err)
		require.Equal(t, "media/images/photo.jpg", key)
	})

	t.Run("invalid_url_parse_scheme_local_network_and_download_errors", func(t *testing.T) {
		svc := &AIService{
			config:     &AIConfig{S3Bucket: "bucket"},
			httpClient: &round20HTTPClient{errByURL: map[string]error{"https://example.com/": errors.New("dial")}},
			s3Client:   &round20S3{},
			logger:     zap.NewNop(),
		}

		_, err := svc.uploadImageToS3(context.Background(), "https://example.com/%zz/")
		require.Error(t, err)

		_, err = svc.uploadImageToS3(context.Background(), "ftp://example.com/")
		require.ErrorIs(t, err, ErrInvalidURLScheme)

		_, err = svc.uploadImageToS3(context.Background(), "http://localhost:8080/")
		require.ErrorIs(t, err, ErrLocalNetworkAccess)

		_, err = svc.uploadImageToS3(context.Background(), "https://example.com/")
		require.Error(t, err)
	})

	t.Run("http_status_not_ok_and_putobject_error_and_success", func(t *testing.T) {
		httpClient := &round20HTTPClient{
			respByURL: map[string]*http.Response{
				"https://example.com/": {
					StatusCode: http.StatusInternalServerError,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("nope")),
				},
				"https://example.com/success/": {
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"image/png"}},
					Body: &round20ErrCloseReadCloser{
						Reader:   bytes.NewReader([]byte("img")),
						closeErr: errors.New("close"),
					},
				},
			},
		}
		s3Client := &round20S3{putObjectErr: errors.New("s3 down")}
		svc := &AIService{
			config:     &AIConfig{S3Bucket: "bucket"},
			httpClient: httpClient,
			s3Client:   s3Client,
			logger:     zap.NewNop(),
		}

		_, err := svc.uploadImageToS3(context.Background(), "https://example.com/")
		require.ErrorIs(t, err, ErrImageDownloadHTTP)

		_, err = svc.uploadImageToS3(context.Background(), "https://example.com/success/")
		require.Error(t, err)

		s3Client.putObjectErr = nil
		key, err := svc.uploadImageToS3(context.Background(), "https://example.com/success/")
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(key, "ai-analysis/ai-image-"))
		require.True(t, strings.HasSuffix(key, ".png"))
		require.NotNil(t, s3Client.putObjectIn)
		require.Equal(t, "bucket", aws.ToString(s3Client.putObjectIn.Bucket))
		require.Equal(t, key, aws.ToString(s3Client.putObjectIn.Key))
	})
}

func TestAIService_Round20_AnalyzeContent_ExerciseTopLevel(t *testing.T) {
	aiModelID := "anthropic.claude-v2"
	body, err := json.Marshal(map[string]any{"completion": `{"ai_generated_probability":0.1,"pattern_consistency":0.2,"style_deviation":0.3,"semantic_coherence":0.4,"suspicious_patterns":[]}`})
	require.NoError(t, err)

	bedrockClient := &round20BedrockRuntime{
		outByModelID: map[string]*bedrockruntime.InvokeModelOutput{
			aiModelID: {Body: body},
		},
	}

	comprehendClient := &round20Comprehend{
		detectDominantLanguageErr: errors.New("skip"),
		detectSentimentOut: &comprehend.DetectSentimentOutput{
			Sentiment: comprehendtypes.SentimentTypeNeutral,
			SentimentScore: &comprehendtypes.SentimentScore{
				Positive: aws.Float32(0.2),
				Negative: aws.Float32(0.2),
				Neutral:  aws.Float32(0.5),
				Mixed:    aws.Float32(0.1),
			},
		},
	}

	sqsClient := &round20SQS{}

	svc := &AIService{
		comprehend: comprehendClient,
		rekognition: &round20Rekognition{
			detectModerationLabelsOut: &rekognition.DetectModerationLabelsOutput{},
			detectTextOut:             &rekognition.DetectTextOutput{},
			recognizeCelebritiesOut:   &rekognition.RecognizeCelebritiesOutput{},
		},
		bedrock:   bedrockClient,
		sqsClient: sqsClient,
		logger:    zap.NewNop(),
		config: &AIConfig{
			EnablePIIDetection:  false,
			EnableAIDetection:   true,
			EnableImageAnalysis: true,
			BedrockModelID:      aiModelID,
			NSFWThreshold:       0.7,
			ToxicityThreshold:   0.8,
			SpamThreshold:       0.6,
			AIContentThreshold:  0.6,
			S3Bucket:            "bucket",
			AIQueueURL:          "https://sqs.example.com/queue",
		},
	}

	analysis, err := svc.AnalyzeContent(context.Background(), &Content{
		ID:        "obj1",
		Type:      "note",
		Text:      "Click here http://a.example http://b.example buy buy buy",
		MediaURLs: []string{"https://bucket.s3.us-east-1.amazonaws.com/media/images/photo.jpg"},
	})
	require.NoError(t, err)
	require.NotNil(t, analysis)
	require.NotNil(t, analysis.SpamAnalysis)
	require.NotNil(t, analysis.TextAnalysis)
	require.NotNil(t, analysis.ImageAnalysis)
	require.NotNil(t, analysis.AIDetection)
	require.NotEmpty(t, analysis.ID)
	require.Equal(t, "obj1", analysis.ObjectID)
	require.Equal(t, "note", analysis.ObjectType)
	require.NotEmpty(t, analysis.ModerationAction)
	require.GreaterOrEqual(t, analysis.Confidence, 0.0)
	require.LessOrEqual(t, analysis.Confidence, 1.0)
	require.Contains(t, analysis.SpamAnalysis.SpamIndicators, SpamIndicator{Type: "spam_phrase", Description: "Contains spam phrase: click here", Severity: 0.7})

	// Ensure SQS marshaling path runs.
	reqID, err := svc.QueueAnalysisRequest(context.Background(), "obj1", "note", false)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(reqID, "ai-req-"))
	require.Equal(t, 1, sqsClient.calls)
	require.NotNil(t, sqsClient.sendMessageIn)
	require.Contains(t, sqsClient.sendMessageIn.MessageAttributes, "ObjectType")
	require.Equal(t, aws.String("String"), sqsClient.sendMessageIn.MessageAttributes["ObjectType"].DataType)
	require.NotNil(t, sqsClient.sendMessageIn.MessageAttributes["ObjectType"].StringValue)
	require.Equal(t, "note", aws.ToString(sqsClient.sendMessageIn.MessageAttributes["ObjectType"].StringValue))
}
