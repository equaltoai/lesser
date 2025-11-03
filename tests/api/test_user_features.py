#!/usr/bin/env python3
import requests
import sys

BASE_URL = "https://5r2w65d6qhymglxbtwlqauvqda0mtwem.lambda-url.us-east-1.on.aws/api/v1"

def test_endorsements(token):
    """Test account endorsements (pinned accounts)."""
    headers = {"Authorization": f"Bearer {token}"}
    
    print("\n=== Testing Endorsements ===")
    
    # Get current endorsements
    print("\n1. Getting current endorsements...")
    response = requests.get(f"{BASE_URL}/endorsements", headers=headers)
    
    if response.status_code == 200:
        endorsements = response.json()
        print(f"✓ Got {len(endorsements)} endorsements")
        for account in endorsements:
            print(f"  - @{account['username']} ({account['display_name']})")
    else:
        print(f"✗ Failed to get endorsements: {response.status_code}")
        print(f"  Response: {response.text}")
        return False
    
    # Note: Endorsing/unendorsing is done via the pin/unpin endpoints
    # which were tested in test_account_features.py
    
    return True

def test_preferences(token):
    """Test user preferences."""
    headers = {"Authorization": f"Bearer {token}"}
    
    print("\n=== Testing Preferences ===")
    
    # Get current preferences
    print("\n1. Getting current preferences...")
    response = requests.get(f"{BASE_URL}/preferences", headers=headers)
    
    if response.status_code == 200:
        prefs = response.json()
        print("✓ Got preferences:")
        for key, value in prefs.items():
            print(f"  {key}: {value}")
    else:
        print(f"✗ Failed to get preferences: {response.status_code}")
        print(f"  Response: {response.text}")
        return False
    
    # Update preferences
    print("\n2. Updating preferences...")
    update_data = {
        "posting:default:visibility": "unlisted",
        "posting:default:sensitive": True,
        "posting:default:language": "es",
        "reading:expand:spoilers": True
    }
    
    response = requests.patch(
        f"{BASE_URL}/preferences",
        headers=headers,
        json=update_data
    )
    
    if response.status_code == 200:
        updated_prefs = response.json()
        print("✓ Updated preferences:")
        for key, value in updated_prefs.items():
            print(f"  {key}: {value}")
            
        # Verify the changes
        if (updated_prefs.get("posting:default:visibility") == "unlisted" and
            updated_prefs.get("posting:default:sensitive") == True and
            updated_prefs.get("posting:default:language") == "es" and
            updated_prefs.get("reading:expand:spoilers") == True):
            print("✓ All preference updates verified")
        else:
            print("✗ Some preferences didn't update correctly")
    else:
        print(f"✗ Failed to update preferences: {response.status_code}")
        print(f"  Response: {response.text}")
        return False
    
    # Reset preferences
    print("\n3. Resetting preferences to defaults...")
    reset_data = {
        "posting:default:visibility": "public",
        "posting:default:sensitive": False,
        "posting:default:language": "en",
        "reading:expand:spoilers": False
    }
    
    response = requests.patch(
        f"{BASE_URL}/preferences",
        headers=headers,
        json=reset_data
    )
    
    if response.status_code == 200:
        print("✓ Preferences reset to defaults")
    else:
        print(f"⚠ Failed to reset preferences: {response.status_code}")
    
    return True

def test_markers(token):
    """Test timeline position markers."""
    headers = {"Authorization": f"Bearer {token}"}
    
    print("\n=== Testing Markers ===")
    
    # Get current markers
    print("\n1. Getting current markers...")
    response = requests.get(f"{BASE_URL}/markers", headers=headers)
    
    if response.status_code == 200:
        markers = response.json()
        print("✓ Got markers:")
        if markers.get("home"):
            print(f"  Home timeline: {markers['home']['last_read_id']} (v{markers['home']['version']})")
        if markers.get("notifications"):
            print(f"  Notifications: {markers['notifications']['last_read_id']} (v{markers['notifications']['version']})")
        if not markers.get("home") and not markers.get("notifications"):
            print("  No markers saved yet")
    else:
        print(f"✗ Failed to get markers: {response.status_code}")
        print(f"  Response: {response.text}")
        return False
    
    # Save markers
    print("\n2. Saving new markers...")
    marker_data = {
        "home": {
            "last_read_id": "123456789"
        },
        "notifications": {
            "last_read_id": "987654321"
        }
    }
    
    response = requests.post(
        f"{BASE_URL}/markers",
        headers=headers,
        json=marker_data
    )
    
    if response.status_code == 200:
        saved_markers = response.json()
        print("✓ Saved markers:")
        if saved_markers.get("home"):
            print(f"  Home timeline: {saved_markers['home']['last_read_id']} (v{saved_markers['home']['version']})")
        if saved_markers.get("notifications"):
            print(f"  Notifications: {saved_markers['notifications']['last_read_id']} (v{saved_markers['notifications']['version']})")
    else:
        print(f"✗ Failed to save markers: {response.status_code}")
        print(f"  Response: {response.text}")
        return False
    
    # Update only one marker
    print("\n3. Updating only home timeline marker...")
    update_data = {
        "home": {
            "last_read_id": "555555555"
        }
    }
    
    response = requests.post(
        f"{BASE_URL}/markers",
        headers=headers,
        json=update_data
    )
    
    if response.status_code == 200:
        updated_markers = response.json()
        print("✓ Updated home marker:")
        if updated_markers.get("home"):
            print(f"  Home timeline: {updated_markers['home']['last_read_id']} (v{updated_markers['home']['version']})")
            # Version should have incremented
            if updated_markers['home']['version'] > 1:
                print("  ✓ Version incremented correctly")
    else:
        print(f"✗ Failed to update marker: {response.status_code}")
    
    # Get specific timeline markers
    print("\n4. Getting only home timeline marker...")
    response = requests.get(
        f"{BASE_URL}/markers",
        headers=headers,
        params={"timeline[]": "home"}
    )
    
    if response.status_code == 200:
        home_only = response.json()
        if home_only.get("home") and not home_only.get("notifications"):
            print("✓ Got only home timeline marker as requested")
        else:
            print("⚠ Response included unexpected markers")
    else:
        print(f"✗ Failed to get specific markers: {response.status_code}")
    
    return True

def main():
    """Main test runner."""
    if len(sys.argv) < 2:
        print("Usage: python test_phase5_user_features.py <access_token>")
        sys.exit(1)
    
    token = sys.argv[1]
    
    print("Testing Phase 5 User Features")
    print("=============================")
    
    all_passed = True
    
    # Test each feature
    if not test_endorsements(token):
        all_passed = False
        
    if not test_preferences(token):
        all_passed = False
        
    if not test_markers(token):
        all_passed = False
    
    print("\n=============================")
    if all_passed:
        print("✅ All Phase 5 tests passed!")
    else:
        print("❌ Some tests failed")
        sys.exit(1)

if __name__ == "__main__":
    main() 
