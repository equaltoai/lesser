package translation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/translate"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	theorydbErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	theorydbmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

type fakeTranslate struct {
	translateCalls int
	listCalls      int

	translateFn func(ctx context.Context, params *translate.TranslateTextInput) (*translate.TranslateTextOutput, error)
	listFn      func(ctx context.Context, params *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error)
}

func (f *fakeTranslate) TranslateText(ctx context.Context, params *translate.TranslateTextInput, _ ...func(*translate.Options)) (*translate.TranslateTextOutput, error) {
	f.translateCalls++
	return f.translateFn(ctx, params)
}

func (f *fakeTranslate) ListLanguages(ctx context.Context, params *translate.ListLanguagesInput, _ ...func(*translate.Options)) (*translate.ListLanguagesOutput, error) {
	f.listCalls++
	return f.listFn(ctx, params)
}

func TestService_TranslateText_CacheHitSkipsTranslate(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)

	db := new(theorydbmocks.MockDB)
	query := new(theorydbmocks.MockQuery)

	svc := &Service{
		client: &fakeTranslate{
			translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
				t.Fatalf("TranslateText should not be called on cache hit")
				return nil, nil
			},
			listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
				return &translate.ListLanguagesOutput{}, nil
			},
		},
		db:           db,
		logger:       zap.NewNop(),
		cacheEnabled: true,
		cacheTTL:     time.Hour,
	}

	cacheKey := svc.generateCacheKey("hello", "auto", "es")
	pk := "TRANSLATION#" + cacheKey

	db.On("Model", mock.MatchedBy(func(model any) bool {
		item, ok := model.(*translationCacheItem)
		return ok && item.PK == pk && item.SK == "RESULT"
	})).Return(query).Once()

	query.On("WithContext", mock.Anything).Return(query)
	query.On("Where", "PK", "=", pk).Return(query)
	query.On("Where", "SK", "=", "RESULT").Return(query)
	query.On("First", mock.Anything).
		Run(func(args mock.Arguments) {
			item := args.Get(0).(*translationCacheItem)
			item.TranslatedText = "hola"
			item.DetectedLanguage = "es"
			item.CachedAt = now
		}).
		Return(nil)

	text, lang, err := svc.TranslateText(context.Background(), "hello", "auto", "es")
	require.NoError(t, err)
	require.Equal(t, "hola", text)
	require.Equal(t, "es", lang)
}

func TestService_TranslateText_CacheMissCallsTranslateAndCaches(t *testing.T) {
	t.Parallel()

	db := new(theorydbmocks.MockDB)
	readQuery := new(theorydbmocks.MockQuery)
	writeQuery := new(theorydbmocks.MockQuery)

	translator := &fakeTranslate{
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
		client:       translator,
		db:           db,
		logger:       zap.NewNop(),
		cacheEnabled: true,
		cacheTTL:     time.Hour,
	}

	cacheKey := svc.generateCacheKey("hello", "auto", "es")
	pk := "TRANSLATION#" + cacheKey

	db.On("Model", mock.MatchedBy(func(model any) bool {
		item, ok := model.(*translationCacheItem)
		return ok && item.PK == pk && item.SK == "RESULT"
	})).Return(readQuery).Once()

	readQuery.On("WithContext", mock.Anything).Return(readQuery)
	readQuery.On("Where", "PK", "=", pk).Return(readQuery)
	readQuery.On("Where", "SK", "=", "RESULT").Return(readQuery)
	readQuery.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound)

	db.On("Model", mock.MatchedBy(func(model any) bool {
		item, ok := model.(*translationCacheItem)
		if !ok {
			return false
		}
		return item.PK == pk &&
			item.SK == "RESULT" &&
			item.TranslatedText == "hola" &&
			item.DetectedLanguage == "es" &&
			!item.CachedAt.IsZero() &&
			item.TTL > item.CachedAt.Unix()
	})).Return(writeQuery).Once()

	writeQuery.On("WithContext", mock.Anything).Return(writeQuery)
	writeQuery.On("CreateOrUpdate").Return(nil)

	text, lang, err := svc.TranslateText(context.Background(), "hello", "auto", "es")
	require.NoError(t, err)
	require.Equal(t, "hola", text)
	require.Equal(t, "es", lang)
	require.Equal(t, 1, translator.translateCalls)
}

func TestService_TranslateText_CacheWriteFailureDoesNotFailRequest(t *testing.T) {
	t.Parallel()

	db := new(theorydbmocks.MockDB)
	readQuery := new(theorydbmocks.MockQuery)
	writeQuery := new(theorydbmocks.MockQuery)

	translator := &fakeTranslate{
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
		client:       translator,
		db:           db,
		logger:       zap.NewNop(),
		cacheEnabled: true,
		cacheTTL:     time.Hour,
	}

	cacheKey := svc.generateCacheKey("hello", "", "es")
	pk := "TRANSLATION#" + cacheKey

	db.On("Model", mock.Anything).Return(readQuery).Once()
	readQuery.On("WithContext", mock.Anything).Return(readQuery)
	readQuery.On("Where", "PK", "=", pk).Return(readQuery)
	readQuery.On("Where", "SK", "=", "RESULT").Return(readQuery)
	readQuery.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound)

	db.On("Model", mock.Anything).Return(writeQuery).Once()
	writeQuery.On("WithContext", mock.Anything).Return(writeQuery)
	writeQuery.On("CreateOrUpdate").Return(errors.New("write failed"))

	text, lang, err := svc.TranslateText(context.Background(), "hello", "", "es")
	require.NoError(t, err)
	require.Equal(t, "hola", text)
	require.Equal(t, "es", lang)
}

func TestService_GetSupportedLanguages_CacheHitSkipsTranslateAPI(t *testing.T) {
	t.Parallel()

	db := new(theorydbmocks.MockDB)
	query := new(theorydbmocks.MockQuery)

	translator := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return &translate.TranslateTextOutput{}, nil
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			t.Fatalf("ListLanguages should not be called on cache hit")
			return nil, nil
		},
	}

	svc := &Service{
		client:       translator,
		db:           db,
		logger:       zap.NewNop(),
		cacheEnabled: true,
		cacheTTL:     time.Hour,
	}

	db.On("Model", mock.Anything).Return(query).Once()
	query.On("WithContext", mock.Anything).Return(query)
	query.On("Where", "PK", "=", "CACHE#LANGUAGES").Return(query)
	query.On("Where", "SK", "=", "SUPPORTED").Return(query)
	query.On("First", mock.Anything).
		Run(func(args mock.Arguments) {
			item := args.Get(0).(*supportedLanguagesCacheItem)
			item.Languages = []LanguageInfo{
				{Code: "en", Name: "English"},
				{Code: "es", Name: "Spanish"},
			}
		}).
		Return(nil)

	langs, err := svc.GetSupportedLanguages(context.Background())
	require.NoError(t, err)
	require.Len(t, langs, 2)
	require.Equal(t, []LanguageInfo{
		{Code: "en", Name: "English"},
		{Code: "es", Name: "Spanish"},
	}, langs)
}
