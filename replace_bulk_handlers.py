#!/usr/bin/env python3
"""
Script to replace duplicated bulk operation handlers with the generic framework
"""

import re
import os

def replace_bulk_handler(file_content, handler_name, operation_type, id_key, validator_entity, processor_func, start_message):
    """Replace a specific bulk handler with the generic framework call"""
    
    # Pattern to match the entire handler function
    pattern = rf'// handle{handler_name} handles.*?\nfunc \(ach \*AsyncCommandHandler\) handle{handler_name}\(ctx context\.Context, conn \*streaming\.ConnectionInfo, cmd \*streaming\.Command\) \(\*streaming\.CommandResponse, error\) \{{[^}}]*?if authErr := ach\.RequireAuth\(conn, cmd\.ID\); authErr != nil \{{[^}}]*?return authErr, nil[^}}]*?\}}[^}}]*?if validationErr := ach\.ValidatePayload\(cmd\.Payload, \[\]string\{{"[^"]*"}\}, cmd\.ID\); validationErr != nil \{{[^}}]*?return validationErr, nil[^}}]*?\}}[^}}]*?(?:{id_key}|accountIDs|statusIDs|memberData) := ach\.Get[^;]*;[^}}]*?if err := common\.ValidateEntityIDsList\([^)]*\); err != nil \{{[^}}]*?return ach\.CreateErrorResponse\([^)]*\), nil[^}}]*?\}}[^}}]*?if err := common\.ValidateIntRange\([^)]*\); err != nil \{{[^}}]*?return ach\.CreateErrorResponse\([^)]*\), nil[^}}]*?\}}[^}}]*?// Start async processing[^}}]*?go [^;]*;[^}}]*?// Return immediate response indicating processing started[^}}]*?data := map\[string\]interface\{{\}}[^}}]*?"operation_id": cmd\.ID,[^}}]*?"status":\s*"processing",[^}}]*?"total":[^,]*,[^}}]*?"processed":\s*0,[^}}]*?"message":[^,]*,[^}}]*?\}}[^}}]*?return ach\.CreateSuccessResponse\(cmd\.ID, data\), nil[^}}]*?\}}'
    
    # Simplified replacement - we'll match the function signature and replace manually
    func_start = f'func (ach *AsyncCommandHandler) handle{handler_name}(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {{'
    
    if func_start in file_content:
        # Find the function boundaries
        start_idx = file_content.find(func_start)
        if start_idx == -1:
            return file_content
        
        # Find the matching closing brace
        brace_count = 0
        current_idx = start_idx + len(func_start)
        
        while current_idx < len(file_content):
            if file_content[current_idx] == '{':
                brace_count += 1
            elif file_content[current_idx] == '}':
                if brace_count == 0:
                    # Found the closing brace
                    end_idx = current_idx + 1
                    break
                brace_count -= 1
            current_idx += 1
        else:
            return file_content  # Couldn't find closing brace
        
        # Replace with the new implementation
        new_implementation = f'''func (ach *AsyncCommandHandler) handle{handler_name}(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {{
\tconfig := BulkOperationConfig{{
\t\tOperationType:   "{operation_type}",
\t\tIDListKey:       "{id_key}",
\t\tValidatorEntity: "{validator_entity}",
\t\tProcessorFunc:   ach.{processor_func},
\t\tStartMessage:    "{start_message}",
\t}}
\treturn ach.handleBulkOperation(ctx, conn, cmd, config)
}}'''
        
        # Preserve the comment before the function
        comment_pattern = rf'// handle{handler_name} handles[^\n]*\n'
        comment_match = re.search(comment_pattern, file_content[:start_idx][::-1])
        if comment_match:
            comment_start = start_idx - comment_match.end()
            file_content = file_content[:comment_start] + f'// handle{handler_name} handles {operation_type} operations\n' + new_implementation + file_content[end_idx:]
        else:
            file_content = file_content[:start_idx] + new_implementation + file_content[end_idx:]
    
    return file_content

def main():
    file_path = "pkg/streaming/handlers/async_commands.go"
    
    # Read the file
    with open(file_path, 'r') as f:
        content = f.read()
    
    # Define bulk handlers to replace
    handlers = [
        ("BulkUnmute", "unmute", "account_ids", "account", "processBulkUnmute", "Bulk unmute operation started"),
        ("BulkBlock", "block", "account_ids", "account", "processBulkBlock", "Bulk block operation started"),
        ("BulkUnblock", "unblock", "account_ids", "account", "processBulkUnblock", "Bulk unblock operation started"),
        ("BulkMute", "mute", "account_ids", "account", "processBulkMute", "Bulk mute operation started"),
        ("BulkDelete", "delete", "status_ids", "status", "processBulkDelete", "Bulk delete operation started"),
        ("BulkArchive", "archive", "status_ids", "status", "processBulkArchive", "Bulk archive operation started"),
        ("BulkRestore", "restore", "status_ids", "status", "processBulkRestore", "Bulk restore operation started"),
    ]
    
    # Apply replacements
    for handler_name, operation_type, id_key, validator_entity, processor_func, start_message in handlers:
        content = replace_bulk_handler(content, handler_name, operation_type, id_key, validator_entity, processor_func, start_message)
        print(f"Processed {handler_name}")
    
    # Write the updated content
    with open(file_path, 'w') as f:
        f.write(content)
    
    print("All bulk handlers have been consolidated")

if __name__ == "__main__":
    main()