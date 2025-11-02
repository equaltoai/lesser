#!/usr/bin/env python3
"""
Create test posts for GraphQL validation
"""
import os
import sys
import time
import requests

GRAPHQL_ENDPOINT = os.getenv("GRAPHQL_ENDPOINT", "https://dev.lesser.host/api/graphql")
TOKEN = os.getenv("GRAPHQL_TOKEN")

if not TOKEN:
    print("ERROR: GRAPHQL_TOKEN environment variable is required")
    sys.exit(1)

def graphql_request(query, variables=None):
    """Execute a GraphQL request"""
    headers = {
        "Authorization": TOKEN if TOKEN.startswith("Bearer ") else f"Bearer {TOKEN}",
        "Content-Type": "application/json"
    }
    
    payload = {"query": query}
    if variables:
        payload["variables"] = variables
    
    response = requests.post(GRAPHQL_ENDPOINT, json=payload, headers=headers, timeout=30)
    
    try:
        return response.json()
    except:
        print(f"Failed to parse response: {response.text}")
        return None

def create_post(content, visibility="PUBLIC"):
    """Create a test post"""
    mutation = """
    mutation CreateNote($input: CreateNoteInput!) {
        createNote(input: $input) {
            object {
                id
                content
                visibility
                createdAt
            }
        }
    }
    """
    
    variables = {
        "input": {
            "content": content,
            "visibility": visibility
        }
    }
    
    result = graphql_request(mutation, variables)
    if not result:
        print(f"✗ Failed to create post: No response")
        return None
    if "data" in result and result.get("data") and result["data"].get("createNote"):
        print(f"✓ Created post: {content[:50]}...")
        return result["data"]["createNote"]["object"] if result["data"]["createNote"].get("object") else result["data"]["createNote"]
    elif "errors" in result:
        print(f"✗ Failed to create post: {result['errors']}")
        return None
    else:
        print(f"✗ Failed to create post: {result}")
        return None

def main():
    print(f"Creating test posts on {GRAPHQL_ENDPOINT}...")
    print()
    
    # Create a variety of test posts
    posts = [
        "Hello from the GraphQL validation tests! #lesser #testing",
        "This is a public post with some interesting content about ActivityPub. #activitypub #federation",
        "Testing media-free posts for the public timeline. #lesser #graphql",
        "Another test post to populate the timeline with meaningful content.",
        "Final test post with multiple hashtags #lesser #graphql #validation #testing",
    ]
    
    created = 0
    for i, content in enumerate(posts, 1):
        print(f"[{i}/{len(posts)}] Creating post...")
        if create_post(content):
            created += 1
            time.sleep(1)  # Rate limiting
    
    print()
    print(f"Summary: Created {created}/{len(posts)} posts successfully")
    
    if created < len(posts):
        sys.exit(1)

if __name__ == "__main__":
    main()
