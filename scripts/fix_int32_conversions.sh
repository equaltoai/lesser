#!/bin/bash
# Script to replace all aws.Int32(int32(...)) with safeInt32(...) in DynamoDB files

# Array of files that need updating
files=(
    "pkg/storage/dynamodb/objects.go"
    "pkg/storage/dynamodb/actor.go"
    "pkg/storage/dynamodb/status_search_strategies.go"
    "pkg/storage/dynamodb/domain_blocks.go"
    "pkg/storage/dynamodb/federation.go"
    "pkg/storage/dynamodb/relationships.go"
    "pkg/storage/dynamodb/mutes.go"
    "pkg/storage/dynamodb/hashtags.go"
    "pkg/storage/dynamodb/collections.go"
    "pkg/storage/dynamodb/search_popularity.go"
    "pkg/storage/dynamodb/notifications.go"
    "pkg/storage/dynamodb/reports.go"
    "pkg/storage/dynamodb/trust.go"
    "pkg/storage/dynamodb/search_service.go"
    "pkg/storage/dynamodb/flags.go"
    "pkg/storage/dynamodb/moderation.go"
    "pkg/storage/dynamodb/activity.go"
    "pkg/storage/dynamodb/severed_relationships.go"
    "pkg/storage/dynamodb/announces.go"
    "pkg/storage/dynamodb/search_analytics.go"
    "pkg/storage/dynamodb/hashtag_follow.go"
    "pkg/storage/dynamodb/likes.go"
    "pkg/storage/dynamodb/reputation.go"
    "pkg/federation/routing/query_optimizer.go"
    "pkg/federation/routing/instance_registry.go"
    "pkg/moderation/advanced/threat_intel.go"
    "pkg/moderation/advanced/reputation.go"
)

echo "Starting replacements..."

for file in "${files[@]}"; do
    if [ -f "$file" ]; then
        echo "Processing $file..."
        # Replace aws.Int32(int32(limit)) with safeInt32(limit)
        sed -i.bak 's/aws\.Int32(int32(\([^)]*\)))/safeInt32(\1)/g' "$file"
        # Remove backup files
        rm -f "$file.bak"
    else
        echo "Warning: $file not found"
    fi
done

echo "Replacements complete!"
echo "Please run 'go build ./...' to verify all files compile correctly." 