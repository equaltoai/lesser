package repositories

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	ttquery "github.com/theory-cloud/tabletheory/v3/pkg/query"
)

func TestActivityHydrationContract_TableTheoryEmbeddedBaseObject(t *testing.T) {
	const (
		activityID = "https://example.com/activities/follow-contract"
		actorID    = "https://example.com/users/alice"
		objectID   = "https://remote.social/users/bob"
	)

	item := map[string]types.AttributeValue{
		"PK":     &types.AttributeValueMemberS{Value: "ACTOR#alice"},
		"SK":     &types.AttributeValueMemberS{Value: "ACTIVITY#2026-04-18T00:00:00Z#" + activityID},
		"gsi2PK": &types.AttributeValueMemberS{Value: "ACTIVITYID#" + activityID},
		"gsi2SK": &types.AttributeValueMemberS{Value: "ACTIVITY#2026-04-18T00:00:00Z#" + activityID},
		"activity": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"id":     &types.AttributeValueMemberS{Value: activityID},
			"type":   &types.AttributeValueMemberS{Value: activitypub.FollowType},
			"actor":  &types.AttributeValueMemberS{Value: actorID},
			"object": &types.AttributeValueMemberS{Value: objectID},
		}},
	}

	var out models.Activity
	require.NoError(t, ttquery.UnmarshalItem(item, &out))
	require.NotNil(t, out.Activity)
	require.Equal(t, activityID, out.Activity.ID)
	require.Equal(t, activitypub.FollowType, out.Activity.Type)
	require.Equal(t, actorID, out.Activity.Actor)
	require.Equal(t, objectID, out.Activity.Object)
}
