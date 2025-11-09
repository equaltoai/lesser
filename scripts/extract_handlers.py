#!/usr/bin/env python3
"""Extract and organize handlers from the main.go file"""

import re
import os

# Define handler groups
HANDLER_GROUPS = {
    'accounts.go': [
        'handleRegistration',
        'handleVerifyCredentials', 
        'handleUpdateCredentials',
        'handleGetAccount',
        'validateRegistrationRequest',
        'isValidUsername'
    ],
    'statuses.go': [
        'handleCreateStatus',
        'handleFavourite',
        'handleUnfavourite',
        'handleReblog',
        'handleUnreblog',
        'handleDeleteStatus',
        'handleUpdateStatus',
        'handleGetStatus',
        'handleGetStatusContext',
        'handleGetAccountStatuses',
        'objectToStatus',
        'generateRandomString',
        'getStringFromMap'
    ],
    'timelines.go': [
        'handleHomeTimeline',
        'handlePublicTimeline',
        'extractStatusID',
        'extractAccountID',
        'extractUsernameFromActorID'
    ],
    'relationships.go': [
        'handleFollow',
        'handleUnfollow',
        'handleBlock',
        'handleUnblock',
        'handleGetBlocks'
    ],
    'search.go': [
        'handleSearch'
    ],
    'instance.go': [
        'handleGetInstance'
    ],
    'notifications.go': [
        'handleGetNotifications'
    ]
}

# Read the main.go file
with open('cmd/api/main.go', 'r') as f:
    content = f.read()

# Extract function definitions
def extract_function(content, func_name):
    """Extract a complete function definition"""
    # Find the start of the function
    pattern = rf'^func {func_name}\('
    match = re.search(pattern, content, re.MULTILINE)
    if not match:
        print(f"Function {func_name} not found")
        return None
    
    start = match.start()
    
    # Find the end by counting braces
    brace_count = 0
    in_function = False
    end = start
    
    for i in range(start, len(content)):
        if content[i] == '{':
            brace_count += 1
            in_function = True
        elif content[i] == '}':
            brace_count -= 1
            if in_function and brace_count == 0:
                end = i + 1
                break
    
    return content[start:end]

# Extract imports
imports_match = re.search(r'import \((.*?)\)', content, re.DOTALL)
imports = imports_match.group(1) if imports_match else ""

# Create handler files
for filename, functions in HANDLER_GROUPS.items():
    print(f"Creating {filename}...")
    
    # Prepare file content
    file_content = f"""package handlers

import (
{imports}
)

"""
    
    # Add each function
    for func_name in functions:
        func_def = extract_function(content, func_name)
        if func_def:
            # Convert to method on Handler struct
            if func_name.startswith('handle'):
                # Main handler functions
                func_def = re.sub(r'^func handle', 'func (h *Handler) Handle', func_def, flags=re.MULTILINE)
                # Replace global vars with handler fields
                func_def = func_def.replace('cfg.', 'h.cfg.')
                func_def = func_def.replace('store.', 'h.store.')
                func_def = func_def.replace('logger.', 'h.logger.')
                func_def = func_def.replace('authMiddleware.', 'h.authMiddleware.')
            else:
                # Helper functions remain as regular functions
                pass
            
            file_content += "\n" + func_def + "\n"
    
    # Write the file
    output_path = f'cmd/api/handlers/{filename}'
    print(f"Writing {output_path}...")
    # Don't actually write, just print what would be written
    print(f"Would write {len(file_content)} bytes to {output_path}")
    print("First 500 chars:")
    print(file_content[:500])
    print("...")

print("\nDone! Review the generated files and make manual adjustments as needed.") 