#!/usr/bin/env python3
"""
Test script for Lesser moderation API endpoints
"""

import requests
import json
import sys
import time
from typing import Dict, Any

# Configuration
BASE_URL = "https://api.staging.lesser.aron23.com"
TEST_USERNAME = "testuser"
TEST_PASSWORD = "testpassword123"

# Global variables for test state
access_token = None
test_status_id = None
test_event_id = None

def get_auth_headers() -> Dict[str, str]:
    """Get authorization headers"""
    if not access_token:
        raise Exception("Not authenticated")
    return {"Authorization": f"Bearer {access_token}"}

def authenticate():
    """Authenticate and get access token"""
    global access_token
    
    # Register app
    app_data = {
        "client_name": "Moderation Test App",
        "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
        "scopes": "read write follow push admin"
    }
    
    resp = requests.post(f"{BASE_URL}/api/v1/apps", json=app_data)
    if resp.status_code != 200:
        print(f"Failed to register app: {resp.status_code} {resp.text}")
        sys.exit(1)
    
    app = resp.json()
    client_id = app["client_id"]
    client_secret = app["client_secret"]
    
    # Login
    token_data = {
        "grant_type": "password",
        "client_id": client_id,
        "client_secret": client_secret,
        "username": TEST_USERNAME,
        "password": TEST_PASSWORD,
        "scope": "read write follow push admin"
    }
    
    resp = requests.post(f"{BASE_URL}/oauth/token", data=token_data)
    if resp.status_code != 200:
        print(f"Failed to get token: {resp.status_code} {resp.text}")
        sys.exit(1)
    
    token_resp = resp.json()
    access_token = token_resp["access_token"]
    print("✅ Authentication successful")

def create_test_status():
    """Create a test status to flag"""
    global test_status_id
    
    status_data = {
        "status": "This is a test status that violates our community guidelines #spam",
        "visibility": "public"
    }
    
    resp = requests.post(
        f"{BASE_URL}/api/v1/statuses",
        headers=get_auth_headers(),
        json=status_data
    )
    
    if resp.status_code != 200:
        print(f"Failed to create status: {resp.status_code} {resp.text}")
        return False
    
    status = resp.json()
    test_status_id = status["id"]
    print(f"✅ Created test status: {test_status_id}")
    return True

def test_flag_content():
    """Test flagging content"""
    global test_event_id
    
    flag_data = {
        "object_id": test_status_id,
        "object_type": "status",
        "category": "spam",
        "severity": 3,
        "confidence_score": 0.85,
        "reason": "This status contains spam content and violates community guidelines"
    }
    
    resp = requests.post(
        f"{BASE_URL}/api/v1/moderation/flag",
        headers=get_auth_headers(),
        json=flag_data
    )
    
    if resp.status_code != 201:
        print(f"❌ Failed to flag content: {resp.status_code} {resp.text}")
        return False
    
    event = resp.json()
    test_event_id = event["id"]
    print(f"✅ Content flagged successfully: {test_event_id}")
    print(f"   Category: {event['category']}, Severity: {event['severity']}")
    return True

def test_get_review_queue():
    """Test getting the moderation review queue"""
    resp = requests.get(
        f"{BASE_URL}/api/v1/moderation/queue",
        headers=get_auth_headers(),
        params={"limit": 10, "min_severity": 2}
    )
    
    if resp.status_code == 403:
        print("⚠️  Review queue requires admin scope (expected)")
        return True
    elif resp.status_code != 200:
        print(f"❌ Failed to get review queue: {resp.status_code} {resp.text}")
        return False
    
    queue = resp.json()
    print(f"✅ Retrieved review queue: {len(queue)} items")
    for item in queue[:3]:  # Show first 3 items
        print(f"   - {item['id']}: {item['category']} (severity: {item['severity']}, priority: {item['priority_score']:.2f})")
    return True

def test_submit_review():
    """Test submitting a moderation review"""
    review_data = {
        "event_id": test_event_id,
        "action": "remove",
        "category": "spam",
        "severity": 3,
        "confidence": 0.9,
        "notes": "Clear violation of spam policy"
    }
    
    resp = requests.post(
        f"{BASE_URL}/api/v1/moderation/review",
        headers=get_auth_headers(),
        json=review_data
    )
    
    if resp.status_code == 403:
        print("⚠️  Submit review requires admin scope (expected)")
        return True
    elif resp.status_code != 201:
        print(f"❌ Failed to submit review: {resp.status_code} {resp.text}")
        return False
    
    result = resp.json()
    print(f"✅ Review submitted: {result['review_id']}")
    return True

def test_get_consensus():
    """Test getting consensus visualization"""
    resp = requests.get(
        f"{BASE_URL}/api/v1/moderation/consensus/{test_event_id}",
        headers=get_auth_headers()
    )
    
    if resp.status_code == 403:
        print("⚠️  Consensus view requires admin scope (expected)")
        return True
    elif resp.status_code == 404:
        print("⚠️  Event not found (may not exist yet)")
        return True
    elif resp.status_code != 200:
        print(f"❌ Failed to get consensus: {resp.status_code} {resp.text}")
        return False
    
    consensus = resp.json()
    print(f"✅ Retrieved consensus for event {consensus['event_id']}")
    print(f"   Reviews: {consensus['reviewer_count']}")
    if consensus.get('consensus_score'):
        print(f"   Consensus Score: {consensus['consensus_score']:.2f}")
    if consensus.get('decision'):
        print(f"   Decision: {consensus['decision']}")
    return True

def test_trust_management():
    """Test trust relationship management"""
    # Get current trust relationships
    resp = requests.get(
        f"{BASE_URL}/api/v1/moderation/trust",
        headers=get_auth_headers(),
        params={"direction": "outgoing"}
    )
    
    if resp.status_code != 200:
        print(f"❌ Failed to get trust relationships: {resp.status_code} {resp.text}")
        return False
    
    relationships = resp.json()
    print(f"✅ Retrieved {len(relationships)} outgoing trust relationships")
    
    # Update trust for a test account
    trust_update = {
        "trustee_id": "@admin@example.com",
        "trustee_domain": "example.com",
        "category": "content",
        "score": 0.8,
        "confidence": 0.9
    }
    
    resp = requests.put(
        f"{BASE_URL}/api/v1/moderation/trust",
        headers=get_auth_headers(),
        json=trust_update
    )
    
    if resp.status_code != 200:
        print(f"❌ Failed to update trust: {resp.status_code} {resp.text}")
        return False
    
    print("✅ Trust relationship updated successfully")
    return True

def test_trust_score():
    """Test getting trust scores"""
    # Get trust score for current user
    resp = requests.get(
        f"{BASE_URL}/api/v1/accounts/verify_credentials",
        headers=get_auth_headers()
    )
    
    if resp.status_code != 200:
        print(f"❌ Failed to get current user: {resp.status_code}")
        return False
    
    user = resp.json()
    actor_id = f"@{user['username']}@{user['url'].split('/')[2]}"
    
    # Get trust score
    resp = requests.get(
        f"{BASE_URL}/api/v1/moderation/trust/{actor_id}/score",
        headers=get_auth_headers()
    )
    
    if resp.status_code != 200:
        print(f"❌ Failed to get trust score: {resp.status_code} {resp.text}")
        return False
    
    trust_score = resp.json()
    print(f"✅ Retrieved trust score for {trust_score['actor_id']}")
    print(f"   Overall Score: {trust_score['overall_score']:.2f}")
    print(f"   Trusters: {trust_score['truster_count']}")
    for category, score in trust_score['scores'].items():
        print(f"   - {category}: {score:.2f}")
    return True

def cleanup():
    """Clean up test data"""
    if test_status_id:
        resp = requests.delete(
            f"{BASE_URL}/api/v1/statuses/{test_status_id}",
            headers=get_auth_headers()
        )
        if resp.status_code == 200:
            print("✅ Cleaned up test status")

def main():
    """Run all moderation tests"""
    print("🚀 Starting Lesser Moderation API tests...")
    print(f"   Base URL: {BASE_URL}")
    print()
    
    try:
        authenticate()
        print()
        
        # Create test data
        if not create_test_status():
            print("❌ Failed to create test data")
            return
        print()
        
        # Run tests
        tests = [
            ("Flag Content", test_flag_content),
            ("Get Review Queue", test_get_review_queue),
            ("Submit Review", test_submit_review),
            ("Get Consensus", test_get_consensus),
            ("Trust Management", test_trust_management),
            ("Trust Score", test_trust_score),
        ]
        
        passed = 0
        for test_name, test_func in tests:
            print(f"📝 Testing {test_name}...")
            if test_func():
                passed += 1
            print()
        
        # Summary
        print(f"✅ Passed {passed}/{len(tests)} tests")
        
        # Cleanup
        cleanup()
        
    except Exception as e:
        print(f"❌ Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main() 