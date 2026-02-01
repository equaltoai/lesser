package moderation

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	comprehendTypes "github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rekognitionTypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeComprehendClient struct {
	dominantLanguageOut *comprehend.DetectDominantLanguageOutput
	dominantLanguageErr error

	sentimentOut *comprehend.DetectSentimentOutput
	sentimentErr error

	entitiesOut *comprehend.DetectEntitiesOutput
	entitiesErr error

	keyPhrasesOut *comprehend.DetectKeyPhrasesOutput
	keyPhrasesErr error

	piiOut *comprehend.DetectPiiEntitiesOutput
	piiErr error
}

func (f *fakeComprehendClient) DetectDominantLanguage(_ context.Context, _ *comprehend.DetectDominantLanguageInput, _ ...func(*comprehend.Options)) (*comprehend.DetectDominantLanguageOutput, error) {
	if f.dominantLanguageErr != nil {
		return nil, f.dominantLanguageErr
	}
	if f.dominantLanguageOut != nil {
		return f.dominantLanguageOut, nil
	}
	return &comprehend.DetectDominantLanguageOutput{}, nil
}

func (f *fakeComprehendClient) DetectSentiment(_ context.Context, _ *comprehend.DetectSentimentInput, _ ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
	if f.sentimentErr != nil {
		return nil, f.sentimentErr
	}
	if f.sentimentOut != nil {
		return f.sentimentOut, nil
	}
	return &comprehend.DetectSentimentOutput{}, nil
}

func (f *fakeComprehendClient) DetectEntities(_ context.Context, _ *comprehend.DetectEntitiesInput, _ ...func(*comprehend.Options)) (*comprehend.DetectEntitiesOutput, error) {
	if f.entitiesErr != nil {
		return nil, f.entitiesErr
	}
	if f.entitiesOut != nil {
		return f.entitiesOut, nil
	}
	return &comprehend.DetectEntitiesOutput{}, nil
}

func (f *fakeComprehendClient) DetectKeyPhrases(_ context.Context, _ *comprehend.DetectKeyPhrasesInput, _ ...func(*comprehend.Options)) (*comprehend.DetectKeyPhrasesOutput, error) {
	if f.keyPhrasesErr != nil {
		return nil, f.keyPhrasesErr
	}
	if f.keyPhrasesOut != nil {
		return f.keyPhrasesOut, nil
	}
	return &comprehend.DetectKeyPhrasesOutput{}, nil
}

func (f *fakeComprehendClient) DetectPiiEntities(_ context.Context, _ *comprehend.DetectPiiEntitiesInput, _ ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
	if f.piiErr != nil {
		return nil, f.piiErr
	}
	if f.piiOut != nil {
		return f.piiOut, nil
	}
	return &comprehend.DetectPiiEntitiesOutput{}, nil
}

type fakeRekognitionClient struct {
	moderationLabelsOut *rekognition.DetectModerationLabelsOutput
	moderationLabelsErr error

	labelsOut *rekognition.DetectLabelsOutput
	labelsErr error

	textOut *rekognition.DetectTextOutput
	textErr error

	facesOut *rekognition.DetectFacesOutput
	facesErr error
}

func (f *fakeRekognitionClient) DetectModerationLabels(_ context.Context, _ *rekognition.DetectModerationLabelsInput, _ ...func(*rekognition.Options)) (*rekognition.DetectModerationLabelsOutput, error) {
	if f.moderationLabelsErr != nil {
		return nil, f.moderationLabelsErr
	}
	if f.moderationLabelsOut != nil {
		return f.moderationLabelsOut, nil
	}
	return &rekognition.DetectModerationLabelsOutput{}, nil
}

func (f *fakeRekognitionClient) DetectLabels(_ context.Context, _ *rekognition.DetectLabelsInput, _ ...func(*rekognition.Options)) (*rekognition.DetectLabelsOutput, error) {
	if f.labelsErr != nil {
		return nil, f.labelsErr
	}
	if f.labelsOut != nil {
		return f.labelsOut, nil
	}
	return &rekognition.DetectLabelsOutput{}, nil
}

func (f *fakeRekognitionClient) DetectText(_ context.Context, _ *rekognition.DetectTextInput, _ ...func(*rekognition.Options)) (*rekognition.DetectTextOutput, error) {
	if f.textErr != nil {
		return nil, f.textErr
	}
	if f.textOut != nil {
		return f.textOut, nil
	}
	return &rekognition.DetectTextOutput{}, nil
}

func (f *fakeRekognitionClient) DetectFaces(_ context.Context, _ *rekognition.DetectFacesInput, _ ...func(*rekognition.Options)) (*rekognition.DetectFacesOutput, error) {
	if f.facesErr != nil {
		return nil, f.facesErr
	}
	if f.facesOut != nil {
		return f.facesOut, nil
	}
	return &rekognition.DetectFacesOutput{}, nil
}

func TestAIAnalyzer_AnalyzeText_Success(t *testing.T) {
	pos, neg, neu, mix := float32(0.1), float32(0.9), float32(0.0), float32(0.0)
	sentimentScore := &comprehendTypes.SentimentScore{
		Positive: &pos,
		Negative: &neg,
		Neutral:  &neu,
		Mixed:    &mix,
	}

	begin, end := int32(0), int32(5)
	entityScore := float32(0.8)

	ai := &AIAnalyzer{
		comprehend: &fakeComprehendClient{
			dominantLanguageOut: &comprehend.DetectDominantLanguageOutput{
				Languages: []comprehendTypes.DominantLanguage{{LanguageCode: aws.String("es")}},
			},
			sentimentOut: &comprehend.DetectSentimentOutput{
				Sentiment:      comprehendTypes.SentimentTypeNegative,
				SentimentScore: sentimentScore,
			},
			entitiesOut: &comprehend.DetectEntitiesOutput{
				Entities: []comprehendTypes.Entity{{
					Text:        aws.String("Alice"),
					Type:        comprehendTypes.EntityTypePerson,
					Score:       &entityScore,
					BeginOffset: &begin,
					EndOffset:   &end,
				}},
			},
			keyPhrasesOut: &comprehend.DetectKeyPhrasesOutput{
				KeyPhrases: []comprehendTypes.KeyPhrase{{
					Text:        aws.String("some phrase"),
					Score:       &entityScore,
					BeginOffset: &begin,
					EndOffset:   &end,
				}},
			},
			piiOut: &comprehend.DetectPiiEntitiesOutput{
				Entities: []comprehendTypes.PiiEntity{{
					Type:        comprehendTypes.PiiEntityTypeSsn,
					Score:       &entityScore,
					BeginOffset: &begin,
					EndOffset:   &end,
				}},
			},
		},
	}

	analysis, err := ai.AnalyzeText(context.Background(), &TextContent{ID: "t1", Text: "hello"})
	require.NoError(t, err)
	require.NotNil(t, analysis)

	assert.Equal(t, "es", analysis.Language)
	assert.NotNil(t, analysis.Sentiment)
	assert.NotEmpty(t, analysis.Entities)
	assert.NotEmpty(t, analysis.KeyPhrases)
	assert.NotEmpty(t, analysis.PIIEntities)
	assert.NotEmpty(t, analysis.RiskLevel)
}

func TestAIAnalyzer_AnalyzeText_IgnoresServiceErrors(t *testing.T) {
	ai := &AIAnalyzer{
		comprehend: &fakeComprehendClient{
			sentimentErr:  errors.New("sentiment boom"),
			entitiesErr:   errors.New("entities boom"),
			keyPhrasesErr: errors.New("phrases boom"),
			piiErr:        errors.New("pii boom"),
		},
	}

	analysis, err := ai.AnalyzeText(context.Background(), &TextContent{ID: "t1", Text: "hello"})
	require.NoError(t, err)
	require.NotNil(t, analysis)

	assert.Equal(t, "en", analysis.Language)
	assert.Nil(t, analysis.Sentiment)
	assert.Empty(t, analysis.Entities)
	assert.Empty(t, analysis.KeyPhrases)
	assert.Empty(t, analysis.PIIEntities)
	assert.Equal(t, "minimal", analysis.RiskLevel)
}

func TestAIAnalyzer_AnalyzeImage_SuccessWithTextAndFaces(t *testing.T) {
	pos, neg, neu, mix := float32(0.1), float32(0.9), float32(0.0), float32(0.0)
	sentimentScore := &comprehendTypes.SentimentScore{Positive: &pos, Negative: &neg, Neutral: &neu, Mixed: &mix}

	ai := &AIAnalyzer{
		comprehend: &fakeComprehendClient{
			sentimentOut: &comprehend.DetectSentimentOutput{
				Sentiment:      comprehendTypes.SentimentTypeNegative,
				SentimentScore: sentimentScore,
			},
		},
		rekognition: &fakeRekognitionClient{
			moderationLabelsOut: &rekognition.DetectModerationLabelsOutput{
				ModerationLabels: []rekognitionTypes.ModerationLabel{{
					Name:       aws.String("Violence"),
					Confidence: aws.Float32(90),
				}},
			},
			labelsOut: &rekognition.DetectLabelsOutput{
				Labels: []rekognitionTypes.Label{{
					Name:       aws.String("Person"),
					Confidence: aws.Float32(99),
				}},
			},
			textOut: &rekognition.DetectTextOutput{
				TextDetections: []rekognitionTypes.TextDetection{
					{Type: rekognitionTypes.TextTypesWord, DetectedText: aws.String("ignore")},
					{Type: rekognitionTypes.TextTypesLine, DetectedText: aws.String("some extracted text")},
				},
			},
			facesOut: &rekognition.DetectFacesOutput{
				FaceDetails: []rekognitionTypes.FaceDetail{{
					Confidence: aws.Float32(99),
					AgeRange:   &rekognitionTypes.AgeRange{Low: aws.Int32(20), High: aws.Int32(30)},
					Emotions: []rekognitionTypes.Emotion{
						{Type: rekognitionTypes.EmotionNameHappy, Confidence: aws.Float32(50)},
					},
				}},
			},
		},
	}

	analysis, err := ai.AnalyzeImage(context.Background(), &ImageContent{ID: "img1", URL: "https://example.com/img", ImageBytes: []byte("x")})
	require.NoError(t, err)
	require.NotNil(t, analysis)

	assert.NotEmpty(t, analysis.ModerationLabels)
	assert.NotEmpty(t, analysis.Labels)
	assert.Equal(t, []string{"some extracted text"}, analysis.DetectedText)
	assert.NotNil(t, analysis.TextAnalysis)
	assert.NotEmpty(t, analysis.Faces)
	assert.NotEmpty(t, analysis.Recommendations)
}

func TestAIAnalyzer_AnalyzeImage_TextDetectionErrorIsIgnored(t *testing.T) {
	ai := &AIAnalyzer{
		comprehend: &fakeComprehendClient{},
		rekognition: &fakeRekognitionClient{
			textErr: errors.New("text boom"),
		},
	}

	analysis, err := ai.AnalyzeImage(context.Background(), &ImageContent{ID: "img1", URL: "x", ImageBytes: []byte("x")})
	require.NoError(t, err)
	require.NotNil(t, analysis)
	assert.Nil(t, analysis.TextAnalysis)
}
