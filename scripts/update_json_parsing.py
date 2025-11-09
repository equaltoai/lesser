#!/usr/bin/env python3
"""
Script to help update JSON parsing to use safe parsing functions.
This identifies patterns and suggests replacements.
"""

import re
import os
import sys
from pathlib import Path

# Patterns to find and their replacements
PATTERNS = [
    {
        'name': 'json.Unmarshal for request bodies',
        'pattern': r'json\.Unmarshal\(.*?\[\]byte\(request\.Body\).*?\)',
        'suggestion': 'common.ParseRequestBody([]byte(request.Body), &variable)',
        'files': 'cmd/api/handlers/*.go'
    },
    {
        'name': 'json.Unmarshal for ActivityPub objects',
        'pattern': r'json\.Unmarshal\(.*?,\s*&(activity|note|actor|object)',
        'suggestion': 'common.ParseActivityPubObject(data, &variable)',
        'files': ['cmd/inbox/*.go', 'cmd/outbox/*.go', 'pkg/federation/*.go']
    },
    {
        'name': 'json.NewDecoder for HTTP responses',
        'pattern': r'json\.NewDecoder\(resp\.Body\)\.Decode',
        'suggestion': 'common.ParseHTTPResponse(resp.Body, &variable)',
        'files': '**/*.go'
    },
    {
        'name': 'generic json.Unmarshal',
        'pattern': r'json\.Unmarshal\(',
        'suggestion': 'Consider using common.ParseRequestBody or common.ParseActivityPubObject',
        'files': '**/*.go'
    }
]

def find_files(pattern):
    """Find files matching the given pattern."""
    if isinstance(pattern, str):
        patterns = [pattern]
    else:
        patterns = pattern
    
    files = []
    for p in patterns:
        files.extend(Path('.').glob(p))
    return files

def scan_file(filepath, pattern_info):
    """Scan a file for the given pattern."""
    try:
        with open(filepath, 'r') as f:
            content = f.read()
            
        matches = []
        for match in re.finditer(pattern_info['pattern'], content, re.MULTILINE):
            line_num = content[:match.start()].count('\n') + 1
            line = content.split('\n')[line_num - 1].strip()
            matches.append({
                'line': line_num,
                'text': line,
                'match': match.group(0)
            })
        
        return matches
    except Exception as e:
        print(f"Error reading {filepath}: {e}")
        return []

def main():
    print("Scanning for JSON parsing patterns to update...\n")
    
    total_found = 0
    
    for pattern_info in PATTERNS:
        print(f"\n=== {pattern_info['name']} ===")
        print(f"Pattern: {pattern_info['pattern']}")
        print(f"Suggestion: {pattern_info['suggestion']}")
        print("-" * 80)
        
        files = find_files(pattern_info['files'])
        pattern_found = 0
        
        for filepath in files:
            # Skip test files and vendor
            if 'test' in str(filepath) or 'vendor' in str(filepath):
                continue
                
            matches = scan_file(filepath, pattern_info)
            if matches:
                print(f"\n{filepath}:")
                for match in matches:
                    print(f"  Line {match['line']}: {match['text']}")
                    pattern_found += len(matches)
        
        if pattern_found == 0:
            print("  No matches found")
        else:
            print(f"\nTotal: {pattern_found} matches")
            total_found += pattern_found
    
    print(f"\n\nGrand total: {total_found} potential updates needed")
    
    # Generate sed commands for simple replacements
    print("\n\n=== Suggested sed commands for simple replacements ===")
    print("# For request body parsing in API handlers:")
    print("find cmd/api/handlers -name '*.go' -exec sed -i.bak \\")
    print("  's/json\\.Unmarshal(\\[\\]byte(request\\.Body), \\&/common.ParseRequestBody([]byte(request.Body), \\&/g' {} \\;")
    
    print("\n# Review changes with:")
    print("git diff")
    
    print("\n# Remove backup files after review:")
    print("find . -name '*.bak' -delete")

if __name__ == '__main__':
    main() 