#!/usr/bin/env python3
"""
Test script for Filters & Mutes functionality
"""

import requests
import time
from datetime import datetime

# Configuration
BASE_URL = "https://api.lesser.social"
USERNAME = "testuser"
PASSWORD = "testpass123"

# Colors for output
GREEN = '\033[92m'
RED = '\033[91m'
YELLOW = '\033[93m'
BLUE = '\033[94m'
RESET = '\033[0m'

def print_test(name):
    print(f"\n{BLUE}Testing: {name}{RESET}")

def print_success(message):
    print(f"{GREEN}✓ {message}{RESET}")

def print_error(message):
    print(f"{RED}✗ {message}{RESET}")

def print_info(message):
    print(f"{YELLOW}ℹ {message}{RESET}")

class FiltersMutesTest:
    def __init__(self):
        self.session = requests.Session()
        self.access_token = None
        self.test_account_id = None
        self.test_filter_id = None

    def register_and_login(self):
        """Register a test user and get access token"""
        print_test("User Registration and Login")
        
        # Register user
        register_data = {
            "username": USERNAME,
            "email": f"{USERNAME}@example.com",
            "password": PASSWORD,
            "agreement": True,
            "locale": "en"
        }
        
        try:
            resp = self.session.post(f"{BASE_URL}/api/v1/accounts", json=register_data)
            if resp.status_code == 201:
                print_success("User registered successfully")
            elif resp.status_code == 422 and "already taken" in resp.text:
                print_info("User already exists, proceeding with login")
            else:
                print_error(f"Registration failed: {resp.status_code} - {resp.text}")
                return False
        except Exception as e:
            print_error(f"Registration error: {e}")
            return False
        
        # Get OAuth token
        token_data = {
            "grant_type": "password",
            "username": USERNAME,
            "password": PASSWORD,
            "scope": "read write follow push"
        }
        
        try:
            resp = self.session.post(f"{BASE_URL}/oauth/token", data=token_data)
            if resp.status_code == 200:
                token_info = resp.json()
                self.access_token = token_info['access_token']
                self.session.headers.update({'Authorization': f'Bearer {self.access_token}'})
                print_success("Login successful")
                return True
            else:
                print_error(f"Login failed: {resp.status_code} - {resp.text}")
                return False
        except Exception as e:
            print_error(f"Login error: {e}")
            return False

    def test_mute_account(self):
        """Test muting an account"""
        print_test("Mute Account")
        
        # First, we need another account to mute
        # For testing, we'll try to mute a non-existent account
        test_account = "nonexistent"
        
        try:
            # Try to mute the account
            resp = self.session.post(f"{BASE_URL}/api/v1/accounts/{test_account}/mute", 
                                    json={"notifications": True})
            
            if resp.status_code == 404:
                print_info("Account not found (expected for non-existent account)")
                # Create a second test account
                self.create_second_account()
                return True
            elif resp.status_code == 200:
                relationship = resp.json()
                if relationship.get('muting'):
                    print_success(f"Successfully muted account {test_account}")
                    self.test_account_id = test_account
                    return True
                print_error(f"Unexpected response body when muting {test_account}")
                return False
            else:
                print_error(f"Mute failed: {resp.status_code} - {resp.text}")
                return False
        except Exception as e:
            print_error(f"Mute error: {e}")
            return False

    def create_second_account(self):
        """Create a second account for testing"""
        print_info("Creating second test account")
        
        # Save current token
        original_token = self.access_token
        
        # Register second user
        register_data = {
            "username": "testuser2",
            "email": "testuser2@example.com",
            "password": "testpass123",
            "agreement": True,
            "locale": "en"
        }
        
        try:
            # Remove auth header for registration
            del self.session.headers['Authorization']
            
            resp = self.session.post(f"{BASE_URL}/api/v1/accounts", json=register_data)
            if resp.status_code in [201, 422]:
                print_success("Second account ready")
                self.test_account_id = "testuser2"
                
                # Restore original token
                self.access_token = original_token
                self.session.headers.update({'Authorization': f'Bearer {self.access_token}'})
                
                # Now try to mute the second account
                resp = self.session.post(f"{BASE_URL}/api/v1/accounts/testuser2/mute", 
                                       json={"notifications": True})
                if resp.status_code == 200:
                    print_success("Successfully muted testuser2")
                    return True
                elif resp.status_code == 404:
                    print_info("Account lookup may require different format")
                    return True
        except Exception as e:
            print_error(f"Second account error: {e}")
        
        # Restore token
        self.access_token = original_token
        self.session.headers.update({'Authorization': f'Bearer {self.access_token}'})
        return False

    def test_get_muted_accounts(self):
        """Test getting list of muted accounts"""
        print_test("Get Muted Accounts")
        
        try:
            resp = self.session.get(f"{BASE_URL}/api/v1/mutes")
            if resp.status_code == 200:
                muted = resp.json()
                print_success(f"Retrieved {len(muted)} muted accounts")
                if len(muted) > 0:
                    print_info(f"First muted account: {muted[0].get('username', 'unknown')}")
                return True
            else:
                print_error(f"Get mutes failed: {resp.status_code} - {resp.text}")
                return False
        except Exception as e:
            print_error(f"Get mutes error: {e}")
            return False

    def test_unmute_account(self):
        """Test unmuting an account"""
        print_test("Unmute Account")
        
        if not self.test_account_id:
            print_info("No account to unmute, skipping")
            return True
        
        try:
            resp = self.session.post(f"{BASE_URL}/api/v1/accounts/{self.test_account_id}/unmute")
            if resp.status_code == 200:
                relationship = resp.json()
                if not relationship.get('muting'):
                    print_success(f"Successfully unmuted account {self.test_account_id}")
                    return True
                print_error("Account still appears muted after unmute call")
                return False
            else:
                print_error(f"Unmute failed: {resp.status_code} - {resp.text}")
                return False
        except Exception as e:
            print_error(f"Unmute error: {e}")
            return False

    def test_create_filter(self):
        """Test creating a filter"""
        print_test("Create Filter")
        
        filter_data = {
            "title": "Test Filter",
            "context": ["home", "public"],
            "filter_action": "warn",
            "keywords_attributes": [
                {"keyword": "spam", "whole_word": True},
                {"keyword": "test123", "whole_word": False}
            ]
        }
        
        try:
            resp = self.session.post(f"{BASE_URL}/api/v2/filters", json=filter_data)
            if resp.status_code == 200:
                filter_info = resp.json()
                self.test_filter_id = filter_info['id']
                print_success(f"Created filter '{filter_info['title']}' with ID {self.test_filter_id}")
                print_info(f"Keywords: {len(filter_info.get('keywords', []))}")
                return True
            else:
                print_error(f"Create filter failed: {resp.status_code} - {resp.text}")
                return False
        except Exception as e:
            print_error(f"Create filter error: {e}")
            return False

    def test_get_filters(self):
        """Test getting all filters"""
        print_test("Get Filters")
        
        try:
            resp = self.session.get(f"{BASE_URL}/api/v2/filters")
            if resp.status_code == 200:
                filters = resp.json()
                print_success(f"Retrieved {len(filters)} filters")
                for f in filters:
                    print_info(f"Filter: {f['title']} - Contexts: {', '.join(f['context'])}")
                return True
            else:
                print_error(f"Get filters failed: {resp.status_code} - {resp.text}")
                return False
        except Exception as e:
            print_error(f"Get filters error: {e}")
            return False

    def test_update_filter(self):
        """Test updating a filter"""
        print_test("Update Filter")
        
        if not self.test_filter_id:
            print_info("No filter to update, skipping")
            return True
        
        update_data = {
            "title": "Updated Test Filter",
            "filter_action": "hide"
        }
        
        try:
            resp = self.session.put(f"{BASE_URL}/api/v2/filters/{self.test_filter_id}", 
                                  json=update_data)
            if resp.status_code == 200:
                filter_info = resp.json()
                print_success(f"Updated filter to '{filter_info['title']}'")
                print_info(f"New action: {filter_info['filter_action']}")
                return True
            else:
                print_error(f"Update filter failed: {resp.status_code} - {resp.text}")
                return False
        except Exception as e:
            print_error(f"Update filter error: {e}")
            return False

    def test_add_filter_keyword(self):
        """Test adding a keyword to a filter"""
        print_test("Add Filter Keyword")
        
        if not self.test_filter_id:
            print_info("No filter to add keyword to, skipping")
            return True
        
        keyword_data = {
            "keyword": "newkeyword",
            "whole_word": True
        }
        
        try:
            resp = self.session.post(f"{BASE_URL}/api/v2/filters/{self.test_filter_id}/keywords", 
                                   json=keyword_data)
            if resp.status_code == 200:
                keyword_info = resp.json()
                print_success(f"Added keyword '{keyword_info['keyword']}'")
                return True
            else:
                print_error(f"Add keyword failed: {resp.status_code} - {resp.text}")
                return False
        except Exception as e:
            print_error(f"Add keyword error: {e}")
            return False

    def test_delete_filter(self):
        """Test deleting a filter"""
        print_test("Delete Filter")
        
        if not self.test_filter_id:
            print_info("No filter to delete, skipping")
            return True
        
        try:
            resp = self.session.delete(f"{BASE_URL}/api/v2/filters/{self.test_filter_id}")
            if resp.status_code == 200:
                print_success(f"Deleted filter {self.test_filter_id}")
                return True
            else:
                print_error(f"Delete filter failed: {resp.status_code} - {resp.text}")
                return False
        except Exception as e:
            print_error(f"Delete filter error: {e}")
            return False

    def run_all_tests(self):
        """Run all tests"""
        print(f"\n{BLUE}=== Filters & Mutes Test Suite ==={RESET}")
        print(f"Testing against: {BASE_URL}")
        print(f"Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
        
        if not self.register_and_login():
            print_error("Failed to register/login, aborting tests")
            return
        
        # Run mute tests
        print(f"\n{YELLOW}--- Mute Tests ---{RESET}")
        self.test_mute_account()
        self.test_get_muted_accounts()
        self.test_unmute_account()
        
        # Run filter tests
        print(f"\n{YELLOW}--- Filter Tests ---{RESET}")
        self.test_create_filter()
        self.test_get_filters()
        self.test_update_filter()
        self.test_add_filter_keyword()
        self.test_delete_filter()
        
        print(f"\n{BLUE}=== Test Suite Complete ==={RESET}")

if __name__ == "__main__":
    tester = FiltersMutesTest()
    tester.run_all_tests() 
