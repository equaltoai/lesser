#!/usr/bin/env python3
"""
Expanded GraphQL validation script for Lesser API
Tests additional functionality beyond the basic comprehensive tests
"""
import os
import sys
import time
import json
import subprocess
from pathlib import Path
import requests
from typing import Dict, Optional
import jwt

GRAPHQL_ENDPOINT = os.getenv("GRAPHQL_ENDPOINT", "https://dev.lesser.host/api/graphql")
ADMIN_TOKEN = os.getenv("ADMIN_TOKEN")
MEMBER_TOKEN = os.getenv("MEMBER_TOKEN")
MOD_TOKEN = os.getenv("MOD_TOKEN")
# Delay between tests to avoid Lambda throttling (in seconds)
TEST_DELAY = float(os.getenv("GRAPHQL_TEST_DELAY", "0.5"))

BOOTSTRAP_ROOT = Path(__file__).resolve().parents[1]
JWT_SECRET_CACHE: Optional[str] = None

def get_jwt_secret() -> Optional[str]:
    """Retrieve the JWT secret from AWS Secrets Manager (cached for repeated use, matches seed_runner format)."""
    global JWT_SECRET_CACHE
    if JWT_SECRET_CACHE:
        return JWT_SECRET_CACHE
    
    try:
        result = subprocess.run(
            [
                "aws",
                "secretsmanager",
                "get-secret-value",
                "--secret-id",
                "lesser/jwt-secret",
                "--query",
                "SecretString",
                "--output",
                "text",
            ],
            capture_output=True,
            text=True,
            check=True,
            env=dict(os.environ, AWS_PROFILE=os.environ.get("AWS_PROFILE", "Lesser")),
        )
        secret_value = result.stdout.strip()
        
        # Parse JSON if the secret is stored as JSON (e.g., {"secret": "..."})
        try:
            secret_json = json.loads(secret_value)
            if isinstance(secret_json, dict) and "secret" in secret_json:
                JWT_SECRET_CACHE = secret_json["secret"]
            else:
                JWT_SECRET_CACHE = secret_value
        except (json.JSONDecodeError, ValueError):
            # Not JSON, use as-is
            JWT_SECRET_CACHE = secret_value
        
        return JWT_SECRET_CACHE
    except (subprocess.CalledProcessError, FileNotFoundError) as exc:
        print(f"Warning: Could not retrieve JWT secret: {exc}", file=sys.stderr)
        return None

def generate_token(secret: str, username: str, client_id: str, scopes: list = None) -> str:
    """Generate a JWT token for a user (matches seed_runner format)."""
    if scopes is None:
        scopes = ["read", "write", "follow", "push"]
    
    now = int(time.time())
    payload = {
        "sub": username,
        "iat": now,
        "exp": now + 3600,
        "username": username,
        "scopes": scopes,
        "client_id": client_id,
    }
    token = jwt.encode(payload, secret, algorithm="HS256")
    if isinstance(token, bytes):
        token = token.decode("utf-8")
    return f"Bearer {token}"

def get_token_for_user(username: str) -> Optional[str]:
    """Generate token for a specific user from bootstrap data."""
    secret = get_jwt_secret()
    if not secret:
        return None
    
    # Find bootstrap directory for user
    bootstrap_dirs = sorted(
        [p for p in BOOTSTRAP_ROOT.iterdir() 
         if p.is_dir() and p.name.startswith(f"bootstrap_{username}")]
    )
    if not bootstrap_dirs:
        return None
    
    user_dir = bootstrap_dirs[0]
    oauth_path = user_dir / "oauth_client.json"
    client_id = "4NQBEFCFIwtk9jd0r4u2Wa"  # Default client ID
    
    if oauth_path.exists():
        with oauth_path.open("r", encoding="utf-8") as handle:
            oauth_data = json.load(handle)
            client_id = oauth_data.get("ClientID", {}).get("S", client_id)
    
    scopes = ["read", "write", "follow", "push"]
    if username == "admin":
        scopes.append("admin")
    elif username == "mod":
        scopes.extend(["admin", "mod"])
    
    return generate_token(secret, username, client_id, scopes)

class ValidationResult:
    def __init__(self, name: str):
        self.name = name
        self.success = False
        self.error = None
        self.data = None
        self.duration = 0

class GraphQLValidator:
    def __init__(self, endpoint: str, token: str):
        self.endpoint = endpoint
        self.token = token if token.startswith("Bearer ") else f"Bearer {token}"
        self.results: list[ValidationResult] = []
        self.test_data = {}  # Store IDs for dependent tests
        
    def graphql_request(self, query: str, variables: Optional[Dict] = None, max_retries: int = 3) -> Optional[Dict]:
        """Execute a GraphQL request with retry logic"""
        headers = {
            "Authorization": self.token,
            "Content-Type": "application/json"
        }
        
        payload = {"query": query}
        if variables:
            payload["variables"] = variables
        
        for attempt in range(max_retries):
            try:
                response = requests.post(
                    self.endpoint, 
                    json=payload, 
                    headers=headers, 
                    timeout=30
                )
                
                if response.status_code == 503:
                    if attempt < max_retries - 1:
                        time.sleep(2 * (attempt + 1))
                        continue
                
                response.raise_for_status()
                return response.json()
            except requests.exceptions.RequestException as e:
                if attempt < max_retries - 1:
                    time.sleep(2 * (attempt + 1))
                    continue
                return {"errors": [{"message": str(e)}]}
        
        return None
    
    def test(self, name: str, query: str, variables: Optional[Dict] = None, expected_success: bool = True) -> ValidationResult:
        """Run a test and record results"""
        result = ValidationResult(name)
        start_time = time.time()
        
        # Add delay before test to avoid Lambda throttling
        if TEST_DELAY > 0:
            time.sleep(TEST_DELAY)
        
        try:
            response = self.graphql_request(query, variables)
            result.duration = time.time() - start_time
            
            if not response:
                result.error = "No response from server"
            elif "errors" in response:
                result.error = response["errors"]
                # Some errors are expected (e.g., not found, validation errors)
                if not expected_success:
                    result.success = True  # Expected error
            elif "data" in response:
                result.success = True
                result.data = response["data"]
            else:
                result.error = "Unexpected response format"
                
        except Exception as e:
            result.duration = time.time() - start_time
            result.error = str(e)
        
        self.results.append(result)
        return result
    
    def print_result(self, result: ValidationResult):
        """Print formatted test result"""
        status = "✓" if result.success else "✗"
        print(f"{status} {result.name} ({result.duration:.2f}s)", end="")
        if result.error:
            error_msg = str(result.error)[:100]
            print(f" - {error_msg}")
        else:
            print()
        if result.data and isinstance(result.data, dict):
            keys = list(result.data.keys())[:3]
            print(f"    Keys: {', '.join(keys)}")
    
    def print_summary(self):
        """Print summary of all tests"""
        print("\n" + "="*80)
        print("VALIDATION SUMMARY")
        print("="*80)
        
        total = len(self.results)
        passed = sum(1 for r in self.results if r.success)
        failed = total - passed
        
        print(f"Total Tests: {total}")
        print(f"Passed: {passed}")
        print(f"Failed: {failed}")
        print(f"Success Rate: {passed/total*100:.1f}%")
        
        if failed > 0:
            print("\nFailed Tests:")
            for r in self.results:
                if not r.success:
                    print(f"  ✗ {r.name}: {r.error}")

def main():
    print("="*80)
    print("EXPANDED GRAPHQL VALIDATION")
    print("="*80)
    print(f"Endpoint: {GRAPHQL_ENDPOINT}")
    print()
    
    if not ADMIN_TOKEN:
        print("ERROR: ADMIN_TOKEN environment variable is required")
        sys.exit(1)
    
    # Create validators for different accounts
    admin_validator = GraphQLValidator(GRAPHQL_ENDPOINT, ADMIN_TOKEN)
    
    # Get member token - use env var if available, otherwise try to generate from bootstrap
    member_token = MEMBER_TOKEN if MEMBER_TOKEN else get_token_for_user("member")
    if not member_token:
        member_token = ADMIN_TOKEN  # Fallback to admin token
    member_validator = GraphQLValidator(GRAPHQL_ENDPOINT, member_token)
    
    # Get mod token - use env var if available, otherwise try to generate from bootstrap
    mod_token = MOD_TOKEN if MOD_TOKEN else get_token_for_user("mod")
    if not mod_token:
        mod_token = ADMIN_TOKEN  # Fallback to admin token
    mod_validator = GraphQLValidator(GRAPHQL_ENDPOINT, mod_token)
    
    # Default validator (for most operations, uses admin)
    validator = admin_validator
    
    # ===== CONVERSATIONS =====
    print("\n--- Conversations ---")
    
    validator.test("Get Conversations", """
        query {
            conversations(first: 10) {
                id
                unread
                accounts {
                    id
                    username
                }
            }
        }
    """)
    
    # ===== LISTS OPERATIONS =====
    print("\n--- Lists Operations ---")
    
    # Create a list
    create_list_result = validator.test("Create List", """
        mutation {
            createList(input: {
                title: "Test List for Validation"
                repliesPolicy: FOLLOWED
            }) {
                id
                title
                repliesPolicy
            }
        }
    """)
    
    list_id = None
    if create_list_result.success and create_list_result.data:
        list_data = create_list_result.data.get("createList", {})
        list_id = list_data.get("id")
        validator.test_data["list_id"] = list_id
    
    if list_id:
        validator.test("Get List", f"""
            query {{
                list(id: "{list_id}") {{
                    id
                    title
                    accountCount
                }}
            }}
        """)
        
        validator.test("Get List Accounts", f"""
            query {{
                listAccounts(id: "{list_id}") {{
                    id
                    username
                }}
            }}
        """)
        
        # Get member ID for adding to list
        member_result = validator.test("Get Member Actor (for list)", """
            query {
                actor(username: "member") {
                    id
                }
            }
        """)
        
        member_id = None
        if member_result.success and member_result.data:
            member_data = member_result.data.get("actor", {})
            member_id = member_data.get("id")
            validator.test_data["member_id"] = member_id
        
        if member_id:
            validator.test("Add Accounts to List", f"""
                mutation {{
                    addAccountsToList(id: "{list_id}", accountIds: ["{member_id}"]) {{
                        id
                        accountCount
                    }}
                }}
            """)
            
            validator.test("Remove Accounts from List", f"""
                mutation {{
                    removeAccountsFromList(id: "{list_id}", accountIds: ["{member_id}"]) {{
                        id
                        accountCount
                    }}
                }}
            """)
        
        validator.test("Update List", f"""
            mutation {{
                updateList(id: "{list_id}", input: {{
                    title: "Updated Test List"
                    exclusive: true
                }}) {{
                    id
                    title
                    exclusive
                }}
            }}
        """)
        
        validator.test("Delete List", f"""
            mutation {{
                deleteList(id: "{list_id}")
            }}
        """)
        
        validator.test("Get List (should fail)", f"""
            query {{
                list(id: "{list_id}") {{
                    id
                }}
            }}
        """, expected_success=False)
    
    # ===== MEDIA OPERATIONS =====
    print("\n--- Media Operations ---")
    
    validator.test("Get Media Library", """
        query {
            mediaLibrary(first: 10) {
                edges {
                    node {
                        id
                        type
                        url
                    }
                }
                totalCount
            }
        }
    """)
    
    # ===== SCHEDULED STATUSES =====
    print("\n--- Scheduled Statuses ---")
    
    # Create a scheduled status
    from datetime import datetime, timedelta
    scheduled_time = (datetime.utcnow() + timedelta(hours=1)).isoformat() + "Z"
    
    schedule_result = validator.test("Schedule Status", f"""
        mutation {{
            scheduleStatus(input: {{
                text: "This is a scheduled status for validation"
                scheduledAt: "{scheduled_time}"
                visibility: PUBLIC
            }}) {{
                id
                scheduledAt
                params {{
                    text
                    visibility
                }}
            }}
        }}
    """)
    
    scheduled_id = None
    if schedule_result.success and schedule_result.data:
        scheduled_data = schedule_result.data.get("scheduleStatus", {})
        scheduled_id = scheduled_data.get("id")
        validator.test_data["scheduled_id"] = scheduled_id
    
    if scheduled_id:
        validator.test("Get Scheduled Status", f"""
            query {{
                scheduledStatus(id: "{scheduled_id}") {{
                    id
                    scheduledAt
                    params {{
                        text
                    }}
                }}
            }}
        """)
        
        validator.test("Get Scheduled Statuses", """
            query {
                scheduledStatuses(first: 10) {
                    id
                    scheduledAt
                }
            }
        """)
        
        new_scheduled_time = (datetime.utcnow() + timedelta(hours=2)).isoformat() + "Z"
        validator.test("Update Scheduled Status", f"""
            mutation {{
                updateScheduledStatus(id: "{scheduled_id}", input: {{
                    scheduledAt: "{new_scheduled_time}"
                }}) {{
                    id
                    scheduledAt
                }}
            }}
        """)
        
        validator.test("Cancel Scheduled Status", f"""
            mutation {{
                cancelScheduledStatus(id: "{scheduled_id}")
            }}
        """)
    
    # ===== CUSTOM EMOJIS =====
    print("\n--- Custom Emojis ---")
    
    validator.test("Get Custom Emojis", """
        query {
            customEmojis {
                id
                shortcode
                url
                visibleInPicker
            }
        }
    """)
    
    # ===== PUSH SUBSCRIPTIONS =====
    print("\n--- Push Subscriptions ---")
    
    validator.test("Get Push Subscription", """
        query {
            pushSubscription {
                id
                endpoint
                alerts {
                    follow
                    favourite
                }
            }
        }
    """)
    
    # ===== USER PREFERENCES =====
    print("\n--- User Preferences ---")
    
    validator.test("Get User Preferences", """
        query {
            userPreferences {
                posting {
                    defaultVisibility
                    defaultSensitive
                }
                reading {
                    expandSpoilers
                }
            }
        }
    """)
    
    validator.test("Update User Preferences", """
        mutation {
            updateUserPreferences(input: {
                defaultPostingVisibility: UNLISTED
                defaultMediaSensitive: false
            }) {
                posting {
                    defaultVisibility
                    defaultSensitive
                }
            }
        }
    """)
    
    # ===== BLOCK/MUTE OPERATIONS =====
    print("\n--- Block/Mute Operations ---")
    
    member_id = validator.test_data.get("member_id")
    if not member_id:
        member_result = validator.test("Get Member Actor (for block)", """
            query {
                actor(username: "member") {
                    id
                }
            }
        """)
        if member_result.success and member_result.data:
            member_data = member_result.data.get("actor", {})
            member_id = member_data.get("id")
    
    if member_id:
        # First, follow the member to create a relationship that can be updated
        # Check if already following first, then follow if not
        follow_result = validator.test("Follow Actor (for relationship update)", f"""
            mutation {{
                followActor(id: "{member_id}") {{
                    id
                    following
                }}
            }}
        """)
        
        # If follow failed with 422 (might already be following), try to unfollow first, then follow again
        if not follow_result.success and "422" in str(follow_result.error):
            validator.test("Unfollow Actor (pre-cleanup for update)", f"""
                mutation {{
                    unfollowActor(id: "{member_id}") {{
                        id
                        following
                    }}
                }}
            """)
            # Try following again
            validator.test("Follow Actor (retry for relationship update)", f"""
                mutation {{
                    followActor(id: "{member_id}") {{
                        id
                        following
                    }}
                }}
            """)
        
        # Now we can update the relationship (only if we successfully followed)
        validator.test("Update Relationship", f"""
            mutation {{
                updateRelationship(id: "{member_id}", input: {{
                    showReblogs: false
                    note: "Test note"
                }}) {{
                    id
                    note
                    showingReblogs
                }}
            }}
        """)
        
        validator.test("Block Actor", f"""
            mutation {{
                blockActor(id: "{member_id}") {{
                    id
                    blocking
                }}
            }}
        """)
        
        validator.test("Get Relationship (after block)", f"""
            query {{
                relationship(id: "{member_id}") {{
                    blocking
                    blockedBy
                }}
            }}
        """)
        
        validator.test("Mute Actor", f"""
            mutation {{
                muteActor(id: "{member_id}", notifications: false) {{
                    id
                    muting
                }}
            }}
        """)
        
        validator.test("Unmute Actor", f"""
            mutation {{
                unmuteActor(id: "{member_id}")
            }}
        """)
        
        validator.test("Unblock Actor", f"""
            mutation {{
                unblockActor(id: "{member_id}")
            }}
        """)
    
    # ===== PIN/UNPIN OPERATIONS =====
    print("\n--- Pin/Unpin Operations ---")
    
    # Create a post to pin
    create_post_result = validator.test("Create Post (for pin)", """
        mutation {
            createNote(input: {
                content: "Post to pin for validation"
                visibility: PUBLIC
            }) {
                object {
                    id
                    content
                }
            }
        }
    """)
    
    post_id = None
    if create_post_result.success and create_post_result.data:
        note_data = create_post_result.data.get("createNote", {}).get("object", {})
        post_id = note_data.get("id")
    
    if post_id:
        validator.test("Pin Object", f"""
            mutation {{
                pinObject(id: "{post_id}") {{
                    id
                }}
            }}
        """)
        
        validator.test("Unpin Object", f"""
            mutation {{
                unpinObject(id: "{post_id}")
            }}
        """)
        
        # Clean up
        validator.test("Delete Post (cleanup)", f"""
            mutation {{
                deleteObject(id: "{post_id}")
            }}
        """)
    
    # ===== NOTIFICATION OPERATIONS =====
    print("\n--- Notification Operations ---")
    
    validator.test("Get Notifications (with filters)", """
        query {
            notifications(types: ["follow", "mention"], first: 10) {
                edges {
                    node {
                        id
                        type
                        read
                    }
                }
                totalCount
            }
        }
    """)
    
    validator.test("Clear Notifications", """
        mutation {
            clearNotifications
        }
    """)
    
    # ===== QUOTE POSTS =====
    print("\n--- Quote Posts ---")
    
    # Create a post to quote
    original_post_result = validator.test("Create Original Post (for quote)", """
        mutation {
            createNote(input: {
                content: "Original post to be quoted"
                visibility: PUBLIC
            }) {
                object {
                    id
                    content
                }
            }
        }
    """)
    
    original_post_id = None
    if original_post_result.success and original_post_result.data:
        note_data = original_post_result.data.get("createNote", {}).get("object", {})
        original_post_id = note_data.get("id")
    
    if original_post_id:
        # Get the post URL for quoting
        validator.test("Get Object (for quote URL)", f"""
            query {{
                object(id: "{original_post_id}") {{
                    id
                }}
            }}
        """)
        
        # Quote the post
        quote_result = validator.test("Create Quote Note", f"""
            mutation {{
                createQuoteNote(input: {{
                    content: "Quoting this post for validation"
                    quoteUrl: "https://dev.lesser.host/objects/{original_post_id}"
                    quoteType: COMMENTARY
                    visibility: PUBLIC
                }}) {{
                    object {{
                        id
                        content
                        quoteUrl
                    }}
                }}
            }}
        """)
        
        quote_post_id = None
        if quote_result.success and quote_result.data:
            quote_data = quote_result.data.get("createQuoteNote", {}).get("object", {})
            quote_post_id = quote_data.get("id")
        
        if quote_post_id:
            validator.test("Get Object with Quotes", f"""
                query {{
                    object(id: "{original_post_id}") {{
                        id
                        quoteCount
                        quotes(first: 5) {{
                            edges {{
                                node {{
                                    id
                                    content
                                }}
                            }}
                        }}
                    }}
                }}
            """)
            
            validator.test("Update Quote Permissions", f"""
                mutation {{
                    updateQuotePermissions(
                        noteId: "{original_post_id}"
                        quoteable: true
                        permission: FOLLOWERS
                    ) {{
                        success
                        note {{
                            id
                        }}
                    }}
                }}
            """)
            
            validator.test("Withdraw From Quotes", f"""
                mutation {{
                    withdrawFromQuotes(noteId: "{quote_post_id}") {{
                        success
                        withdrawnCount
                    }}
                }}
            """)
        
        # Clean up
        if quote_post_id:
            validator.test("Delete Quote Post (cleanup)", f"""
                mutation {{
                    deleteObject(id: "{quote_post_id}")
                }}
            """)
        validator.test("Delete Original Post (cleanup)", f"""
            mutation {{
                deleteObject(id: "{original_post_id}")
            }}
        """)
    
    # ===== HASHTAG FOLLOWING =====
    print("\n--- Hashtag Following ---")
    
    # Create a post with hashtag first
    validator.test("Create Post with Hashtag", """
        mutation {
            createNote(input: {
                content: "Post with #validation hashtag for testing"
                visibility: PUBLIC
            }) {
                object {
                    id
                }
            }
        }
    """)
    
    validator.test("Get Hashtag", """
        query {
            hashtag(name: "validation") {
                name
                followerCount
                postCount
                isFollowing
            }
        }
    """)
    
    validator.test("Follow Hashtag", """
        mutation {
            followHashtag(hashtag: "validation", notifyLevel: ALL) {
                success
                hashtag {
                    name
                    isFollowing
                }
            }
        }
    """)
    
    validator.test("Get Followed Hashtags", """
        query {
            followedHashtags(first: 10) {
                edges {
                    node {
                        name
                        isFollowing
                    }
                }
            }
        }
    """)
    
    validator.test("Hashtag Timeline", """
        query {
            hashtagTimeline(hashtag: "validation", first: 10) {
                edges {
                    node {
                        id
                        content
                    }
                }
            }
        }
    """)
    
    validator.test("Multi Hashtag Timeline", """
        query {
            multiHashtagTimeline(hashtags: ["validation", "test"], mode: ANY, first: 10) {
                edges {
                    node {
                        id
                        content
                    }
                }
            }
        }
    """)
    
    validator.test("Suggested Hashtags", """
        query {
            suggestedHashtags(limit: 10) {
                hashtag {
                    name
                }
                reason
            }
        }
    """)
    
    validator.test("Update Hashtag Notifications", """
        mutation {
            updateHashtagNotifications(
                hashtag: "validation"
                settings: {
                    level: MUTUALS
                    muted: false
                }
            ) {
                success
                hashtag {
                    name
                    notificationSettings {
                        level
                    }
                }
            }
        }
    """)
    
    validator.test("Unfollow Hashtag", """
        mutation {
            unfollowHashtag(hashtag: "validation") {
                success
                hashtag {
                    name
                    isFollowing
                }
            }
        }
    """)
    
    # ===== THREAD SYNCHRONIZATION =====
    print("\n--- Thread Synchronization ---")
    
    # Create a thread
    root_post_result = validator.test("Create Root Post (for thread)", """
        mutation {
            createNote(input: {
                content: "Root post for thread synchronization test"
                visibility: PUBLIC
            }) {
                object {
                    id
                }
            }
        }
    """)
    
    root_post_id = None
    if root_post_result.success and root_post_result.data:
        root_data = root_post_result.data.get("createNote", {}).get("object", {})
        root_post_id = root_data.get("id")
    
    if root_post_id:
        validator.test("Get Thread Context", f"""
            query {{
                threadContext(noteId: "{root_post_id}") {{
                    rootNote {{
                        id
                    }}
                    replyCount
                    syncStatus
                }}
            }}
        """)
        
        validator.test("Sync Missing Replies", f"""
            mutation {{
                syncMissingReplies(noteId: "{root_post_id}") {{
                    success
                    syncedReplies
                }}
            }}
        """)
        
        # Clean up
        validator.test("Delete Root Post (cleanup)", f"""
            mutation {{
                deleteObject(id: "{root_post_id}")
            }}
        """)
    
    # ===== SEVERED RELATIONSHIPS =====
    print("\n--- Severed Relationships ---")
    
    validator.test("Get Severed Relationships", """
        query {
            severedRelationships(first: 10) {
                edges {
                    node {
                        id
                        localInstance
                        remoteInstance
                        reason
                    }
                }
            }
        }
    """)
    
    # ===== TRUST & MODERATION =====
    print("\n--- Trust & Moderation ---")
    
    validator.test("Get Moderation Queue", """
        query {
            moderationQueue(first: 10) {
                id
                decision
                confidence
            }
        }
    """, expected_success=False)  # Expected: no moderation events yet
    
    member_id = validator.test_data.get("member_id")
    if not member_id:
        member_result = validator.test("Get Member Actor (for trust)", """
            query {
                actor(username: "member") {
                    id
                }
            }
        """)
        if member_result.success and member_result.data:
            member_data = member_result.data.get("actor", {})
            member_id = member_data.get("id")
    
    if member_id:
        validator.test("Get Trust Graph", f"""
            query {{
                trustGraph(actorId: "{member_id}", category: CONTENT) {{
                    from {{
                        id
                    }}
                    to {{
                        id
                    }}
                    score
                }}
            }}
        """, expected_success=False)  # Expected: DynamoDB index may not exist yet
        
        validator.test("Update Trust", f"""
            mutation {{
                updateTrust(input: {{
                    targetActorId: "{member_id}"
                    category: CONTENT
                    score: 0.8
                }}) {{
                    score
                    category
                }}
            }}
        """)
    
    # Create a post to flag
    flag_post_result = validator.test("Create Post (for flag)", """
        mutation {
            createNote(input: {
                content: "Post to flag for moderation testing"
                visibility: PUBLIC
            }) {
                object {
                    id
                }
            }
        }
    """)
    
    flag_post_id = None
    if flag_post_result.success and flag_post_result.data:
        flag_data = flag_post_result.data.get("createNote", {}).get("object", {})
        flag_post_id = flag_data.get("id")
    
    if flag_post_id:
        validator.test("Flag Object", f"""
            mutation {{
                flagObject(input: {{
                    objectId: "{flag_post_id}"
                    reason: "Test flag for validation"
                    evidence: ["Test evidence"]
                }}) {{
                    moderationId
                    queued
                }}
            }}
        """)
        
        # Clean up
        validator.test("Delete Flagged Post (cleanup)", f"""
            mutation {{
                deleteObject(id: "{flag_post_id}")
            }}
        """)
    
    # ===== COMMUNITY NOTES =====
    print("\n--- Community Notes ---")
    
    # Create a post for community note (as admin)
    note_post_result = admin_validator.test("Create Post (for community note)", """
        mutation {
            createNote(input: {
                content: "Post for community note testing"
                visibility: PUBLIC
            }) {
                object {
                    id
                }
            }
        }
    """)
    
    note_post_id = None
    if note_post_result.success and note_post_result.data:
        note_data = note_post_result.data.get("createNote", {}).get("object", {})
        note_post_id = note_data.get("id")
    
    if note_post_id:
        # Admin adds community note
        community_note_result = admin_validator.test("Add Community Note", f"""
            mutation {{
                addCommunityNote(input: {{
                    objectId: "{note_post_id}"
                    content: "This is a test community note"
                }}) {{
                    note {{
                        id
                        content
                    }}
                    object {{
                        id
                    }}
                }}
            }}
        """)
        
        community_note_id = None
        if community_note_result.success and community_note_result.data:
            cn_data = community_note_result.data.get("addCommunityNote", {}).get("note", {})
            community_note_id = cn_data.get("id")
        
        if community_note_id:
            # Member votes on admin's community note (different actor)
            member_validator.test("Vote Community Note", f"""
                mutation {{
                    voteCommunityNote(id: "{community_note_id}", helpful: true) {{
                        id
                        helpful
                        notHelpful
                    }}
                }}
            """)
        
        # Clean up
        admin_validator.test("Delete Post (cleanup)", f"""
            mutation {{
                deleteObject(id: "{note_post_id}")
            }}
        """)
    
    # ===== AI ANALYSIS =====
    print("\n--- AI Analysis ---")
    
    validator.test("Get AI Capabilities", """
        query {
            aiCapabilities {
                textAnalysis {
                    sentimentAnalysis
                    toxicityDetection
                }
                imageAnalysis {
                    nsfwDetection
                }
            }
        }
    """, expected_success=False)  # Expected: AI Analysis disabled by default
    
    validator.test("Get AI Stats", """
        query {
            aiStats(period: DAY) {
                period
                totalAnalyses
                toxicityRate
                spamRate
            }
        }
    """, expected_success=False)  # Expected: AI Analysis disabled by default
    
    # Create a post for AI analysis
    ai_post_result = validator.test("Create Post (for AI analysis)", """
        mutation {
            createNote(input: {
                content: "Post for AI analysis testing"
                visibility: PUBLIC
            }) {
                object {
                    id
                }
            }
        }
    """)
    
    ai_post_id = None
    if ai_post_result.success and ai_post_result.data:
        ai_data = ai_post_result.data.get("createNote", {}).get("object", {})
        ai_post_id = ai_data.get("id")
    
    if ai_post_id:
        validator.test("Request AI Analysis", f"""
            mutation {{
                requestAIAnalysis(objectId: "{ai_post_id}", objectType: "status", force: false) {{
                    message
                    objectId
                }}
            }}
        """, expected_success=False)  # Expected: AI Analysis disabled by default
        
        validator.test("Get AI Analysis", f"""
            query {{
                aiAnalysis(objectId: "{ai_post_id}") {{
                    id
                    objectId
                    overallRisk
                    moderationAction
                }}
            }}
        """, expected_success=False)  # Expected: AI Analysis disabled by default
        
        # Clean up
        validator.test("Delete Post (cleanup)", f"""
            mutation {{
                deleteObject(id: "{ai_post_id}")
            }}
        """)
    
    # ===== EXPLAIN OBJECT =====
    print("\n--- Debug Operations ---")
    
    # Create a post to explain
    explain_post_result = validator.test("Create Post (for explain)", """
        mutation {
            createNote(input: {
                content: "Post for explain object testing"
                visibility: PUBLIC
            }) {
                object {
                    id
                }
            }
        }
    """)
    
    explain_post_id = None
    if explain_post_result.success and explain_post_result.data:
        explain_data = explain_post_result.data.get("createNote", {}).get("object", {})
        explain_post_id = explain_data.get("id")
    
    if explain_post_id:
        validator.test("Explain Object", f"""
            query {{
                explainObject(id: "{explain_post_id}") {{
                    object {{
                        id
                    }}
                    storageLocation
                    sizeBytes
                }}
            }}
        """)
        
        # Clean up
        validator.test("Delete Post (cleanup)", f"""
            mutation {{
                deleteObject(id: "{explain_post_id}")
            }}
        """)
        
        # Try to explain after deletion (should fail)
        validator.test("Explain Object (after delete)", f"""
            query {{
                explainObject(id: "{explain_post_id}") {{
                    object {{
                        id
                    }}
                    storageLocation
                    sizeBytes
                }}
            }}
        """, expected_success=False)  # Expected: object was deleted
    
    validator.test("Get Federation Status", """
        query {
            federationStatus(domain: "example.com") {
                domain
                reachable
                lastContact
            }
        }
    """)
    
    # Aggregate results from all validators
    all_results = admin_validator.results.copy()
    all_results.extend(member_validator.results)
    all_results.extend(mod_validator.results)
    
    # Print all results
    print("\n" + "="*80)
    print("TEST RESULTS")
    print("="*80)
    for result in all_results:
        admin_validator.print_result(result)
    
    # Print summary using aggregated results
    print("\n" + "="*80)
    print("VALIDATION SUMMARY")
    print("="*80)
    
    total = len(all_results)
    passed = sum(1 for r in all_results if r.success)
    failed = total - passed
    
    print(f"Total Tests: {total}")
    print(f"Passed: {passed}")
    print(f"Failed: {failed}")
    print(f"Success Rate: {passed/total*100:.1f}%")
    
    if failed > 0:
        print("\nFailed Tests:")
        for r in all_results:
            if not r.success:
                error_msg = str(r.error)[:100] if r.error else "Unknown error"
                print(f"  ✗ {r.name}: {error_msg}")
    
    # Exit with error code if any tests failed
    sys.exit(1 if failed > 0 else 0)

if __name__ == "__main__":
    main()
