package translation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/translate"
	translatetypes "github.com/aws/aws-sdk-go-v2/service/translate/types"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	storagemocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	theorydbmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestService_DetectLanguage_UsesTranslateText(t *testing.T) {
	t.Parallel()

	translator := &fakeTranslate{
		translateFn: func(_ context.Context, in *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			require.Equal(t, "bonjour", aws.ToString(in.Text))
			require.Equal(t, "en", aws.ToString(in.TargetLanguageCode))
			require.Nil(t, in.SourceLanguageCode)
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
		client: translator,
		logger: zap.NewNop(),
	}

	lang, confidence, err := svc.DetectLanguage(context.Background(), "bonjour")
	require.NoError(t, err)
	require.Equal(t, "fr", lang)
	require.Equal(t, float32(1.0), confidence)
}

func TestService_TranslateHTML_StripsAndTranslates(t *testing.T) {
	t.Parallel()

	translator := &fakeTranslate{
		translateFn: func(_ context.Context, in *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			require.Equal(t, "Hello world", aws.ToString(in.Text))
			require.Equal(t, "es", aws.ToString(in.TargetLanguageCode))
			require.Nil(t, in.SourceLanguageCode)
			return &translate.TranslateTextOutput{
				TranslatedText:     aws.String("hola"),
				SourceLanguageCode: aws.String("en"),
			}, nil
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client: translator,
		logger: zap.NewNop(),
	}

	text, detectedLang, err := svc.TranslateHTML(context.Background(), "<p>Hello<br/>world</p>", "auto", "es")
	require.NoError(t, err)
	require.Equal(t, "hola", text)
	require.Equal(t, "en", detectedLang)
}

func TestService_GetSupportedLanguages_CacheMissCallsTranslateAPIAndCaches(t *testing.T) {
	t.Parallel()

	db := new(theorydbmocks.MockDB)
	readQuery := new(theorydbmocks.MockQuery)
	writeQuery := new(theorydbmocks.MockQuery)

	translator := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			t.Fatalf("TranslateText should not be called when listing languages")
			return nil, nil
		},
		listFn: func(_ context.Context, in *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			require.NotNil(t, in.MaxResults)
			require.Equal(t, int32(100), *in.MaxResults)
			return &translate.ListLanguagesOutput{
				Languages: []translatetypes.Language{
					{LanguageCode: aws.String("en"), LanguageName: aws.String("English")},
					{LanguageCode: aws.String("es"), LanguageName: aws.String("Spanish")},
				},
			}, nil
		},
	}

	svc := &Service{
		client:       translator,
		db:           db,
		logger:       zap.NewNop(),
		cacheEnabled: true,
		cacheTTL:     time.Hour,
	}

	db.On("Model", mock.MatchedBy(func(model any) bool {
		item, ok := model.(*supportedLanguagesCacheItem)
		return ok && item.PK == "CACHE#LANGUAGES" && item.SK == "SUPPORTED"
	})).Return(readQuery).Once()
	readQuery.On("WithContext", mock.Anything).Return(readQuery)
	readQuery.On("Where", "PK", "=", "CACHE#LANGUAGES").Return(readQuery)
	readQuery.On("Where", "SK", "=", "SUPPORTED").Return(readQuery)
	readQuery.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound)

	db.On("Model", mock.MatchedBy(func(model any) bool {
		item, ok := model.(*supportedLanguagesCacheItem)
		if !ok {
			return false
		}
		if item.PK != "CACHE#LANGUAGES" || item.SK != "SUPPORTED" {
			return false
		}
		if len(item.Languages) != 2 {
			return false
		}
		if item.Languages[0] != (LanguageInfo{Code: "en", Name: "English"}) {
			return false
		}
		if item.Languages[1] != (LanguageInfo{Code: "es", Name: "Spanish"}) {
			return false
		}
		return !item.CachedAt.IsZero() && item.TTL > item.CachedAt.Unix()
	})).Return(writeQuery).Once()
	writeQuery.On("WithContext", mock.Anything).Return(writeQuery)
	writeQuery.On("CreateOrUpdate").Return(nil)

	langs, err := svc.GetSupportedLanguages(context.Background())
	require.NoError(t, err)
	require.Equal(t, []LanguageInfo{
		{Code: "en", Name: "English"},
		{Code: "es", Name: "Spanish"},
	}, langs)
	require.Equal(t, 1, translator.listCalls)
}

func TestCacheItems_TableName_DefaultsToTestTableDuringGoTest(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "")
	t.Setenv("DYNAMO_TABLE_NAME", "")
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("STAGE", "")

	require.Equal(t, "test-table", translationCacheItem{}.TableName())
	require.Equal(t, "test-table", supportedLanguagesCacheItem{}.TableName())
}

func TestNewService_UsesStoreDBAndDefaultsLogger(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	store := new(storagemocks.MockRepositoryStorage)
	db := new(theorydbmocks.MockDB)
	store.On("GetDB").Return(db).Once()

	svc, err := NewService(context.Background(), &lesserconfig.Config{}, store, nil, false)
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.NotNil(t, svc.client)
	require.Equal(t, db, svc.db)
	require.False(t, svc.cacheEnabled)
	require.Equal(t, 30*24*time.Hour, svc.cacheTTL)
	require.NotNil(t, svc.logger)

	store.AssertExpectations(t)
}

func TestService_GetCachedTranslation_ReturnsErrorWhenUninitialized(t *testing.T) {
	t.Parallel()

	var nilSvc *Service
	_, err := nilSvc.getCachedTranslation(context.Background(), "k")
	require.Error(t, err)

	svc := &Service{}
	_, err = svc.getCachedTranslation(context.Background(), "k")
	require.Error(t, err)
}

func TestService_GetCachedTranslation_PropagatesNonNotFoundError(t *testing.T) {
	t.Parallel()

	db := new(theorydbmocks.MockDB)
	query := new(theorydbmocks.MockQuery)

	svc := &Service{db: db, logger: zap.NewNop()}

	db.On("Model", mock.MatchedBy(func(model any) bool {
		item, ok := model.(*translationCacheItem)
		return ok && item.PK == "TRANSLATION#k" && item.SK == "RESULT"
	})).Return(query).Once()

	query.On("WithContext", mock.Anything).Return(query)
	query.On("Where", "PK", "=", "TRANSLATION#k").Return(query)
	query.On("Where", "SK", "=", "RESULT").Return(query)
	query.On("First", mock.Anything).Return(errors.New("boom"))

	_, err := svc.getCachedTranslation(context.Background(), "k")
	require.Error(t, err)
}

func TestService_GetSupportedLanguages_CacheWriteFailureDoesNotFailRequest(t *testing.T) {
	t.Parallel()

	db := new(theorydbmocks.MockDB)
	readQuery := new(theorydbmocks.MockQuery)
	writeQuery := new(theorydbmocks.MockQuery)

	translator := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return &translate.TranslateTextOutput{}, nil
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{
				Languages: []translatetypes.Language{
					{LanguageCode: aws.String("en"), LanguageName: aws.String("English")},
					{LanguageCode: aws.String("es"), LanguageName: aws.String("Spanish")},
				},
			}, nil
		},
	}

	svc := &Service{
		client:       translator,
		db:           db,
		logger:       zap.NewNop(),
		cacheEnabled: true,
		cacheTTL:     time.Hour,
	}

	db.On("Model", mock.MatchedBy(func(model any) bool {
		item, ok := model.(*supportedLanguagesCacheItem)
		return ok && item.PK == "CACHE#LANGUAGES" && item.SK == "SUPPORTED"
	})).Return(readQuery).Once()
	readQuery.On("WithContext", mock.Anything).Return(readQuery)
	readQuery.On("Where", "PK", "=", "CACHE#LANGUAGES").Return(readQuery)
	readQuery.On("Where", "SK", "=", "SUPPORTED").Return(readQuery)
	readQuery.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound)

	db.On("Model", mock.MatchedBy(func(model any) bool {
		item, ok := model.(*supportedLanguagesCacheItem)
		return ok && item.PK == "CACHE#LANGUAGES" && item.SK == "SUPPORTED" && len(item.Languages) == 2
	})).Return(writeQuery).Once()
	writeQuery.On("WithContext", mock.Anything).Return(writeQuery)
	writeQuery.On("CreateOrUpdate").Return(errors.New("write failed"))

	langs, err := svc.GetSupportedLanguages(context.Background())
	require.NoError(t, err)
	require.Equal(t, []LanguageInfo{
		{Code: "en", Name: "English"},
		{Code: "es", Name: "Spanish"},
	}, langs)
}

func TestService_GetSupportedLanguages_ListLanguagesError_ReturnsError(t *testing.T) {
	t.Parallel()

	translator := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return &translate.TranslateTextOutput{}, nil
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return nil, errors.New("list boom")
		},
	}

	svc := &Service{
		client: translator,
		logger: zap.NewNop(),
	}

	_, err := svc.GetSupportedLanguages(context.Background())
	require.Error(t, err)
}

func TestService_TranslateText_TranslateAPIError_ReturnsError(t *testing.T) {
	t.Parallel()

	translator := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return nil, errors.New("translate boom")
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client: translator,
		logger: zap.NewNop(),
	}

	_, _, err := svc.TranslateText(context.Background(), "hello", "en", "es")
	require.Error(t, err)
}

func TestService_CacheTranslation_ReturnsErrorWhenUninitialized(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	require.Error(t, svc.cacheTranslation(context.Background(), "k", "hola", "es"))
}

func TestService_GetCachedLanguages_ReturnsErrorWhenUninitialized(t *testing.T) {
	t.Parallel()

	var nilSvc *Service
	_, err := nilSvc.getCachedLanguages(context.Background())
	require.Error(t, err)
}

func TestService_GetCachedLanguages_PropagatesNonNotFoundError(t *testing.T) {
	t.Parallel()

	db := new(theorydbmocks.MockDB)
	query := new(theorydbmocks.MockQuery)

	svc := &Service{db: db, logger: zap.NewNop()}

	db.On("Model", mock.MatchedBy(func(model any) bool {
		item, ok := model.(*supportedLanguagesCacheItem)
		return ok && item.PK == "CACHE#LANGUAGES" && item.SK == "SUPPORTED"
	})).Return(query).Once()

	query.On("WithContext", mock.Anything).Return(query)
	query.On("Where", "PK", "=", "CACHE#LANGUAGES").Return(query)
	query.On("Where", "SK", "=", "SUPPORTED").Return(query)
	query.On("First", mock.Anything).Return(errors.New("boom"))

	_, err := svc.getCachedLanguages(context.Background())
	require.Error(t, err)
}

func TestService_DetectLanguage_TranslateAPIError_ReturnsError(t *testing.T) {
	t.Parallel()

	translator := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return nil, errors.New("detect boom")
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client: translator,
		logger: zap.NewNop(),
	}

	_, _, err := svc.DetectLanguage(context.Background(), "bonjour")
	require.Error(t, err)
}

func TestService_TranslateHTML_TranslateError_ReturnsError(t *testing.T) {
	t.Parallel()

	translator := &fakeTranslate{
		translateFn: func(_ context.Context, _ *translate.TranslateTextInput) (*translate.TranslateTextOutput, error) {
			return nil, errors.New("translate boom")
		},
		listFn: func(_ context.Context, _ *translate.ListLanguagesInput) (*translate.ListLanguagesOutput, error) {
			return &translate.ListLanguagesOutput{}, nil
		},
	}

	svc := &Service{
		client: translator,
		logger: zap.NewNop(),
	}

	_, _, err := svc.TranslateHTML(context.Background(), "<p>Hello</p>", "auto", "es")
	require.Error(t, err)
}

func TestStripHTMLTags_BreaksOnMalformedTagOrdering(t *testing.T) {
	t.Parallel()

	require.Equal(t, ">oops<", stripHTMLTags(">oops<"))
}
