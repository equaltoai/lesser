package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	securityPublicationPrefix       = "PUBLICATION#"
	securityPublicationMemberSuffix = "#MEMBER"
	securityPublicationUserPrefix   = "USER#"
)

func executeSecurityFindingsCMSPublicationMemberRepair(
	ctx context.Context,
	client securityFindingsMigrationClient,
	tableName string,
	apply bool,
	limit int,
) (securityFindingsOperationSummary, error) {
	summary := securityFindingsOperationSummary{Name: "cms-publication-members"}

	items, err := scanSecurityFindingsItems(ctx, client, tableName,
		"begins_with(PK, :pkPrefix) AND begins_with(SK, :skPrefix)",
		map[string]types.AttributeValue{
			":pkPrefix": &types.AttributeValueMemberS{Value: securityPublicationPrefix},
			":skPrefix": &types.AttributeValueMemberS{Value: securityPublicationUserPrefix},
		},
	)
	if err != nil {
		return summary, fmt.Errorf("scan publication member rows: %w", err)
	}

	for _, item := range items {
		summary.Scanned++
		candidate, sample, ok, skipped := buildCMSPublicationMemberRepair(item)
		if skipped {
			summary.Skipped++
			appendSecurityFindingsSample(&summary, sample)
			continue
		}
		if !ok {
			continue
		}
		if securityFindingsCandidateLimitReached(&summary, limit) {
			break
		}

		summary.Candidates++
		summary.PlannedWrites++
		appendSecurityFindingsSample(&summary, sample)
		if apply {
			_, err := client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName: aws.String(tableName),
				Key:       migrationDynamoKey(candidate.PK, candidate.SK),
				UpdateExpression: aws.String(
					"SET gsi1PK = :gsi1pk, gsi1SK = :gsi1sk, publicationID = :publicationID, userID = :userID",
				),
				ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":gsi1pk":        &types.AttributeValueMemberS{Value: candidate.GSI1PK},
					":gsi1sk":        &types.AttributeValueMemberS{Value: candidate.GSI1SK},
					":publicationID": &types.AttributeValueMemberS{Value: candidate.PublicationID},
					":userID":        &types.AttributeValueMemberS{Value: candidate.UserID},
				},
			})
			if err != nil {
				return summary, fmt.Errorf("repair publication member %s %s: %w", candidate.PK, candidate.SK, err)
			}
			summary.AppliedWrites++
		}
	}

	return summary, nil
}

type cmsPublicationMemberRepairCandidate struct {
	PK            string
	SK            string
	PublicationID string
	UserID        string
	GSI1PK        string
	GSI1SK        string
}

func buildCMSPublicationMemberRepair(item map[string]types.AttributeValue) (cmsPublicationMemberRepairCandidate, string, bool, bool) {
	pk, sk, ok := migrationItemKey(item)
	if !ok {
		return cmsPublicationMemberRepairCandidate{}, "publication member row missing PK/SK", false, true
	}
	publicationID, userID, ok := parseCMSPublicationMemberKey(pk, sk)
	if !ok {
		return cmsPublicationMemberRepairCandidate{}, fmt.Sprintf("publication member row has unexpected key %s %s", pk, sk), false, true
	}

	desired := cmsPublicationMemberRepairCandidate{
		PK:            pk,
		SK:            sk,
		PublicationID: publicationID,
		UserID:        userID,
		GSI1PK:        fmt.Sprintf("USER#%s#PUBLICATION", userID),
		GSI1SK:        fmt.Sprintf("PUBLICATION#%s", publicationID),
	}

	currentPublicationID, _ := attributeString(item["publicationID"])
	currentUserID, _ := attributeString(item["userID"])
	currentGSI1PK, _ := attributeString(item["gsi1PK"])
	currentGSI1SK, _ := attributeString(item["gsi1SK"])
	if currentPublicationID == desired.PublicationID &&
		currentUserID == desired.UserID &&
		currentGSI1PK == desired.GSI1PK &&
		currentGSI1SK == desired.GSI1SK {
		return cmsPublicationMemberRepairCandidate{}, "", false, false
	}

	return desired, fmt.Sprintf("repair %s %s -> %s/%s", pk, sk, desired.GSI1PK, desired.GSI1SK), true, false
}

func parseCMSPublicationMemberKey(pk string, sk string) (string, string, bool) {
	if !strings.HasPrefix(pk, securityPublicationPrefix) || !strings.HasSuffix(pk, securityPublicationMemberSuffix) {
		return "", "", false
	}
	publicationID := strings.TrimSuffix(strings.TrimPrefix(pk, securityPublicationPrefix), securityPublicationMemberSuffix)
	if publicationID == "" || strings.Contains(publicationID, "#") {
		return "", "", false
	}
	if !strings.HasPrefix(sk, securityPublicationUserPrefix) {
		return "", "", false
	}
	userID := strings.TrimPrefix(sk, securityPublicationUserPrefix)
	if strings.TrimSpace(userID) == "" {
		return "", "", false
	}
	return publicationID, userID, true
}
