package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ConvertStatusToGraphQLObject exposes the internal status → GraphQL Object conversion.
//
// This is used by out-of-band processors (for example stream routers) that need to
// produce GraphQL subscription payloads without executing a full GraphQL operation.
func (r *Resolver) ConvertStatusToGraphQLObject(ctx context.Context, status *models.Status) *model.Object {
	return r.convertStatusToObject(ctx, status)
}

// ConvertConversationToGraphQL exposes the internal conversation → GraphQL Conversation conversion.
//
// This is used by out-of-band processors (for example stream routers) that need to
// produce GraphQL subscription payloads without executing a full GraphQL operation.
func (r *Resolver) ConvertConversationToGraphQL(ctx context.Context, conv *models.Conversation) *model.Conversation {
	return r.convertConversationToGraphQL(ctx, conv)
}
