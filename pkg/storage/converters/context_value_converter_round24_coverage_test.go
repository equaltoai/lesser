package converters

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/require"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

func TestRegisterContextConverters_NotExtendedDB_Round24(t *testing.T) {
	db := &mocks.MockDB{}
	require.Error(t, RegisterContextConverters(db))
}

func TestContextValueConverter_ToAttributeValue_TypedNilPointer_Round24(t *testing.T) {
	converter := ContextValueConverter{}

	var ptr *int
	av, err := converter.ToAttributeValue(ptr)
	require.NoError(t, err)

	_, ok := av.(*types.AttributeValueMemberNULL)
	require.True(t, ok)
}

func TestContextValueConverter_ToAttributeValue_UnsupportedType_Round24(t *testing.T) {
	converter := ContextValueConverter{}
	_, err := converter.ToAttributeValue(123)
	require.Error(t, err)
}

func TestContextValueConverter_ToAttributeValue_MarshalError_Round24(t *testing.T) {
	converter := ContextValueConverter{}

	ctx := activitypub.ContextValue{map[struct{ K int }]string{{K: 1}: "a"}}
	_, err := converter.ToAttributeValue(ctx)
	require.Error(t, err)
}

func TestContextValueConverter_FromAttributeValue_NULLAndDefault_Round24(t *testing.T) {
	converter := ContextValueConverter{}

	var ctx activitypub.ContextValue
	require.NoError(t, converter.FromAttributeValue(&types.AttributeValueMemberNULL{Value: true}, &ctx))
	require.Nil(t, ctx)

	av := &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"k": &types.AttributeValueMemberS{Value: "v"},
	}}
	require.NoError(t, converter.FromAttributeValue(av, &ctx))
	require.Len(t, ctx, 1)
	require.Contains(t, ctx[0], "k")
}

func TestContextValueConverter_FromAttributeValue_TargetErrors_Round24(t *testing.T) {
	converter := ContextValueConverter{}
	av := &types.AttributeValueMemberNULL{Value: true}

	require.Error(t, converter.FromAttributeValue(av, nil))

	var nonPtr activitypub.ContextValue
	require.Error(t, converter.FromAttributeValue(av, nonPtr))

	var wrong []string
	require.Error(t, converter.FromAttributeValue(av, &wrong))

	var wrongPtr *string
	require.Error(t, converter.FromAttributeValue(av, &wrongPtr))
}
