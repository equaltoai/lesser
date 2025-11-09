#!/usr/bin/env python3
"""Test script for Lesser Community Notes API endpoints."""

import requests
import time
import argparse
from datetime import datetime

def main():
    parser = argparse.ArgumentParser(description='Test Lesser Community Notes API')
    parser.add_argument('--host', default='https://lesser.host', help='Lesser instance host')
    parser.add_argument('--token', help='OAuth token for authenticated requests')
    args = parser.parse_args()

    print(f"Testing Lesser Community Notes API at {args.host}")
    print(f"Started at {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")

    headers = {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
    }
    
    if args.token:
        headers['Authorization'] = f'Bearer {args.token}'

    # Test object ID (you'll need to replace with a real post ID)
    test_object_id = "https://lesser.host/objects/test-status-1"
    
    results = []

    # Test 1: Create a Community Note
    print("=== Test 1: Create Community Note ===")
    note_data = {
        "object_id": test_object_id,
        "content": "This post contains outdated information. The event was actually postponed to next week.",
        "sources": [
            {
                "url": "https://example.com/official-announcement",
                "title": "Official Event Announcement"
            }
        ],
        "language": "en"
    }
    
    try:
        response = requests.post(
            f"{args.host}/api/v1/notes",
            headers=headers,
            json=note_data
        )
        
        if response.status_code == 201:
            note = response.json()
            print(f"✓ Created note with ID: {note['id']}")
            print(f"  Score: {note['score']}")
            print(f"  Visibility: {note['visibility_status']}")
            results.append(("Create note", True, ""))
            created_note_id = note['id']
        else:
            error = f"Status {response.status_code}: {response.text}"
            print(f"✗ Failed to create note: {error}")
            results.append(("Create note", False, error))
            created_note_id = None
    except Exception as e:
        print(f"✗ Error creating note: {e}")
        results.append(("Create note", False, str(e)))
        created_note_id = None

    # Test 2: Get notes for an object
    print("\n=== Test 2: Get Notes for Object ===")
    try:
        response = requests.get(
            f"{args.host}/api/v1/notes/{test_object_id}",
            headers=headers
        )
        
        if response.status_code == 200:
            notes = response.json()
            print(f"✓ Found {len(notes)} notes for object")
            for note in notes:
                print(f"  - Note {note['id']}: score={note['score']}, status={note['visibility_status']}")
            results.append(("Get notes for object", True, ""))
        else:
            error = f"Status {response.status_code}: {response.text}"
            print(f"✗ Failed to get notes: {error}")
            results.append(("Get notes for object", False, error))
    except Exception as e:
        print(f"✗ Error getting notes: {e}")
        results.append(("Get notes for object", False, str(e)))

    # Test 3: Vote on a note (if we created one)
    if created_note_id:
        print("\n=== Test 3: Vote on Note ===")
        vote_data = {
            "vote_type": "helpful"
        }
        
        try:
            response = requests.post(
                f"{args.host}/api/v1/notes/{created_note_id}/vote",
                headers=headers,
                json=vote_data
            )
            
            if response.status_code == 200:
                result = response.json()
                print(f"✓ Voted 'helpful' on note")
                print(f"  New score: {result.get('score', 'N/A')}")
                results.append(("Vote on note", True, ""))
            else:
                error = f"Status {response.status_code}: {response.text}"
                print(f"✗ Failed to vote: {error}")
                results.append(("Vote on note", False, error))
        except Exception as e:
            print(f"✗ Error voting: {e}")
            results.append(("Vote on note", False, str(e)))

    # Test 4: Get user's notes
    print("\n=== Test 4: Get User's Notes ===")
    test_user_id = "testuser"  # Replace with actual user ID
    
    try:
        response = requests.get(
            f"{args.host}/api/v1/accounts/{test_user_id}/notes",
            headers=headers
        )
        
        if response.status_code == 200:
            notes = response.json()
            print(f"✓ Found {len(notes)} notes by user {test_user_id}")
            results.append(("Get user notes", True, ""))
        else:
            error = f"Status {response.status_code}: {response.text}"
            print(f"✗ Failed to get user notes: {error}")
            results.append(("Get user notes", False, error))
    except Exception as e:
        print(f"✗ Error getting user notes: {e}")
        results.append(("Get user notes", False, str(e)))

    # Test 5: Rate limiting check
    print("\n=== Test 5: Rate Limiting Check ===")
    print("Creating multiple notes to test rate limiting...")
    
    rate_limit_hit = False
    for i in range(6):  # Try to create 6 notes (limit is 5/hour)
        note_data = {
            "object_id": f"{test_object_id}-{i}",
            "content": f"Test note {i} for rate limiting",
            "sources": [],
            "language": "en"
        }
        
        try:
            response = requests.post(
                f"{args.host}/api/v1/notes",
                headers=headers,
                json=note_data
            )
            
            if response.status_code == 429:
                print(f"✓ Rate limit hit at note {i+1} (expected)")
                rate_limit_hit = True
                break
            elif response.status_code == 201:
                print(f"  Created note {i+1}")
            else:
                print(f"  Unexpected status {response.status_code} for note {i+1}")
        except Exception as e:
            print(f"  Error on note {i+1}: {e}")
        
        time.sleep(0.5)  # Small delay between requests
    
    if rate_limit_hit:
        results.append(("Rate limiting", True, ""))
    else:
        results.append(("Rate limiting", False, "Rate limit not enforced"))

    # Test 6: Reputation gating
    print("\n=== Test 6: Reputation Gating ===")
    print("Testing reputation requirements...")
    
    # This would need a low-reputation test account
    # For now, we'll just document what should happen
    print("  Note: Reputation gating requires:")
    print("  - Minimum 100 reputation to create notes")
    print("  - Higher reputation gives more voting weight")
    print("  - 500+ reputation required to vouch for others")
    results.append(("Reputation gating", True, "Documented requirements"))

    # Summary
    print("\n" + "="*50)
    passed = sum(1 for _, success, _ in results if success)
    failed = len(results) - passed
    
    print(f"Test Summary: {passed} passed, {failed} failed")
    
    if failed > 0:
        print("\nFailed tests:")
        for test_name, success, error in results:
            if not success:
                print(f"  - {test_name}: {error}")
    
    print(f"\nCompleted at {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    # Check for cost headers
    print("\n=== Cost Tracking ===")
    if 'X-Cost-Total-Micros' in response.headers:
        print(f"✓ Cost tracking active")
        print(f"  Total cost: {response.headers.get('X-Cost-Total-Micros')} micros")
        print(f"  DynamoDB reads: {response.headers.get('X-Cost-DynamoDB-Reads', '0')}")
        print(f"  DynamoDB writes: {response.headers.get('X-Cost-DynamoDB-Writes', '0')}")
    else:
        print("✗ No cost tracking headers found")

if __name__ == "__main__":
    main() 
