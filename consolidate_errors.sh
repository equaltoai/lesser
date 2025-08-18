#!/bin/bash

# Script to consolidate error response duplications across all API files
# This will systematically replace error patterns with consolidated common functions

LIFT_DIR="/Users/aronprice/lesser/cmd/api/lift"

# Common error patterns to replace
declare -A replacements=(
    ['ctx.Status(400).JSON(map[string]string{"error": err.Error()})']='common.RespondValidationError(ctx, err)'
    ['ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})']='common.RespondUnauthorized(ctx)'
    ['ctx.Status(401).JSON(map[string]string{"error": "authorization required"})']='common.RespondMissingAuth(ctx)'
    ['ctx.Status(401).JSON(map[string]string{"error": "invalid token"})']='common.RespondInvalidToken(ctx)'
    ['ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})']='common.RespondInsufficientScope(ctx)'
    ['ctx.Status(403).JSON(map[string]string{"error": "Forbidden"})']='common.RespondForbidden(ctx)'
    ['ctx.Status(403).JSON(map[string]string{"error": err.Error()})']='common.RespondForbidden(ctx, err.Error())'
    ['ctx.Status(404).JSON(map[string]string{"error": "Not found"})']='common.RespondNotFound(ctx)'
    ['ctx.Status(404).JSON(map[string]string{"error": "not found"})']='common.RespondNotFound(ctx)'
    ['ctx.Status(404).JSON(map[string]string{"error": "Account not found"})']='common.RespondAccountNotFound(ctx)'
    ['ctx.Status(404).JSON(map[string]string{"error": "Status not found"})']='common.RespondStatusNotFound(ctx)'
    ['ctx.Status(404).JSON(map[string]string{"error": "status not found"})']='common.RespondStatusNotFound(ctx)'
    ['ctx.Status(404).JSON(map[string]string{"error": "User not found"})']='common.RespondUserNotFound(ctx)'
    ['ctx.Status(404).JSON(map[string]string{"error": "user not found"})']='common.RespondUserNotFound(ctx)'
    ['ctx.Status(404).JSON(map[string]string{"error": "Actor not found"})']='common.RespondActorNotFound(ctx)'
    ['ctx.Status(404).JSON(map[string]string{"error": "actor not found"})']='common.RespondActorNotFound(ctx)'
    ['ctx.Status(422).JSON(map[string]string{"error": err.Error()})']='common.RespondUnprocessableEntity(ctx, err.Error())'
    ['ctx.Status(422).JSON(map[string]string{"error": "status text too long"})']='common.RespondStatusTooLong(ctx)'
    ['ctx.Status(422).JSON(map[string]string{"error": "invalid content"})']='common.RespondInvalidContent(ctx)'
    ['ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})']='common.RespondInternalServerError(ctx)'
    ['ctx.Status(500).JSON(map[string]string{"error": "internal server error"})']='common.RespondInternalServerError(ctx)'
    ['ctx.Status(500).JSON(map[string]string{"error": "database error"})']='common.RespondDatabaseError(ctx)'
    ['ctx.Status(400).JSON(map[string]string{"error": "invalid request format"})']='common.RespondInvalidRequest(ctx)'
    ['ctx.Status(400).JSON(map[string]string{"error": "invalid request"})']='common.RespondInvalidRequest(ctx)'
    ['ctx.Status(400).JSON(map[string]string{"error": "missing account id"})']='common.RespondMissingAccountID(ctx)'
    ['ctx.Status(400).JSON(map[string]string{"error": "missing status id"})']='common.RespondMissingStatusID(ctx)'
)

# Specific error patterns with resource names
declare -A resource_patterns=(
    ['failed to create']='common.RespondFailedToCreate(ctx, "{resource}")'
    ['failed to update']='common.RespondFailedToUpdate(ctx, "{resource}")'
    ['failed to delete']='common.RespondFailedToDelete(ctx, "{resource}")'
    ['failed to get']='common.RespondFailedToGet(ctx, "{resource}")'
)

echo "Starting error response consolidation..."

# Count initial duplications
initial_count=$(grep -r "ctx\.Status([0-9]*).JSON(map\[string\]string{\"error\"" "$LIFT_DIR" | wc -l)
echo "Initial duplications: $initial_count"

# Process each .go file in the lift directory
for file in "$LIFT_DIR"/*.go; do
    if [[ -f "$file" ]]; then
        filename=$(basename "$file")
        echo "Processing $filename..."
        
        # Count duplications in this file before processing
        before_count=$(grep -c "ctx\.Status([0-9]*).JSON(map\[string\]string{\"error\"" "$file")
        
        if [[ $before_count -gt 0 ]]; then
            echo "  Found $before_count duplications in $filename"
            
            # Apply each replacement pattern
            for pattern in "${!replacements[@]}"; do
                replacement="${replacements[$pattern]}"
                # Use sed to replace the pattern (escape special characters)
                escaped_pattern=$(echo "$pattern" | sed 's/[[\.*^$()+?{|]/\\&/g')
                escaped_replacement=$(echo "$replacement" | sed 's/[[\.*^$()+?{|]/\\&/g')
                sed -i.bak "s/$escaped_pattern/$escaped_replacement/g" "$file"
            done
            
            # Count duplications after processing
            after_count=$(grep -c "ctx\.Status([0-9]*).JSON(map\[string\]string{\"error\"" "$file")
            eliminated=$((before_count - after_count))
            
            if [[ $eliminated -gt 0 ]]; then
                echo "  Eliminated $eliminated duplications from $filename"
            fi
        fi
    fi
done

# Count final duplications
final_count=$(grep -r "ctx\.Status([0-9]*).JSON(map\[string\]string{\"error\"" "$LIFT_DIR" | wc -l)
total_eliminated=$((initial_count - final_count))

echo ""
echo "Consolidation complete!"
echo "Initial duplications: $initial_count"
echo "Final duplications: $final_count"
echo "Total eliminated: $total_eliminated"
echo ""

# Show remaining duplications by file
echo "Remaining duplications by file:"
for file in "$LIFT_DIR"/*.go; do
    if [[ -f "$file" ]]; then
        count=$(grep -c "ctx\.Status([0-9]*).JSON(map\[string\]string{\"error\"" "$file")
        if [[ $count -gt 0 ]]; then
            echo "  $(basename "$file"): $count"
        fi
    fi
done

# Clean up backup files
rm -f "$LIFT_DIR"/*.bak

echo "Script completed successfully!"