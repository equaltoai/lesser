package repositories

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

// listTrustRelationshipsByCategory walks the small, fixed trust-category set in
// canonical order, querying each exact partition (PK or GSI1) and merging pages
// up to limit. It is the single shared implementation behind the trust list
// methods of both TrustRepository and UserRepository (separate repository
// surfaces that share this access shape). Cursor values are base64url JSON
// {category,last_sk} blobs; see encode/decodeTrustRelationshipCursor.
//
// partitionPrefix is the key prefix of the exact partition ("TRUST#" for
// outgoing, "TRUSTED#" for incoming on GSI1); index is "" for the base table or
// "gsi1"; keyField/skField are the partition/sort key attribute names; convert
// maps a stored model to the storage view.
func listTrustRelationshipsByCategory(
	ctx context.Context,
	db core.DB,
	subjectID string,
	partitionPrefix string,
	index string,
	keyField string,
	skField string,
	errorContext string,
	limit int,
	cursor string,
	convert func(*models.TrustRelationship) *storage.TrustRelationship,
) ([]*storage.TrustRelationship, string, error) {
	decodedCursor, err := decodeTrustRelationshipCursor(cursor)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "trust relationship", "cursor decode")
	}

	startCategoryIndex := 0
	categoryCursorSK := decodedCursor.LastSK
	if decodedCursor.Category != "" {
		startCategoryIndex = trustCategoryIndex(decodedCursor.Category)
		if startCategoryIndex < 0 {
			return nil, "", ErrorHandler.HandleQueryError(storage.ErrInvalidInput, "trust relationship", "invalid cursor category")
		}
	}

	lastSK := func(model *models.TrustRelationship) string {
		if skField == "gsi1SK" {
			return model.GSI1SK
		}
		return model.SK
	}

	relationships := make([]*storage.TrustRelationship, 0, limit)
	for i := startCategoryIndex; i < len(trustRelationshipCategoryOrder) && len(relationships) < limit; i++ {
		category := trustRelationshipCategoryOrder[i]
		partitionKey := fmt.Sprintf("%s%s#%s", partitionPrefix, subjectID, category)

		remaining := limit - len(relationships)
		query := db.WithContext(ctx).Model(&models.TrustRelationship{})
		if index != "" {
			query = query.Index(index)
		}
		query = query.Where(keyField, "=", partitionKey).
			OrderBy(skField, "ASC").
			Limit(remaining + 1)

		if categoryCursorSK != "" {
			query = query.Where(skField, ">", categoryCursorSK)
		}

		var page []*models.TrustRelationship
		if err := query.All(&page); err != nil {
			return nil, "", ErrorHandler.HandleQueryError(err, "trust relationship", errorContext)
		}

		if len(page) == 0 {
			categoryCursorSK = ""
			continue
		}

		hasMore := len(page) > remaining
		if hasMore {
			page = page[:remaining]
		}

		for _, model := range page {
			relationships = append(relationships, convert(model))
		}

		if hasMore {
			nextCursor := encodeTrustRelationshipCursor(trustRelationshipCursor{
				Category: string(category),
				LastSK:   lastSK(page[len(page)-1]),
			})
			return relationships, nextCursor, nil
		}

		// If we hit the requested limit exactly at a category boundary, advance
		// the cursor to the next category so callers can continue pagination.
		if len(relationships) >= limit {
			if i+1 < len(trustRelationshipCategoryOrder) {
				nextCursor := encodeTrustRelationshipCursor(trustRelationshipCursor{
					Category: string(trustRelationshipCategoryOrder[i+1]),
					LastSK:   "",
				})
				return relationships, nextCursor, nil
			}
			return relationships, "", nil
		}

		categoryCursorSK = ""
	}

	return relationships, "", nil
}
