#!/usr/bin/env python3
"""
Interactive test script for lesser API using Mastodon.py
Requires manual OAuth authorization for comprehensive testing
First install: pip install Mastodon.py
"""

from mastodon import Mastodon
import time
import requests
from datetime import datetime

# Your lesser instance
INSTANCE_URL = 'https://lesser.host'

# Global counters for test results
tests_passed = 0
tests_failed = 0

def log_test(success, test_name, details=""):
    """Log test result with consistent formatting"""
    global tests_passed, tests_failed
    
    if success:
        tests_passed += 1
        print(f"✓ {test_name}")
        if details:
            print(f"  {details}")
    else:
        tests_failed += 1
        print(f"✗ {test_name}")
        if details:
            print(f"  ERROR: {details}")

def test_instance_info():
    """Test instance information endpoints"""
    print("\n=== Testing Instance Info ===\n")
    
    # Create anonymous client
    mastodon = Mastodon(api_base_url=INSTANCE_URL)
    
    try:
        # Test v1 instance endpoint
        instance = mastodon.instance()
        log_test(True, "GET /api/v1/instance", 
                f"URI: {instance['uri']}, Version: {instance['version']}")
    except Exception as e:
        log_test(False, "GET /api/v1/instance", str(e))
        
    try:
        # Test v2 instance endpoint  
        response = requests.get(f"{INSTANCE_URL}/api/v2/instance")
        if response.status_code == 200:
            instance_v2 = response.json()
            log_test(True, "GET /api/v2/instance", 
                    f"Domain: {instance_v2.get('domain', 'N/A')}")
        else:
            log_test(False, "GET /api/v2/instance", f"Status {response.status_code}")
    except Exception as e:
        log_test(False, "GET /api/v2/instance", str(e))
    
    # Test additional instance endpoints
    endpoints = [
        ('/api/v1/instance/peers', 'Instance peers'),
        ('/api/v1/instance/activity', 'Instance activity'),
        ('/api/v1/instance/rules', 'Instance rules'),
        ('/api/v1/instance/extended_description', 'Extended description'),
        ('/api/v1/custom_emojis', 'Custom emojis'),
    ]
    
    for endpoint, description in endpoints:
        try:
            response = requests.get(f"{INSTANCE_URL}{endpoint}")
            if response.status_code == 200:
                data = response.json()
                if isinstance(data, list):
                    log_test(True, f"GET {endpoint}", f"{description}: {len(data)} items")
                else:
                    log_test(True, f"GET {endpoint}", f"{description}: OK")
            else:
                log_test(False, f"GET {endpoint}", f"Status {response.status_code}")
        except Exception as e:
            log_test(False, f"GET {endpoint}", str(e))

def test_app_registration():
    """Test OAuth app registration"""
    print("\n\n=== Testing App Registration ===\n")
    
    try:
        # Register app
        client_id, client_secret = Mastodon.create_app(
            'lesser-test-manual',
            api_base_url=INSTANCE_URL,
            scopes=['read', 'write', 'follow', 'push'],
            website='https://github.com/equaltoai/lesser'
        )
        log_test(True, "POST /api/v1/apps", 
                f"Client ID: {client_id[:20]}...")
        return client_id, client_secret
    except Exception as e:
        log_test(False, "POST /api/v1/apps", str(e))
        return None, None

def test_oauth_flow(client_id, client_secret):
    """Test OAuth authentication flow"""
    print("\n\n=== Testing OAuth Flow ===\n")
    
    try:
        # Create client
        mastodon = Mastodon(
            client_id=client_id,
            client_secret=client_secret,
            api_base_url=INSTANCE_URL
        )
        
        # Get auth URL
        auth_url = mastodon.auth_request_url(
            scopes=['read', 'write', 'follow', 'push']
        )
        print(f"\n🔗 Please visit this URL to authorize the app:")
        print(f"   {auth_url}\n")
        print("After authorizing, paste the authorization code here:")
        
        # Get code from user
        auth_code = input("Authorization code: ").strip()
        
        if not auth_code:
            log_test(False, "OAuth authorization", "No code provided")
            return None
        
        # Log in with code
        mastodon.log_in(
            code=auth_code,
            scopes=['read', 'write', 'follow', 'push']
        )
        log_test(True, "POST /oauth/token", "Access token obtained")
        
        return mastodon
        
    except Exception as e:
        log_test(False, "OAuth flow", str(e))
        return None

def test_authenticated_endpoints(mastodon):
    """Test endpoints that require authentication"""
    print("\n\n=== Testing Authenticated Endpoints ===\n")
    
    # Test verify credentials
    try:
        account = mastodon.account_verify_credentials()
        log_test(True, "GET /api/v1/accounts/verify_credentials", 
                f"@{account['username']} (ID: {account['id']})")
        account_id = account['id']
    except Exception as e:
        log_test(False, "GET /api/v1/accounts/verify_credentials", str(e))
        return None
        
    # Test timelines
    try:
        home_tl = mastodon.timeline_home(limit=5)
        log_test(True, "GET /api/v1/timelines/home", f"Statuses: {len(home_tl)}")
    except Exception as e:
        log_test(False, "GET /api/v1/timelines/home", str(e))
        
    try:
        public_tl = mastodon.timeline_public(limit=5)
        log_test(True, "GET /api/v1/timelines/public", f"Statuses: {len(public_tl)}")
    except Exception as e:
        log_test(False, "GET /api/v1/timelines/public", str(e))
        
    try:
        local_tl = mastodon.timeline_local(limit=5)
        log_test(True, "GET /api/v1/timelines/public?local=true", f"Statuses: {len(local_tl)}")
    except Exception as e:
        log_test(False, "GET /api/v1/timelines/public?local=true", str(e))
        
    # Test notifications
    try:
        notifications = mastodon.notifications(limit=5)
        log_test(True, "GET /api/v1/notifications", f"Notifications: {len(notifications)}")
    except Exception as e:
        log_test(False, "GET /api/v1/notifications", str(e))
        
    # Test favorites
    try:
        favorites = mastodon.favourites(limit=5)
        log_test(True, "GET /api/v1/favourites", f"Favorites: {len(favorites)}")
    except Exception as e:
        log_test(False, "GET /api/v1/favourites", str(e))
        
    # Test bookmarks
    try:
        bookmarks = mastodon.bookmarks(limit=5)
        log_test(True, "GET /api/v1/bookmarks", f"Bookmarks: {len(bookmarks)}")
    except Exception as e:
        log_test(False, "GET /api/v1/bookmarks", str(e))
        
    # Test lists
    try:
        lists = mastodon.lists()
        log_test(True, "GET /api/v1/lists", f"Lists: {len(lists)}")
    except Exception as e:
        log_test(False, "GET /api/v1/lists", str(e))
        
    # Test filters
    try:
        filters = mastodon.filters()
        log_test(True, "GET /api/v1/filters", f"Filters: {len(filters)}")
    except Exception as e:
        log_test(False, "GET /api/v1/filters", str(e))
        
    # Test preferences
    try:
        prefs = mastodon.preferences()
        log_test(True, "GET /api/v1/preferences", 
                f"Language: {prefs.get('posting:default:language', 'N/A')}")
    except Exception as e:
        log_test(False, "GET /api/v1/preferences", str(e))
        
    # Test conversations
    try:
        conversations = mastodon.conversations(limit=5)
        log_test(True, "GET /api/v1/conversations", f"Conversations: {len(conversations)}")
    except Exception as e:
        log_test(False, "GET /api/v1/conversations", str(e))
        
    # Test mutes
    try:
        mutes = mastodon.mutes(limit=5)
        log_test(True, "GET /api/v1/mutes", f"Muted accounts: {len(mutes)}")
    except Exception as e:
        log_test(False, "GET /api/v1/mutes", str(e))
        
    # Test blocks
    try:
        blocks = mastodon.blocks(limit=5)
        log_test(True, "GET /api/v1/blocks", f"Blocked accounts: {len(blocks)}")
    except Exception as e:
        log_test(False, "GET /api/v1/blocks", str(e))
        
    # Test follow requests
    try:
        follow_requests = mastodon.follow_requests(limit=5)
        log_test(True, "GET /api/v1/follow_requests", f"Requests: {len(follow_requests)}")
    except Exception as e:
        log_test(False, "GET /api/v1/follow_requests", str(e))
        
    return account_id

def test_search(mastodon):
    """Test search functionality"""
    print("\n\n=== Testing Search ===\n")
    
    try:
        # V2 search
        results = mastodon.search_v2('test', limit=5)
        log_test(True, "GET /api/v2/search", 
                f"Accounts: {len(results.get('accounts', []))}, "
                f"Statuses: {len(results.get('statuses', []))}, "
                f"Hashtags: {len(results.get('hashtags', []))}")
    except Exception as e:
        log_test(False, "GET /api/v2/search", str(e))
    
    try:
        # Search with resolve
        mastodon.search_v2('test', resolve=True, limit=5)
        log_test(True, "GET /api/v2/search?resolve=true", "Search with resolve")
    except Exception as e:
        log_test(False, "GET /api/v2/search?resolve=true", str(e))

def test_trends(mastodon):
    """Test trends endpoints"""
    print("\n\n=== Testing Trends ===\n")
    
    try:
        tags = mastodon.trending_tags()
        log_test(True, "GET /api/v1/trends/tags", f"Trending tags: {len(tags)}")
    except Exception as e:
        log_test(False, "GET /api/v1/trends/tags", str(e))
        
    try:
        statuses = mastodon.trending_statuses()
        log_test(True, "GET /api/v1/trends/statuses", f"Trending statuses: {len(statuses)}")
    except Exception as e:
        log_test(False, "GET /api/v1/trends/statuses", str(e))
        
    try:
        links = mastodon.trending_links()
        log_test(True, "GET /api/v1/trends/links", f"Trending links: {len(links)}")
    except Exception as e:
        log_test(False, "GET /api/v1/trends/links", str(e))

def test_timeline_fanout(mastodon):
    """Test timeline fan-out functionality with user interaction"""
    print("\n\n=== Testing Timeline Fan-out ===\n")
    
    print("\n📝 This test will create a test post and verify it appears in various timelines.")
    proceed = input("Do you want to proceed? (y/n): ").strip().lower()
    
    if proceed != 'y':
        print("Skipping timeline fan-out test")
        return
    
    try:
        # Create a unique test status
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        test_content = f"Manual test post {timestamp} #lessertest #fanouttest"
        
        # Post a status
        status = mastodon.status_post(test_content, visibility='public')
        log_test(True, "POST /api/v1/statuses", 
                f"Created status ID: {status['id']}")
        print(f"  Content: {test_content}")
        
        # Wait for processing
        print("\n⏳ Waiting 3 seconds for timeline processing...")
        time.sleep(3)
        
        # Check home timeline
        home_tl = mastodon.timeline_home(limit=10)
        found_in_home = any(s['id'] == status['id'] for s in home_tl)
        log_test(found_in_home, "Status appears in home timeline", 
                "Fan-out working" if found_in_home else "Not found")
        
        # Check public timeline
        public_tl = mastodon.timeline_public(limit=10)
        found_in_public = any(s['id'] == status['id'] for s in public_tl)
        log_test(found_in_public, "Status appears in public timeline",
                "Public visibility working" if found_in_public else "Not found")
        
        # Check local timeline
        local_tl = mastodon.timeline_local(limit=10)
        found_in_local = any(s['id'] == status['id'] for s in local_tl)
        log_test(found_in_local, "Status appears in local timeline",
                "Local timeline working" if found_in_local else "Not found")
        
        # Check hashtag timelines
        for tag in ['lessertest', 'fanouttest']:
            try:
                hashtag_tl = mastodon.timeline_hashtag(tag, limit=10)
                found_in_hashtag = any(s['id'] == status['id'] for s in hashtag_tl)
                log_test(found_in_hashtag, f"Status appears in #{tag} timeline",
                        "Hashtag extraction working" if found_in_hashtag else "Not indexed")
            except Exception as e:
                log_test(False, f"Hashtag timeline #{tag}", str(e))
        
        # Test interactions
        print("\n🔄 Testing status interactions...")
        
        try:
            # Favorite
            mastodon.status_favourite(status['id'])
            log_test(True, "POST /api/v1/statuses/:id/favourite", "Favorited")
            time.sleep(1)
            
            # Check if favorited
            status_check = mastodon.status(status['id'])
            log_test(status_check['favourited'], "Status shows as favorited", 
                    f"Favorites count: {status_check['favourites_count']}")
            
            # Unfavorite
            mastodon.status_unfavourite(status['id'])
            log_test(True, "POST /api/v1/statuses/:id/unfavourite", "Unfavorited")
            
            # Boost
            boost = mastodon.status_reblog(status['id'])
            log_test(True, "POST /api/v1/statuses/:id/reblog", 
                    f"Boosted as ID: {boost['id']}")
            time.sleep(1)
            
            # Unboost
            mastodon.status_unreblog(status['id'])
            log_test(True, "POST /api/v1/statuses/:id/unreblog", "Unboosted")
            
            # Bookmark
            mastodon.status_bookmark(status['id'])
            log_test(True, "POST /api/v1/statuses/:id/bookmark", "Bookmarked")
            
            # Check bookmarks
            bookmarks = mastodon.bookmarks(limit=5)
            found_bookmark = any(b['id'] == status['id'] for b in bookmarks)
            log_test(found_bookmark, "Status appears in bookmarks", 
                    "Bookmark system working" if found_bookmark else "Not found")
            
            # Unbookmark
            mastodon.status_unbookmark(status['id'])
            log_test(True, "POST /api/v1/statuses/:id/unbookmark", "Unbookmarked")
            
        except Exception as e:
            log_test(False, "Status interactions", str(e))
        
        # Ask before deleting
        print(f"\n🗑️  Ready to delete test status (ID: {status['id']})")
        delete = input("Delete the test status? (y/n): ").strip().lower()
        
        if delete == 'y':
            try:
                mastodon.status_delete(status['id'])
                log_test(True, "DELETE /api/v1/statuses/:id", "Test status deleted")
            except Exception as e:
                log_test(False, "DELETE /api/v1/statuses/:id", str(e))
        else:
            print(f"ℹ️  Test status kept: {status['url']}")
            
    except Exception as e:
        log_test(False, "Timeline fan-out test", str(e))

def test_account_operations(mastodon):
    """Test account-related operations"""
    print("\n\n=== Testing Account Operations ===\n")
    
    print("\n👤 This test will look up and potentially follow/unfollow accounts.")
    proceed = input("Do you want to proceed? (y/n): ").strip().lower()
    
    if proceed != 'y':
        print("Skipping account operations test")
        return
    
    # Ask for a test account
    print("\nEnter a username to test with (e.g., 'admin' or 'admin@example.com'):")
    test_account = input("Username: ").strip()
    
    if not test_account:
        print("No username provided, skipping test")
        return
    
    try:
        # Lookup account
        results = mastodon.account_search(test_account, limit=1)
        if not results:
            log_test(False, "Account lookup", f"No account found for '{test_account}'")
            return
            
        account = results[0]
        account_id = account['id']
        log_test(True, "GET /api/v1/accounts/search", 
                f"Found @{account['acct']} (ID: {account_id})")
        
        # Get account details
        account_detail = mastodon.account(account_id)
        log_test(True, f"GET /api/v1/accounts/{account_id}", 
                f"Followers: {account_detail['followers_count']}, "
                f"Following: {account_detail['following_count']}")
        
        # Get account statuses
        statuses = mastodon.account_statuses(account_id, limit=5)
        log_test(True, f"GET /api/v1/accounts/{account_id}/statuses", 
                f"Statuses: {len(statuses)}")
        
        # Test relationships
        relationships = mastodon.account_relationships([account_id])
        if relationships:
            rel = relationships[0]
            log_test(True, "GET /api/v1/accounts/relationships", 
                    f"Following: {rel['following']}, "
                    f"Followed by: {rel['followed_by']}")
            
            # Test follow/unfollow if not already following
            if not rel['following']:
                print(f"\n🤝 Would you like to follow @{account['acct']}? (y/n): ", end='')
                if input().strip().lower() == 'y':
                    mastodon.account_follow(account_id)
                    log_test(True, f"POST /api/v1/accounts/{account_id}/follow", 
                            "Follow request sent")
                    
                    # Unfollow
                    time.sleep(2)
                    mastodon.account_unfollow(account_id)
                    log_test(True, f"POST /api/v1/accounts/{account_id}/unfollow", 
                            "Unfollowed")
                    
    except Exception as e:
        log_test(False, "Account operations", str(e))

def main():
    """Run all tests"""
    global tests_passed, tests_failed
    
    print(f"🧪 Interactive lesser API Test")
    print(f"   Instance: {INSTANCE_URL}\n")
    print("=" * 60)
    
    # Test public endpoints
    test_instance_info()
    
    # Test OAuth flow
    client_id, client_secret = test_app_registration()
    if not client_id:
        print("\n❌ App registration failed, cannot continue")
        return
        
    # Get authenticated client
    mastodon = test_oauth_flow(client_id, client_secret)
    if not mastodon:
        print("\n❌ OAuth flow failed, cannot continue")
        return
        
    # Test authenticated endpoints
    test_authenticated_endpoints(mastodon)
    test_search(mastodon)
    test_trends(mastodon)
    
    # Interactive tests
    test_timeline_fanout(mastodon)
    test_account_operations(mastodon)
    
    # Summary
    print("\n" + "=" * 60)
    print(f"\n📊 Test Summary:")
    print(f"   ✓ Passed: {tests_passed}")
    print(f"   ✗ Failed: {tests_failed}")
    print(f"   Total: {tests_passed + tests_failed}")
    
    if tests_failed == 0:
        print("\n🎉 All tests passed!")
    else:
        print(f"\n⚠️  {tests_failed} tests failed")

if __name__ == "__main__":
    main() 
