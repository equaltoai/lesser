#!/bin/bash

# Script to add panic recovery middleware to all Lambda functions
# This adds the panic recovery as the FIRST middleware to catch all panics

set -e

echo "Adding panic recovery middleware to all Lambda functions..."

# List of Lambda function directories
LAMBDA_DIRS=(
    "activity-processor"
    "actor"
    "ai-processor"
    "api"
    "collections"
    "configure-instance"
    "cost-aggregator"
    "dlq-processor"
    "enhanced-federation-processor"
    "export-generator"
    "federation-aggregator"
    "federation-delivery"
    "federation-timeseries"
    "federation-tracker"
    "graphql"
    "import-processor"
    "inbox"
    "init-deploy"
    "media-processor"
    "metrics-aggregator"
    "metrics-processor"
    "moderation-processor"
    "note-processor"
    "notification-processor"
    "objects"
    "outbox"
    "push-delivery"
    "report-trust-updater"
    "search-indexer"
    "status-indexer"
    "streaming"
    "stream-router"
    "trend-aggregator"
    "webfinger"
    "websocket-cost-aggregator"
)

BASE_DIR="/home/aron/ai-workspace/codebases/lesser/cmd"

for dir in "${LAMBDA_DIRS[@]}"; do
    MAIN_FILE="$BASE_DIR/$dir/main.go"
    
    if [ ! -f "$MAIN_FILE" ]; then
        echo "Warning: $MAIN_FILE not found, skipping..."
        continue
    fi
    
    # Check if already has panic recovery
    if grep -q "PanicRecovery" "$MAIN_FILE"; then
        echo "✓ $dir already has panic recovery"
        continue
    fi
    
    # Check if it has lift.New() call
    if ! grep -q "lift.New()" "$MAIN_FILE"; then
        echo "⚠ $dir doesn't use lift framework, skipping..."
        continue
    fi
    
    echo "Adding panic recovery to $dir..."
    
    # Add the import if not present
    if ! grep -q "github.com/equaltoai/lesser/pkg/middleware" "$MAIN_FILE"; then
        # Add import after other imports
        sed -i '/^import (/a\\t"github.com/equaltoai/lesser/pkg/middleware"' "$MAIN_FILE"
    fi
    
    # Add panic recovery as first middleware after app := lift.New()
    # This is a bit complex, so we'll use a temporary file
    TEMP_FILE=$(mktemp)
    
    awk '
    /app := lift\.New\(\)/ {
        print $0
        print ""
        print "\t// Panic recovery middleware (MUST be first to catch all panics)"
        if (match($0, /^[\t ]+/)) {
            indent = substr($0, RSTART, RLENGTH)
        } else {
            indent = "\t"
        }
        print indent "app.Use(middleware.PanicRecovery(lambdaCtx.Logger))"
        next
    }
    {print}
    ' "$MAIN_FILE" > "$TEMP_FILE"
    
    mv "$TEMP_FILE" "$MAIN_FILE"
    
    echo "✓ Added panic recovery to $dir"
done

echo ""
echo "Summary:"
echo "========="
grep -l "PanicRecovery" $BASE_DIR/*/main.go 2>/dev/null | wc -l | xargs echo "Lambda functions with panic recovery:"
echo ""
echo "Done! All Lambda functions now have panic recovery middleware."