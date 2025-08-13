#!/usr/bin/env python3
"""
Script to automatically remove unused functions and variables from Go code.
This reads the linter output and removes the unused code.
"""
import subprocess
import re
import os

def get_unused_items():
    """Get list of unused items from linter output."""
    try:
        result = subprocess.run(['make', 'lint'], 
                              capture_output=True, text=True, cwd='/Users/aronprice/lesser')
        # The linter output is in stdout, not stderr
        output = result.stdout
        
        unused_items = []
        for line in output.split('\n'):
            if 'is unused' in line:
                # Parse line like: "file.go:123:19: func (*Handler).xyz is unused (unused)"
                match = re.match(r'([^:]+):(\d+):\d+:\s*(.+?)\s+is unused', line)
                if match:
                    file_path, line_num, item_info = match.groups()
                    unused_items.append({
                        'file': file_path,
                        'line': int(line_num),
                        'info': item_info.strip()
                    })
        
        return unused_items
    except Exception as e:
        print(f"Error getting linter output: {e}")
        return []

def read_file_lines(file_path):
    """Read file and return lines."""
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            return f.readlines()
    except Exception as e:
        print(f"Error reading {file_path}: {e}")
        return []

def find_function_end(lines, start_line):
    """Find the end of a function starting at start_line."""
    brace_count = 0
    in_function = False
    
    for i in range(start_line - 1, len(lines)):
        line = lines[i].strip()
        
        # Skip empty lines and comments
        if not line or line.startswith('//'):
            continue
            
        # Check for function signature
        if not in_function and ('func ' in line):
            in_function = True
        
        if in_function:
            # Count braces
            brace_count += line.count('{')
            brace_count -= line.count('}')
            
            # If braces are balanced, function is complete
            if brace_count == 0 and '{' in lines[i]:
                return i + 1  # Return line after function end
    
    return start_line  # Fallback

def remove_unused_functions(unused_items):
    """Remove unused functions from files."""
    # Group by file
    files_to_modify = {}
    for item in unused_items:
        if item['file'] not in files_to_modify:
            files_to_modify[item['file']] = []
        files_to_modify[item['file']].append(item)
    
    for file_path, items in files_to_modify.items():
        full_path = os.path.join('/Users/aronprice/lesser', file_path)
        if not os.path.exists(full_path):
            continue
            
        lines = read_file_lines(full_path)
        if not lines:
            continue
        
        # Sort items by line number in reverse order (remove from bottom up)
        items.sort(key=lambda x: x['line'], reverse=True)
        
        # Track removals
        removed_count = 0
        
        for item in items:
            if 'func ' in item['info']:
                # Remove function
                start_line = item['line']
                end_line = find_function_end(lines, start_line)
                
                print(f"Removing function from {file_path}:{start_line}-{end_line}")
                
                # Remove lines (including function comment if present)
                remove_start = start_line - 1
                # Check for comment before function
                if (remove_start > 0 and 
                    lines[remove_start - 1].strip().startswith('//')):
                    remove_start -= 1
                
                del lines[remove_start:end_line]
                removed_count += 1
            elif 'var ' in item['info'] or 'const ' in item['info']:
                # Remove variable/constant - just the single line
                line_index = item['line'] - 1
                if line_index < len(lines):
                    print(f"Removing variable/constant from {file_path}:{item['line']}")
                    del lines[line_index]
                    removed_count += 1
        
        if removed_count > 0:
            # Write modified file
            try:
                with open(full_path, 'w', encoding='utf-8') as f:
                    f.writelines(lines)
                print(f"Modified {file_path}: removed {removed_count} items")
            except Exception as e:
                print(f"Error writing {full_path}: {e}")

def main():
    print("Getting unused items from linter...")
    unused_items = get_unused_items()
    
    if not unused_items:
        print("No unused items found.")
        return
    
    print(f"Found {len(unused_items)} unused items.")
    
    # Only remove functions for now (safer)
    function_items = [item for item in unused_items if 'func ' in item['info']]
    print(f"Found {len(function_items)} unused functions to remove.")
    
    if function_items:
        remove_unused_functions(function_items)
        print("Removed unused functions. Run linter again to check.")

if __name__ == '__main__':
    main()