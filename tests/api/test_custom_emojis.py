#!/usr/bin/env python3
"""
Test script for custom emojis functionality in Lesser
"""

import requests
import json

# Configuration
BASE_URL = "http://localhost:8080"
ADMIN_TOKEN = None  # Will be set after login

def test_get_custom_emojis():
    """Test getting custom emojis (public endpoint)"""
    print("\n=== Testing GET /api/v1/custom_emojis ===")
    
    resp = requests.get(f"{BASE_URL}/api/v1/custom_emojis")
    print(f"Status: {resp.status_code}")
    print(f"Response: {json.dumps(resp.json(), indent=2)}")
    
    assert resp.status_code == 200
    assert isinstance(resp.json(), list)

def test_create_custom_emoji():
    """Test creating a custom emoji as admin"""
    print("\n=== Testing POST /api/v1/admin/custom_emojis ===")
    
    emoji_data = {
        "shortcode": "partyparrot",
        "url": "https://example.com/emojis/partyparrot.gif",
        "static_url": "https://example.com/emojis/partyparrot.png",
        "category": "animated"
    }
    
    headers = {"Authorization": f"Bearer {ADMIN_TOKEN}"}
    resp = requests.post(
        f"{BASE_URL}/api/v1/admin/custom_emojis",
        json=emoji_data,
        headers=headers
    )
    
    print(f"Status: {resp.status_code}")
    if resp.status_code == 200:
        emoji = resp.json()
        print(f"Created emoji: {json.dumps(emoji, indent=2)}")
        return emoji["shortcode"]
    else:
        print(f"Error: {resp.text}")
        return None

def test_update_custom_emoji(shortcode):
    """Test updating a custom emoji"""
    print(f"\n=== Testing PUT /api/v1/admin/custom_emojis/{shortcode} ===")
    
    update_data = {
        "category": "birds",
        "visible_in_picker": False
    }
    
    headers = {"Authorization": f"Bearer {ADMIN_TOKEN}"}
    resp = requests.put(
        f"{BASE_URL}/api/v1/admin/custom_emojis/{shortcode}",
        json=update_data,
        headers=headers
    )
    
    print(f"Status: {resp.status_code}")
    if resp.status_code == 200:
        print(f"Updated emoji: {json.dumps(resp.json(), indent=2)}")

def test_delete_custom_emoji(shortcode):
    """Test deleting a custom emoji"""
    print(f"\n=== Testing DELETE /api/v1/admin/custom_emojis/{shortcode} ===")
    
    headers = {"Authorization": f"Bearer {ADMIN_TOKEN}"}
    resp = requests.delete(
        f"{BASE_URL}/api/v1/admin/custom_emojis/{shortcode}",
        headers=headers
    )
    
    print(f"Status: {resp.status_code}")
    assert resp.status_code == 200

def test_emoji_categories():
    """Test multiple emojis with categories"""
    print("\n=== Testing emoji categories ===")
    
    # Create emojis in different categories
    emojis = [
        {
            "shortcode": "blobcat",
            "url": "https://example.com/emojis/blobcat.png",
            "category": "cats"
        },
        {
            "shortcode": "blobdog",
            "url": "https://example.com/emojis/blobdog.png",
            "category": "dogs"
        },
        {
            "shortcode": "blobneutral",
            "url": "https://example.com/emojis/blobneutral.png",
            "category": ""
        }
    ]
    
    headers = {"Authorization": f"Bearer {ADMIN_TOKEN}"}
    
    for emoji in emojis:
        resp = requests.post(
            f"{BASE_URL}/api/v1/admin/custom_emojis",
            json=emoji,
            headers=headers
        )
        print(f"Created {emoji['shortcode']}: {resp.status_code}")
    
    # List all emojis
    resp = requests.get(f"{BASE_URL}/api/v1/custom_emojis")
    all_emojis = resp.json()
    
    # Check categories
    categories = set()
    for emoji in all_emojis:
        if "category" in emoji and emoji["category"]:
            categories.add(emoji["category"])
    
    print(f"Found categories: {categories}")
    print(f"Total emojis: {len(all_emojis)}")

def test_emoji_errors():
    """Test error cases"""
    print("\n=== Testing error cases ===")
    
    # Try to create emoji without auth
    resp = requests.post(
        f"{BASE_URL}/api/v1/admin/custom_emojis",
        json={"shortcode": "test", "url": "test"}
    )
    print(f"Create without auth: {resp.status_code} (should be 401)")
    assert resp.status_code == 401
    
    # Try to create emoji with missing fields
    headers = {"Authorization": f"Bearer {ADMIN_TOKEN}"}
    resp = requests.post(
        f"{BASE_URL}/api/v1/admin/custom_emojis",
        json={"shortcode": "test"},
        headers=headers
    )
    print(f"Create with missing URL: {resp.status_code} (should be 422)")
    assert resp.status_code == 422
    
    # Try to update non-existent emoji
    resp = requests.put(
        f"{BASE_URL}/api/v1/admin/custom_emojis/nonexistent",
        json={"category": "test"},
        headers=headers
    )
    print(f"Update non-existent: {resp.status_code} (should be 404)")
    assert resp.status_code == 404

def test_emoji_in_content():
    """Test how emojis would be used in content"""
    print("\n=== Testing emoji usage patterns ===")
    
    # Example content with custom emojis
    content_examples = [
        "Hello :wave: world!",
        "I love :blobcat: so much!",
        "Party time :partyparrot::partyparrot::partyparrot:",
        "Multiple :blobcat: emojis :blobdog: in one :blobneutral: message"
    ]
    
    print("Example content with custom emojis:")
    for content in content_examples:
        print(f"  - {content}")
    
    print("\nNote: Content parsing and replacement is a TODO item")

def main():
    """Run all custom emoji tests"""
    global ADMIN_TOKEN
    
    print("=== Lesser Custom Emojis Test Suite ===")
    
    # For testing, you'd need proper OAuth tokens
    # ADMIN_TOKEN = "your-admin-token-here"
    
    # Test public endpoint
    test_get_custom_emojis()
    
    # Admin tests would require proper authentication
    # shortcode = test_create_custom_emoji()
    # if shortcode:
    #     test_update_custom_emoji(shortcode)
    #     test_delete_custom_emoji(shortcode)
    
    # test_emoji_categories()
    # test_emoji_errors()
    
    test_emoji_in_content()
    
    print("\n✓ Custom emoji tests completed!")
    print("\nNote: For full testing, you need:")
    print("1. A running Lesser instance")
    print("2. Admin user with proper role")
    print("3. OAuth tokens for authentication")
    print("4. DynamoDB table with proper configuration")

if __name__ == "__main__":
    main() 