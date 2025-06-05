#!/bin/bash

# Migration script for API Gateway v1 to v2
# This script handles the safe automated parts of the migration

echo "Starting API Gateway v1 to v2 migration..."

# Step 1: Create backup
echo "Creating backup..."
cp -r pkg pkg.backup
cp -r cmd cmd.backup

# Step 2: Replace type names in all Go files
echo "Replacing type names..."
find pkg cmd -name "*.go" -type f -exec sed -i.bak \
  -e 's/events\.APIGatewayProxyRequest/events.APIGatewayV2HTTPRequest/g' \
  -e 's/events\.APIGatewayProxyResponse/events.APIGatewayV2HTTPResponse/g' \
  {} +

# Step 3: Update function signatures to return pointers
echo "Updating function signatures to return pointers..."
find pkg cmd -name "*.go" -type f -exec sed -i.bak \
  -e 's/\(func.*\) events\.APIGatewayV2HTTPResponse/\1 *events.APIGatewayV2HTTPResponse/g' \
  -e 's/\(func.*\) (events\.APIGatewayV2HTTPResponse/\1 (*events.APIGatewayV2HTTPResponse/g' \
  {} +

# Step 4: Find files that need manual updates
echo ""
echo "Files that need manual updates for request.Path -> request.RawPath:"
grep -r "request\.Path[^P]" pkg cmd --include="*.go" | grep -v ".bak" || echo "None found"

echo ""
echo "Files that need manual updates for request.HTTPMethod:"
grep -r "request\.HTTPMethod" pkg cmd --include="*.go" | grep -v ".bak" || echo "None found"

echo ""
echo "Files that need & added to return statements:"
grep -r "return events\.APIGatewayV2HTTPResponse{" pkg cmd --include="*.go" | grep -v ".bak" | head -20

echo ""
echo "Migration script complete. Manual steps required:"
echo "1. Change request.Path to request.RawPath"
echo "2. Change request.HTTPMethod to request.RequestContext.HTTP.Method"  
echo "3. Add & to all 'return events.APIGatewayV2HTTPResponse{' statements"
echo "4. Update auth middleware to handle v2 requests"
echo "5. Test each handler"
echo ""
echo "Backup created in *.backup directories" 