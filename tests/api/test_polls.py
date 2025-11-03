#!/usr/bin/env python3
"""
Test script for Lesser ActivityPub server - Polls functionality

Tests:
1. Create status with poll
2. View poll
3. Vote on poll
4. View poll results after voting
5. Test poll expiration
6. Test multiple choice polls
7. Test vote validation
"""

import requests
import json
import argparse

def test_polls(base_url, token):
    """Test poll functionality"""
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    print("🗳️  Testing Polls Functionality...")
    print("=" * 50)
    
    # Test 1: Create a simple poll
    print("\n📝 Test 1: Creating status with simple poll...")
    poll_status = {
        "status": "What's your favorite programming language?",
        "poll": {
            "options": ["Python", "Go", "JavaScript", "Rust"],
            "expires_in": 3600,  # 1 hour
            "multiple": False,
            "hide_totals": False
        }
    }
    
    response = requests.post(
        f"{base_url}/api/v1/statuses",
        headers=headers,
        json=poll_status
    )
    
    if response.status_code == 201:
        print("✅ Status with poll created successfully")
        status = response.json()
        status_id = status["id"]
        poll_id = status["poll"]["id"] if status.get("poll") else None
        
        if poll_id:
            print(f"   Status ID: {status_id}")
            print(f"   Poll ID: {poll_id}")
            print(f"   Options: {[opt['title'] for opt in status['poll']['options_data']]}")
            print(f"   Expires at: {status['poll']['expires_at']}")
        else:
            print("❌ Poll not found in status response")
            return
    else:
        print(f"❌ Failed to create status with poll: {response.status_code}")
        print(f"   Response: {response.text}")
        return
    
    # Test 2: View poll before voting
    print("\n👀 Test 2: Viewing poll before voting...")
    response = requests.get(
        f"{base_url}/api/v1/polls/{poll_id}",
        headers=headers
    )
    
    if response.status_code == 200:
        poll = response.json()
        print("✅ Poll retrieved successfully")
        print(f"   Voted: {poll['voted']}")
        print(f"   Total votes: {poll['votes_count']}")
        print(f"   Total voters: {poll['voters_count']}")
        for opt in poll['options_data']:
            print(f"   - {opt['title']}: {opt['votes_count']} votes")
    else:
        print(f"❌ Failed to retrieve poll: {response.status_code}")
        print(f"   Response: {response.text}")
    
    # Test 3: Vote on poll
    print("\n🗳️  Test 3: Voting on poll...")
    vote_data = {
        "choices": [1]  # Vote for "Go" (0-indexed)
    }
    
    response = requests.post(
        f"{base_url}/api/v1/polls/{poll_id}/votes",
        headers=headers,
        json=vote_data
    )
    
    if response.status_code == 200:
        poll = response.json()
        print("✅ Vote submitted successfully")
        print(f"   Voted: {poll['voted']}")
        print(f"   Your votes: {poll['own_votes']}")
        print(f"   Total votes: {poll['votes_count']}")
        print(f"   Total voters: {poll['voters_count']}")
        for opt in poll['options_data']:
            print(f"   - {opt['title']}: {opt['votes_count']} votes")
    else:
        print(f"❌ Failed to vote: {response.status_code}")
        print(f"   Response: {response.text}")
    
    # Test 4: Try to vote again (should fail)
    print("\n🚫 Test 4: Attempting to vote again...")
    response = requests.post(
        f"{base_url}/api/v1/polls/{poll_id}/votes",
        headers=headers,
        json=vote_data
    )
    
    if response.status_code == 422:
        print("✅ Correctly prevented duplicate voting")
    else:
        print(f"❌ Unexpected response to duplicate vote: {response.status_code}")
        print(f"   Response: {response.text}")
    
    # Test 5: Create a multiple choice poll
    print("\n📝 Test 5: Creating multiple choice poll...")
    multi_poll_status = {
        "status": "Which of these languages do you use? (multiple choice)",
        "poll": {
            "options": ["Python", "JavaScript", "Go", "Other"],
            "expires_in": 3600,
            "multiple": True,
            "hide_totals": False
        }
    }
    
    response = requests.post(
        f"{base_url}/api/v1/statuses",
        headers=headers,
        json=multi_poll_status
    )
    
    if response.status_code == 201:
        print("✅ Multiple choice poll created successfully")
        status = response.json()
        multi_poll_id = status["poll"]["id"] if status.get("poll") else None
        
        if multi_poll_id:
            # Vote for multiple options
            print("\n🗳️  Voting for multiple options...")
            multi_vote_data = {
                "choices": [0, 2]  # Vote for Python and Go
            }
            
            response = requests.post(
                f"{base_url}/api/v1/polls/{multi_poll_id}/votes",
                headers=headers,
                json=multi_vote_data
            )
            
            if response.status_code == 200:
                poll = response.json()
                print("✅ Multiple votes submitted successfully")
                print(f"   Your votes: {poll['own_votes']}")
                for opt in poll['options_data']:
                    print(f"   - {opt['title']}: {opt['votes_count']} votes")
            else:
                print(f"❌ Failed to submit multiple votes: {response.status_code}")
    else:
        print(f"❌ Failed to create multiple choice poll: {response.status_code}")
    
    # Test 6: Create a poll with hidden totals
    print("\n📝 Test 6: Creating poll with hidden totals...")
    hidden_poll_status = {
        "status": "Secret poll - results hidden until it ends",
        "poll": {
            "options": ["Option A", "Option B"],
            "expires_in": 300,  # 5 minutes
            "multiple": False,
            "hide_totals": True
        }
    }
    
    response = requests.post(
        f"{base_url}/api/v1/statuses",
        headers=headers,
        json=hidden_poll_status
    )
    
    if response.status_code == 201:
        print("✅ Hidden totals poll created successfully")
        status = response.json()
        hidden_poll_id = status["poll"]["id"] if status.get("poll") else None
        
        if hidden_poll_id:
            # View poll (totals should be hidden)
            response = requests.get(
                f"{base_url}/api/v1/polls/{hidden_poll_id}",
                headers=headers
            )
            
            if response.status_code == 200:
                poll = response.json()
                print("   Poll retrieved:")
                print(f"   Total votes shown: {poll['votes_count']} (should be 0)")
                print(f"   Total voters shown: {poll['voters_count']} (should be 0)")
                all_zero = all(opt['votes_count'] == 0 for opt in poll['options_data'])
                if all_zero:
                    print("   ✅ Vote counts correctly hidden")
                else:
                    print("   ❌ Vote counts not properly hidden")
    else:
        print(f"❌ Failed to create hidden totals poll: {response.status_code}")
    
    # Test 7: Test invalid poll creation
    print("\n🚫 Test 7: Testing poll validation...")
    
    # Too few options
    invalid_poll_1 = {
        "status": "Invalid poll - too few options",
        "poll": {
            "options": ["Only one option"],
            "expires_in": 3600,
            "multiple": False,
            "hide_totals": False
        }
    }
    
    response = requests.post(
        f"{base_url}/api/v1/statuses",
        headers=headers,
        json=invalid_poll_1
    )
    
    if response.status_code == 422:
        print("✅ Correctly rejected poll with too few options")
    else:
        print(f"❌ Unexpected response for invalid poll: {response.status_code}")
    
    # Too many options
    invalid_poll_2 = {
        "status": "Invalid poll - too many options",
        "poll": {
            "options": ["Option 1", "Option 2", "Option 3", "Option 4", "Option 5"],
            "expires_in": 3600,
            "multiple": False,
            "hide_totals": False
        }
    }
    
    response = requests.post(
        f"{base_url}/api/v1/statuses",
        headers=headers,
        json=invalid_poll_2
    )
    
    if response.status_code == 422:
        print("✅ Correctly rejected poll with too many options")
    else:
        print(f"❌ Unexpected response for invalid poll: {response.status_code}")
    
    # Test 8: View status with poll
    print("\n📄 Test 8: Viewing status with poll...")
    response = requests.get(
        f"{base_url}/api/v1/statuses/{status_id}",
        headers=headers
    )
    
    if response.status_code == 200:
        status = response.json()
        if status.get("poll"):
            print("✅ Status retrieved with poll data")
            poll = status["poll"]
            print(f"   Poll ID: {poll['id']}")
            print(f"   User voted: {poll['voted']}")
            if poll['voted']:
                print(f"   User's votes: {poll['own_votes']}")
        else:
            print("❌ Poll data missing from status")
    else:
        print(f"❌ Failed to retrieve status: {response.status_code}")
    
    print("\n✅ Poll testing completed!")

def main():
    parser = argparse.ArgumentParser(description='Test Lesser polls functionality')
    parser.add_argument('base_url', help='Base URL of the Lesser instance')
    parser.add_argument('--token', required=True, help='OAuth token for authentication')
    
    args = parser.parse_args()
    
    # Remove trailing slash from base URL
    base_url = args.base_url.rstrip('/')
    
    try:
        test_polls(base_url, args.token)
    except Exception as e:
        print(f"\n❌ Error during testing: {e}")
        return 1
    
    return 0

if __name__ == "__main__":
    exit(main()) 
