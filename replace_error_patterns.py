#!/usr/bin/env python3
"""
Script to systematically replace duplicated error response patterns
with the common.error_responses framework
"""

import re
import os
import subprocess

# Common patterns to replace
REPLACEMENTS = [
    # 400 Bad Request patterns
    (r'ctx\.Status\(http\.StatusBadRequest\)\.JSON\(map\[string\]string\{"error": err\.Error\(\)\}\)', 'common.RespondValidationError(ctx, err)'),
    (r'ctx\.Status\(400\)\.JSON\(map\[string\]string\{"error": err\.Error\(\)\}\)', 'common.RespondValidationError(ctx, err)'),
    (r'ctx\.Status\(http\.StatusBadRequest\)\.JSON\(map\[string\]string\{"error": "([^"]+)"\}\)', r'common.RespondBadRequest(ctx, "\1")'),
    (r'ctx\.Status\(400\)\.JSON\(map\[string\]string\{"error": "([^"]+)"\}\)', r'common.RespondBadRequest(ctx, "\1")'),
    
    # 401 Unauthorized patterns
    (r'ctx\.Status\(http\.StatusUnauthorized\)\.JSON\(map\[string\]string\{"error": "unauthorized"\}\)', 'common.RespondUnauthorized(ctx)'),
    (r'ctx\.Status\(401\)\.JSON\(map\[string\]string\{"error": "unauthorized"\}\)', 'common.RespondUnauthorized(ctx)'),
    (r'ctx\.Status\(http\.StatusUnauthorized\)\.JSON\(map\[string\]string\{"error": "missing token"\}\)', 'common.RespondMissingAuth(ctx)'),
    (r'ctx\.Status\(401\)\.JSON\(map\[string\]string\{"error": "missing token"\}\)', 'common.RespondMissingAuth(ctx)'),
    (r'ctx\.Status\(http\.StatusUnauthorized\)\.JSON\(map\[string\]string\{"error": err\.Error\(\)\}\)', 'common.RespondUnauthorized(ctx, err.Error())'),
    (r'ctx\.Status\(401\)\.JSON\(map\[string\]string\{"error": err\.Error\(\)\}\)', 'common.RespondUnauthorized(ctx, err.Error())'),
    (r'ctx\.Status\(http\.StatusUnauthorized\)\.JSON\(map\[string\]string\{"error": "([^"]+)"\}\)', r'common.RespondUnauthorized(ctx, "\1")'),
    (r'ctx\.Status\(401\)\.JSON\(map\[string\]string\{"error": "([^"]+)"\}\)', r'common.RespondUnauthorized(ctx, "\1")'),
    
    # 403 Forbidden patterns
    (r'ctx\.Status\(http\.StatusForbidden\)\.JSON\(map\[string\]string\{"error": "insufficient scope"\}\)', 'common.RespondInsufficientScope(ctx)'),
    (r'ctx\.Status\(403\)\.JSON\(map\[string\]string\{"error": "insufficient scope"\}\)', 'common.RespondInsufficientScope(ctx)'),
    (r'ctx\.Status\(http\.StatusForbidden\)\.JSON\(map\[string\]string\{"error": "([^"]+)"\}\)', r'common.RespondForbidden(ctx, "\1")'),
    (r'ctx\.Status\(403\)\.JSON\(map\[string\]string\{"error": "([^"]+)"\}\)', r'common.RespondForbidden(ctx, "\1")'),
    
    # 404 Not Found patterns
    (r'ctx\.Status\(http\.StatusNotFound\)\.JSON\(map\[string\]string\{"error": "account not found"\}\)', 'common.RespondAccountNotFound(ctx)'),
    (r'ctx\.Status\(404\)\.JSON\(map\[string\]string\{"error": "account not found"\}\)', 'common.RespondAccountNotFound(ctx)'),
    (r'ctx\.Status\(http\.StatusNotFound\)\.JSON\(map\[string\]string\{"error": "status not found"\}\)', 'common.RespondStatusNotFound(ctx)'),
    (r'ctx\.Status\(404\)\.JSON\(map\[string\]string\{"error": "status not found"\}\)', 'common.RespondStatusNotFound(ctx)'),
    (r'ctx\.Status\(http\.StatusNotFound\)\.JSON\(map\[string\]string\{"error": "user not found"\}\)', 'common.RespondUserNotFound(ctx)'),
    (r'ctx\.Status\(404\)\.JSON\(map\[string\]string\{"error": "user not found"\}\)', 'common.RespondUserNotFound(ctx)'),
    (r'ctx\.Status\(http\.StatusNotFound\)\.JSON\(map\[string\]string\{"error": "actor not found"\}\)', 'common.RespondActorNotFound(ctx)'),
    (r'ctx\.Status\(404\)\.JSON\(map\[string\]string\{"error": "actor not found"\}\)', 'common.RespondActorNotFound(ctx)'),
    (r'ctx\.Status\(http\.StatusNotFound\)\.JSON\(map\[string\]string\{"error": fmt\.Sprintf\("([^"]*)", ([^)]+)\)\}\)', r'common.RespondNotFound(ctx, fmt.Sprintf("\1", \2))'),
    (r'ctx\.Status\(404\)\.JSON\(map\[string\]string\{"error": fmt\.Sprintf\("([^"]*)", ([^)]+)\)\}\)', r'common.RespondNotFound(ctx, fmt.Sprintf("\1", \2))'),
    (r'ctx\.Status\(http\.StatusNotFound\)\.JSON\(map\[string\]string\{"error": "([^"]+)"\}\)', r'common.RespondNotFound(ctx, "\1")'),
    (r'ctx\.Status\(404\)\.JSON\(map\[string\]string\{"error": "([^"]+)"\}\)', r'common.RespondNotFound(ctx, "\1")'),
    
    # 422 Unprocessable Entity patterns
    (r'ctx\.Status\(http\.StatusUnprocessableEntity\)\.JSON\(map\[string\]string\{"error": "([^"]+)"\}\)', r'common.RespondUnprocessableEntity(ctx, "\1")'),
    (r'ctx\.Status\(422\)\.JSON\(map\[string\]string\{"error": "([^"]+)"\}\)', r'common.RespondUnprocessableEntity(ctx, "\1")'),
    
    # 500 Internal Server Error patterns
    (r'ctx\.Status\(http\.StatusInternalServerError\)\.JSON\(map\[string\]string\{"error": "internal server error"\}\)', 'common.RespondInternalServerError(ctx)'),
    (r'ctx\.Status\(500\)\.JSON\(map\[string\]string\{"error": "internal server error"\}\)', 'common.RespondInternalServerError(ctx)'),
    (r'ctx\.Status\(http\.StatusInternalServerError\)\.JSON\(map\[string\]string\{"error": "database error"\}\)', 'common.RespondDatabaseError(ctx)'),
    (r'ctx\.Status\(500\)\.JSON\(map\[string\]string\{"error": "database error"\}\)', 'common.RespondDatabaseError(ctx)'),
    (r'ctx\.Status\(http\.StatusInternalServerError\)\.JSON\(map\[string\]string\{"error": "([^"]+)"\}\)', r'common.RespondInternalServerError(ctx, "\1")'),
    (r'ctx\.Status\(500\)\.JSON\(map\[string\]string\{"error": "([^"]+)"\}\)', r'common.RespondInternalServerError(ctx, "\1")'),
]

def process_file(filepath):
    """Process a single file to replace error patterns"""
    try:
        with open(filepath, 'r') as f:
            content = f.read()
        
        original_content = content
        replacements_made = 0
        
        # Apply all replacements
        for pattern, replacement in REPLACEMENTS:
            new_content = re.sub(pattern, replacement, content)
            if new_content != content:
                replacements_made += len(re.findall(pattern, content))
                content = new_content
        
        # Only write if changes were made
        if content != original_content:
            with open(filepath, 'w') as f:
                f.write(content)
            print(f"Processed {filepath}: {replacements_made} replacements")
            return replacements_made
        
        return 0
        
    except Exception as e:
        print(f"Error processing {filepath}: {e}")
        return 0

def main():
    """Process all Go files in cmd/api/lift/"""
    directory = "cmd/api/lift"
    total_replacements = 0
    
    for root, dirs, files in os.walk(directory):
        for file in files:
            if file.endswith('.go'):
                filepath = os.path.join(root, file)
                replacements = process_file(filepath)
                total_replacements += replacements
    
    print(f"\nTotal replacements made: {total_replacements}")
    
    # Test compilation
    print("\nTesting compilation...")
    result = subprocess.run(['go', 'build', './cmd/api/lift/...'], 
                          capture_output=True, text=True)
    if result.returncode == 0:
        print("Compilation successful!")
    else:
        print("Compilation failed:")
        print(result.stderr)

if __name__ == "__main__":
    main()