package converters

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

func TestContextValueConverter_ToAttributeValue(t *testing.T) {
	converter := ContextValueConverter{}

	ctx := activitypub.ContextValue{
		"https://www.w3.org/ns/activitystreams",
		map[string]any{"toot": "http://joinmastodon.org/ns#"},
	}
	av, err := converter.ToAttributeValue(ctx)
	require.NoError(t, err)

	list, ok := av.(*types.AttributeValueMemberL)
	require.True(t, ok, "expected list attribute value")
	require.Len(t, list.Value, 2)

	var decoded []any
	require.NoError(t, attributevalue.Unmarshal(av, &decoded))
	require.Equal(t, []any{
		"https://www.w3.org/ns/activitystreams",
		map[string]any{"toot": "http://joinmastodon.org/ns#"},
	}, decoded)
}

func TestContextValueConverter_ToAttributeValueNil(t *testing.T) {
	converter := ContextValueConverter{}

	av, err := converter.ToAttributeValue((*activitypub.ContextValue)(nil))
	require.NoError(t, err)

	_, ok := av.(*types.AttributeValueMemberNULL)
	require.True(t, ok, "expected NULL attribute value")
}

func TestContextValueConverter_FromAttributeValueLegacyString(t *testing.T) {
	converter := ContextValueConverter{}
	var ctx activitypub.ContextValue

	err := converter.FromAttributeValue(&types.AttributeValueMemberS{Value: "https://www.w3.org/ns/activitystreams"}, &ctx)
	require.NoError(t, err)
	require.Equal(t, activitypub.ContextValue{"https://www.w3.org/ns/activitystreams"}, ctx)
}

func TestContextValueConverter_FromAttributeValueList(t *testing.T) {
	converter := ContextValueConverter{}

	av, err := attributevalue.Marshal([]any{
		"https://www.w3.org/ns/activitystreams",
		map[string]any{"toot": "http://joinmastodon.org/ns#"},
	})
	require.NoError(t, err)

	var ctx activitypub.ContextValue
	require.NoError(t, converter.FromAttributeValue(av, &ctx))

	require.Len(t, ctx, 2)
	require.Equal(t, "https://www.w3.org/ns/activitystreams", ctx[0])
	require.Equal(t, map[string]any{"toot": "http://joinmastodon.org/ns#"}, ctx[1])
}

func TestContextValueConverter_FromAttributeValuePointerPointer(t *testing.T) {
	converter := ContextValueConverter{}
	av := &types.AttributeValueMemberS{Value: "https://www.w3.org/ns/activitystreams"}

	var ctx *activitypub.ContextValue
	require.NoError(t, converter.FromAttributeValue(av, &ctx))

	require.NotNil(t, ctx)
	require.Equal(t, activitypub.ContextValue{"https://www.w3.org/ns/activitystreams"}, *ctx)
}

func TestRegisterContextConverters(t *testing.T) {
	mockDB := mocks.NewMockExtendedDBStrict()
	mockDB.On("RegisterTypeConverter", contextValueType, mock.Anything).Return(nil).Once()

	err := RegisterContextConverters(mockDB)
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
}

func TestRegisterContextConvertersNilDB(t *testing.T) {
	err := RegisterContextConverters(nil)
	require.Error(t, err)
}
