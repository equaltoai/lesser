#!/usr/bin/env python3
"""
Test script for Lesser's Portable Reputation API

This script demonstrates:
1. Getting reputation for an actor
2. Creating vouches
3. Exporting portable reputation
4. Importing reputation from another instance
5. Verifying reputation documents
6. Revoking vouches
"""

import requests
import json
import sys
import time

# Configuration
BASE_URL = "https://lab.lesser.social"
API_BASE = f"{BASE_URL}/api/v1"

# Test credentials (you'll need valid tokens)
TOKEN_USER1 = "your_token_here"  # User with high reputation
TOKEN_USER2 = "your_token_here"  # User to vouch for
TOKEN_USER3 = "your_token_here"  # User importing reputation

# Actor IDs
ACTOR1 = "https://lab.lesser.social/users/testuser1"
ACTOR2 = "https://lab.lesser.social/users/testuser2"
ACTOR3 = "https://lab.lesser.social/users/testuser3"


def print_section(title):
    """Print a section header"""
    print(f"\n{'=' * 60}")
    print(f"  {title}")
    print('=' * 60)


def make_request(method, url, token=None, data=None):
    """Make an API request and return the response"""
    headers = {
        "Content-Type": "application/json",
        "User-Agent": "LesserReputationTest/1.0"
    }
    
    if token:
        headers["Authorization"] = f"Bearer {token}"
    
    try:
        if method == "GET":
            response = requests.get(url, headers=headers)
        elif method == "POST":
            response = requests.post(url, headers=headers, json=data)
        elif method == "DELETE":
            response = requests.delete(url, headers=headers)
        else:
            raise ValueError(f"Unsupported method: {method}")
        
        return response
    except Exception as e:
        print(f"❌ Request failed: {e}")
        return None


def test_get_reputation():
    """Test getting reputation for an actor"""
    print_section("1. Getting Actor Reputation")
    
    url = f"{API_BASE}/reputation/{ACTOR1}"
    response = make_request("GET", url, TOKEN_USER1)
    
    if response and response.status_code == 200:
        rep = response.json()
        print(f"✅ Got reputation for {rep['id']}")
        print(f"   Total Score: {rep['total_score']}/1000")
        print(f"   - Trust Score: {rep['trust_score']}/250")
        print(f"   - Activity Score: {rep['activity_score']}/250")
        print(f"   - Moderation Score: {rep['moderation_score']}/250")
        print(f"   - Community Score: {rep['community_score']}/250")
        print(f"   Evidence:")
        print(f"   - Posts: {rep['evidence']['total_posts']}")
        print(f"   - Followers: {rep['evidence']['total_followers']}")
        print(f"   - Account Age: {rep['evidence']['account_age']} days")
        print(f"   - Vouches: {rep['evidence']['vouch_count']}")
        
        # Check cost headers
        if 'X-Cost-Total-Micros' in response.headers:
            cost = int(response.headers['X-Cost-Total-Micros']) / 1000000
            print(f"   💰 Operation cost: ${cost:.6f}")
        
        return rep
    else:
        print(f"❌ Failed to get reputation: {response.status_code if response else 'No response'}")
        if response:
            print(f"   Error: {response.text}")
        return None


def test_create_vouch():
    """Test creating a vouch"""
    print_section("2. Creating a Vouch")
    
    vouch_data = {
        "to": ACTOR2,
        "confidence": 0.8,
        "context": "Known this user for years, very trustworthy and positive contributor"
    }
    
    url = f"{API_BASE}/vouches"
    response = make_request("POST", url, TOKEN_USER1, vouch_data)
    
    if response and response.status_code == 201:
        vouch = response.json()
        print(f"✅ Created vouch {vouch['id']}")
        print(f"   From: {vouch['from']}")
        print(f"   To: {vouch['to']}")
        print(f"   Confidence: {vouch['confidence']}")
        print(f"   Context: {vouch['context']}")
        print(f"   Voucher Reputation: {vouch['voucher_reputation']}")
        print(f"   Expires: {vouch['expires_at']}")
        return vouch
    else:
        print(f"❌ Failed to create vouch: {response.status_code if response else 'No response'}")
        if response:
            print(f"   Error: {response.text}")
        return None


def test_get_vouches():
    """Test getting vouches for an actor"""
    print_section("3. Getting Vouches")
    
    url = f"{API_BASE}/vouches/{ACTOR2}"
    response = make_request("GET", url, TOKEN_USER1)
    
    if response and response.status_code == 200:
        vouches = response.json()
        print(f"✅ Got {len(vouches)} vouches for actor")
        for vouch in vouches[:3]:  # Show first 3
            print(f"   - From: {vouch['from']}")
            print(f"     Confidence: {vouch['confidence']}")
            print(f"     Created: {vouch['created_at']}")
            print(f"     Active: {vouch['active']}")
        return vouches
    else:
        print(f"❌ Failed to get vouches: {response.status_code if response else 'No response'}")
        return None


def test_export_reputation():
    """Test exporting portable reputation"""
    print_section("4. Exporting Portable Reputation")
    
    url = f"{API_BASE}/reputation/export"
    response = make_request("POST", url, TOKEN_USER1)
    
    if response and response.status_code == 200:
        portable_rep = response.json()
        print(f"✅ Exported portable reputation document")
        print(f"   Actor: {portable_rep['actor']}")
        print(f"   Issuer: {portable_rep['issuer']}")
        print(f"   Issued At: {portable_rep['issuedAt']}")
        print(f"   Expires At: {portable_rep['expiresAt']}")
        print(f"   Total Score: {portable_rep['reputation']['totalScore']}")
        print(f"   Vouches Included: {len(portable_rep['vouches'])}")
        
        # Pretty print a sample
        print("\n   Document Preview:")
        preview = {
            "@context": portable_rep.get("@context", []),
            "@type": portable_rep.get("@type"),
            "actor": portable_rep.get("actor"),
            "reputation": {
                "totalScore": portable_rep["reputation"]["totalScore"],
                "calculatedAt": portable_rep["reputation"]["calculatedAt"]
            }
        }
        print(json.dumps(preview, indent=4))
        
        return portable_rep
    else:
        print(f"❌ Failed to export reputation: {response.status_code if response else 'No response'}")
        return None


def test_verify_reputation(document):
    """Test verifying a reputation document"""
    print_section("5. Verifying Reputation Document")
    
    url = f"{API_BASE}/reputation/verify"
    response = make_request("POST", url, None, {"document": json.dumps(document)})
    
    if response and response.status_code == 200:
        result = response.json()
        print(f"✅ Verification completed")
        print(f"   Valid: {result['valid']}")
        print(f"   Actor: {result['actorId']}")
        print(f"   Issuer: {result['issuer']}")
        print(f"   Signature Valid: {result['signatureValid']}")
        print(f"   Not Expired: {result['notExpired']}")
        print(f"   Issuer Trusted: {result['issuerTrusted']}")
        if result.get('error'):
            print(f"   Error: {result['error']}")
        return result
    else:
        print(f"❌ Failed to verify reputation: {response.status_code if response else 'No response'}")
        return None


def test_import_reputation(document):
    """Test importing reputation from another instance"""
    print_section("6. Importing Reputation")
    
    url = f"{API_BASE}/reputation/import"
    response = make_request("POST", url, TOKEN_USER3, {"document": json.dumps(document)})
    
    if response and response.status_code == 200:
        result = response.json()
        print(f"✅ Import {'successful' if result['success'] else 'failed'}")
        print(f"   Actor: {result['actorId']}")
        print(f"   Previous Score: {result['previousScore']}")
        print(f"   Imported Score: {result['importedScore']}")
        print(f"   Vouches Imported: {result['vouchesImported']}")
        if result.get('message'):
            print(f"   Message: {result['message']}")
        if result.get('error'):
            print(f"   Error: {result['error']}")
        return result
    else:
        print(f"❌ Failed to import reputation: {response.status_code if response else 'No response'}")
        return None


def test_revoke_vouch(vouch_id):
    """Test revoking a vouch"""
    print_section("7. Revoking a Vouch")
    
    url = f"{API_BASE}/vouches/{vouch_id}"
    response = make_request("DELETE", url, TOKEN_USER1)
    
    if response and response.status_code == 204:
        print(f"✅ Successfully revoked vouch {vouch_id}")
        return True
    else:
        print(f"❌ Failed to revoke vouch: {response.status_code if response else 'No response'}")
        return False


def test_reputation_keys():
    """Test getting reputation keys from well-known endpoint"""
    print_section("8. Getting Reputation Keys")
    
    url = f"{BASE_URL}/.well-known/reputation-keys"
    response = make_request("GET", url)
    
    if response and response.status_code == 200:
        keys = response.json()
        print(f"✅ Got reputation keys")
        print(f"   Public Key: {keys.get('publicKey', 'N/A')[:50]}...")
        print(f"   Algorithm: {keys.get('algorithm', 'N/A')}")
        print(f"   Key ID: {keys.get('keyId', 'N/A')}")
        print(f"   Created: {keys.get('created', 'N/A')}")
        return keys
    else:
        print(f"❌ Failed to get reputation keys: {response.status_code if response else 'No response'}")
        return None


def run_all_tests():
    """Run all reputation API tests"""
    print("\n🚀 Lesser Portable Reputation API Test Suite")
    print("=" * 60)
    
    # Test 1: Get reputation
    reputation = test_get_reputation()
    if reputation:
        print(f"   Summary: Total {reputation.get('total_score', 'n/a')} / 1000")
    
    # Test 2: Create vouch
    vouch = test_create_vouch()
    
    # Test 3: Get vouches
    vouches = test_get_vouches()
    if vouches is not None:
        print(f"   Retrieved {len(vouches)} vouches for actor {ACTOR2}")
    
    # Test 4: Export reputation
    portable_rep = test_export_reputation()
    
    # Test 5: Verify reputation (if we have a document)
    if portable_rep:
        verification = test_verify_reputation(portable_rep)
        if verification and not verification.get("valid", False):
            print("⚠️  Verification reported invalid reputation document")
    
    # Test 6: Import reputation (would need a document from another instance)
    # Skipping actual import as it would need a real document from another instance
    print_section("6. Importing Reputation")
    print("⏭️  Skipping import test (requires document from another instance)")
    
    # Test 7: Revoke vouch (if we created one)
    if vouch:
        time.sleep(2)  # Wait a bit before revoking
        revoked = test_revoke_vouch(vouch['id'])
        if not revoked:
            print("⚠️  Vouch revocation failed")
    
    # Test 8: Get reputation keys
    keys = test_reputation_keys()
    if keys:
        print(f"   Active key ID: {keys.get('keyId', 'unknown')}")
    
    print("\n" + "=" * 60)
    print("✅ Test suite completed!")
    print("=" * 60)


def main():
    """Main function"""
    if len(sys.argv) > 1:
        # Run specific test
        test_name = sys.argv[1]
        if test_name == "reputation":
            test_get_reputation()
        elif test_name == "vouch":
            test_create_vouch()
        elif test_name == "export":
            test_export_reputation()
        elif test_name == "keys":
            test_reputation_keys()
        else:
            print(f"Unknown test: {test_name}")
            print("Available tests: reputation, vouch, export, keys")
    else:
        # Run all tests
        run_all_tests()


if __name__ == "__main__":
    main() 
