package translation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/translate"
	translatetypes "github.com/aws/aws-sdk-go-v2/service/translate/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeTranslate struct {
	translateCalls int
	listCalls      int

	lastTranslateInput *translate.TranslateTextInput

	translateFn func(ctx context.Context, params *translate.TranslateTextInput) (*translate.TranslateTextOutput, error)
	listFn      func(ctx context.Context, params *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error)
}

func (f *fakeTranslate) TranslateText(ctx context.Context, params *translate.TranslateTextInput, _ ...func(*translate.Options)) (*translate.TranslateTextOutput, error) {
	f.translateCalls++
	f.lastTranslateInput = params
	return f.translateFn(ctx, params)
}

func (f *fakeTranslate) ListLanguages(ctx context.Context, params *translate.ListLanguagesInput, _ ...func(*translate.Options)) (*translate.ListLanguagesOutput, error) {
	f.listCalls++
	return f.listFn(ctx, params)
}

type fakeDynamo struct {
	getCalls int
	putCalls int

	lastGet *dynamodb.GetItemInput
	lastPut *dynamodb.PutItemInput

	getFn func(ctx context.Context, params *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error)
	putFn func(ctx context.Context, params *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error)
}

func (f *fakeDynamo) GetItem(ctx context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.getCalls++
	f.lastGet = params
	return f.getFn(ctx, params)
}

func (f *fakeDynamo) PutItem(ctx context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.putCalls++
	f.lastPut = params
	return f.putFn(ctx, params)
}

func TestService_TranslateText_CacheHitSkipsTranslate(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)

	dynamo := &fakeDynamo{
		getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{
				Item: map[string]dynamotypes.AttributeValue{
					"TranslatedText":   &dynamotypes.AttributeValueMemberS{Value: "hola"},
					"DetectedLanguage": &dynamotypes.AttributeValueMemberS{Value: "es"},
					"CachedAt":         &dynamotypes.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
				},
			}, nil
		},
		putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	client := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			t.Fatalf("TranslateText should not be called on cache hit")
			return nil, nil
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client:       client,
		dynamoClient: dynamo,
		tableName:    "tbl",
		logger:       zap.NewNop(),
		cacheEnabled: true,
		cacheTTL:     time.Hour,
	}

	text, lang, err := svc.TranslateText(context.Background(), "hello", "auto", "es")
	require.NoError(t, err)
	require.Equal(t, "hola", text)
	require.Equal(t, "es", lang)
	require.Equal(t, 1, dynamo.getCalls)
	require.Equal(t, 0, client.translateCalls)
}

func TestService_TranslateText_CacheMissCallsTranslateAndCaches(t *testing.T) {
	t.Parallel()

	dynamo := &fakeDynamo{
		getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: nil}, nil
		},
		putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	client := &fakeTranslate{
		translateFn: func(_ context.Context, in *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			require.Nil(t, in.SourceLanguageCode) // "auto" should not set explicit source
			return &translate.TranslateTextOutput{
				TranslatedText:     aws.String("hola"),
				SourceLanguageCode: aws.String("es"),
			}, nil
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client:       client,
		dynamoClient: dynamo,
		tableName:    "tbl",
		logger:       zap.NewNop(),
		cacheEnabled: true,
		cacheTTL:     time.Hour,
	}

	text, lang, err := svc.TranslateText(context.Background(), "hello", "auto", "es")
	require.NoError(t, err)
	require.Equal(t, "hola", text)
	require.Equal(t, "es", lang)
	require.Equal(t, 1, client.translateCalls)
	require.Equal(t, 1, dynamo.putCalls)

	require.NotNil(t, dynamo.lastPut)
	require.Equal(t, "tbl", aws.ToString(dynamo.lastPut.TableName))
	require.Contains(t, dynamo.lastPut.Item, "PK")
	require.Contains(t, dynamo.lastPut.Item, "SK")
}

func TestService_TranslateText_SourceLanguageExplicit(t *testing.T) {
	t.Parallel()

	dynamo := &fakeDynamo{
		getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: nil}, nil
		},
		putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	client := &fakeTranslate{
		translateFn: func(_ context.Context, in *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			require.Equal(t, "fr", aws.ToString(in.SourceLanguageCode))
			return &translate.TranslateTextOutput{
				TranslatedText:     aws.String("hello"),
				SourceLanguageCode: aws.String("fr"),
			}, nil
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client:       client,
		dynamoClient: dynamo,
		tableName:    "tbl",
		logger:       zap.NewNop(),
		cacheEnabled: false,
		cacheTTL:     time.Hour,
	}

	_, _, err := svc.TranslateText(context.Background(), "bonjour", "fr", "en")
	require.NoError(t, err)
}

func TestService_TranslateText_CacheWriteFailureDoesNotFailRequest(t *testing.T) {
	t.Parallel()

	dynamo := &fakeDynamo{
		getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: nil}, nil
		},
		putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
			return nil, errors.New("dynamo down")
		},
	}

	client := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return &translate.TranslateTextOutput{
				TranslatedText:     aws.String("hola"),
				SourceLanguageCode: aws.String("es"),
			}, nil
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client:       client,
		dynamoClient: dynamo,
		tableName:    "tbl",
		logger:       zap.NewNop(),
		cacheEnabled: true,
		cacheTTL:     time.Hour,
	}

	text, lang, err := svc.TranslateText(context.Background(), "hello", "", "es")
	require.NoError(t, err)
	require.Equal(t, "hola", text)
	require.Equal(t, "es", lang)
	require.Equal(t, 1, dynamo.putCalls)
}

func TestService_GetSupportedLanguages_CacheHitAndMiss(t *testing.T) {
	t.Parallel()

	cachedLanguagesItem := map[string]dynamotypes.AttributeValue{
		"Languages": &dynamotypes.AttributeValueMemberL{Value: []dynamotypes.AttributeValue{
			&dynamotypes.AttributeValueMemberM{Value: map[string]dynamotypes.AttributeValue{
				"Code": &dynamotypes.AttributeValueMemberS{Value: "en"},
				"Name": &dynamotypes.AttributeValueMemberS{Value: "English"},
			}},
		}},
	}

	dynamo := &fakeDynamo{
		getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: cachedLanguagesItem}, nil
		},
		putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	client := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return &translate.TranslateTextOutput{}, nil
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			t.Fatalf("ListLanguages should not be called on cache hit")
			return nil, nil
		},
	}

	svc := &Service{
		client:       client,
		dynamoClient: dynamo,
		tableName:    "tbl",
		logger:       zap.NewNop(),
		cacheEnabled: true,
		cacheTTL:     time.Hour,
	}

	langs, err := svc.GetSupportedLanguages(context.Background())
	require.NoError(t, err)
	require.Len(t, langs, 1)
	require.Equal(t, "en", langs[0].Code)

	dynamo.getFn = func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
		return &dynamodb.GetItemOutput{Item: nil}, nil
	}
	client.listFn = func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
		return &translate.ListLanguagesOutput{
			Languages: []translatetypes.Language{
				{LanguageCode: aws.String("es"), LanguageName: aws.String("Spanish")},
			},
		}, nil
	}

	langs, err = svc.GetSupportedLanguages(context.Background())
	require.NoError(t, err)
	require.Len(t, langs, 1)
	require.Equal(t, "es", langs[0].Code)
	require.Equal(t, 1, client.listCalls)
	require.Equal(t, 1, dynamo.putCalls)
}

func TestService_DetectLanguage_UsesTranslateAutoDetection(t *testing.T) {
	t.Parallel()

	client := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return &translate.TranslateTextOutput{
				TranslatedText:     aws.String("Hello"),
				SourceLanguageCode: aws.String("es"),
			}, nil
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client:       client,
		dynamoClient: &fakeDynamo{getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) { return &dynamodb.GetItemOutput{}, nil }, putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) { return &dynamodb.PutItemOutput{}, nil }},
		tableName:    "tbl",
		logger:       zap.NewNop(),
		cacheEnabled: false,
		cacheTTL:     time.Hour,
	}

	lang, confidence, err := svc.DetectLanguage(context.Background(), "hola")
	require.NoError(t, err)
	require.Equal(t, "es", lang)
	require.Equal(t, float32(1.0), confidence)
}

func TestService_TranslateHTML_StripsTagsBeforeTranslation(t *testing.T) {
	t.Parallel()

	client := &fakeTranslate{
		translateFn: func(_ context.Context, in *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			require.Equal(t, "Hello world", aws.ToString(in.Text))
			return &translate.TranslateTextOutput{
				TranslatedText:     aws.String("Hola mundo"),
				SourceLanguageCode: aws.String("en"),
			}, nil
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client:       client,
		dynamoClient: &fakeDynamo{getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) { return &dynamodb.GetItemOutput{}, nil }, putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) { return &dynamodb.PutItemOutput{}, nil }},
		tableName:    "tbl",
		logger:       zap.NewNop(),
		cacheEnabled: false,
		cacheTTL:     time.Hour,
	}

	out, detected, err := svc.TranslateHTML(context.Background(), "<p>Hello<br/>world</p>", "en", "es")
	require.NoError(t, err)
	require.Equal(t, "Hola mundo", out)
	require.Equal(t, "en", detected)
}

func TestService_TranslateText_TranslateErrorSurfaces(t *testing.T) {
	t.Parallel()

	client := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return nil, errors.New("boom")
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client:       client,
		dynamoClient: &fakeDynamo{getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) { return &dynamodb.GetItemOutput{}, nil }, putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) { return &dynamodb.PutItemOutput{}, nil }},
		tableName:    "tbl",
		logger:       zap.NewNop(),
		cacheEnabled: false,
		cacheTTL:     time.Hour,
	}

	_, _, err := svc.TranslateText(context.Background(), "hello", "", "es")
	require.Error(t, err)
	require.Contains(t, err.Error(), "translation failed")
}

func TestService_GetSupportedLanguages_ListErrorSurfaces(t *testing.T) {
	t.Parallel()

	client := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return &translate.TranslateTextOutput{}, nil
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return nil, errors.New("boom")
		},
	}

	svc := &Service{
		client:       client,
		dynamoClient: &fakeDynamo{getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) { return &dynamodb.GetItemOutput{}, nil }, putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) { return &dynamodb.PutItemOutput{}, nil }},
		tableName:    "tbl",
		logger:       zap.NewNop(),
		cacheEnabled: false,
		cacheTTL:     time.Hour,
	}

	_, err := svc.GetSupportedLanguages(context.Background())
	require.Error(t, err)
}

func TestService_DetectLanguage_ErrorSurfaces(t *testing.T) {
	t.Parallel()

	client := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return nil, errors.New("boom")
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client:       client,
		dynamoClient: &fakeDynamo{getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) { return &dynamodb.GetItemOutput{}, nil }, putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) { return &dynamodb.PutItemOutput{}, nil }},
		tableName:    "tbl",
		logger:       zap.NewNop(),
		cacheEnabled: false,
		cacheTTL:     time.Hour,
	}

	_, _, err := svc.DetectLanguage(context.Background(), "hola")
	require.Error(t, err)
}

func TestService_TranslateHTML_PropagatesTranslateError(t *testing.T) {
	t.Parallel()

	client := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return nil, errors.New("boom")
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client:       client,
		dynamoClient: &fakeDynamo{getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) { return &dynamodb.GetItemOutput{}, nil }, putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) { return &dynamodb.PutItemOutput{}, nil }},
		tableName:    "tbl",
		logger:       zap.NewNop(),
		cacheEnabled: false,
		cacheTTL:     time.Hour,
	}

	_, _, err := svc.TranslateHTML(context.Background(), "<p>x</p>", "", "es")
	require.Error(t, err)
}

func TestCacheParsing_InvalidShapesReturnNil(t *testing.T) {
	t.Parallel()

	svc := &Service{logger: zap.NewNop()}

	langs, err := svc.parseLanguagesFromCache(map[string]dynamotypes.AttributeValue{})
	require.NoError(t, err)
	require.Nil(t, langs)

	langs, err = svc.parseLanguagesFromCache(map[string]dynamotypes.AttributeValue{
		"Languages": &dynamotypes.AttributeValueMemberS{Value: "not a list"},
	})
	require.NoError(t, err)
	require.Nil(t, langs)

	require.Nil(t, svc.parseLanguageItem(&dynamotypes.AttributeValueMemberS{Value: "x"}))
	require.Equal(t, "", svc.extractStringValue(map[string]dynamotypes.AttributeValue{}, "missing"))
	require.Equal(t, "", svc.extractStringValue(map[string]dynamotypes.AttributeValue{"Code": &dynamotypes.AttributeValueMemberN{Value: "1"}}, "Code"))
	require.Nil(t, svc.parseLanguageItem(&dynamotypes.AttributeValueMemberM{Value: map[string]dynamotypes.AttributeValue{}}))
}

func TestService_CacheReadErrorsSurfaceFromHelpers(t *testing.T) {
	t.Parallel()

	dynamo := &fakeDynamo{
		getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
			return nil, errors.New("boom")
		},
		putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	svc := &Service{
		client:       &fakeTranslate{translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) { return &translate.TranslateTextOutput{}, nil }, listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) { return &translate.ListLanguagesOutput{}, nil }},
		dynamoClient: dynamo,
		tableName:    "tbl",
		logger:       zap.NewNop(),
		cacheEnabled: true,
		cacheTTL:     time.Hour,
	}

	_, err := svc.getCachedLanguages(context.Background())
	require.Error(t, err)

	_, err = svc.getCachedTranslation(context.Background(), "k")
	require.Error(t, err)
}
