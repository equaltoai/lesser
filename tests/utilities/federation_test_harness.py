#!/usr/bin/env python3
"""
Lesser Federation Test Harness
Simulates federation between instances for testing ActivityPub flows
"""

import json
import time
import asyncio
import hashlib
import base64
from datetime import datetime, timezone
from typing import Dict, List, Any, Optional, Tuple
from dataclasses import dataclass, field
from urllib.parse import urlparse
import logging

try:
    import aiohttp
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import rsa, padding
    from cryptography.hazmat.backends import default_backend
except ImportError:
    print("Please install required packages: pip install aiohttp cryptography")
    exit(1)

# Set up logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)


@dataclass
class FederationInstance:
    """Represents a mock federation instance"""
    domain: str
    inbox_url: str
    actors: Dict[str, Dict[str, Any]] = field(default_factory=dict)
    received_activities: List[Dict[str, Any]] = field(default_factory=list)
    private_key: Any = None
    public_key: str = ""
    
    def __post_init__(self):
        # Generate keypair for signing
        self.private_key = rsa.generate_private_key(
            public_exponent=65537,
            key_size=2048,
            backend=default_backend()
        )
        self.public_key = self.private_key.public_key().public_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PublicFormat.SubjectPublicKeyInfo
        ).decode()


class FederationTestHarness:
    """Test harness for ActivityPub federation"""
    
    def __init__(self, target_instance_url: str):
        self.target_instance_url = target_instance_url.rstrip('/')
        self.mock_instances: Dict[str, FederationInstance] = {}
        self.test_results: List[Dict[str, Any]] = []
        self.session: Optional[aiohttp.ClientSession] = None
        
    async def __aenter__(self):
        self.session = aiohttp.ClientSession()
        return self
        
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        if self.session:
            await self.session.close()
    
    def create_mock_instance(self, domain: str) -> FederationInstance:
        """Create a mock federation instance"""
        instance = FederationInstance(
            domain=domain,
            inbox_url=f"https://{domain}/inbox",
        )
        self.mock_instances[domain] = instance
        logger.info(f"Created mock instance: {domain}")
        return instance
    
    def create_mock_actor(self, instance: FederationInstance, username: str) -> Dict[str, Any]:
        """Create a mock actor on an instance"""
        actor_id = f"https://{instance.domain}/users/{username}"
        actor = {
            "@context": "https://www.w3.org/ns/activitystreams",
            "id": actor_id,
            "type": "Person",
            "preferredUsername": username,
            "name": f"{username.title()} User",
            "inbox": f"{actor_id}/inbox",
            "outbox": f"{actor_id}/outbox",
            "followers": f"{actor_id}/followers",
            "following": f"{actor_id}/following",
            "publicKey": {
                "id": f"{actor_id}#main-key",
                "owner": actor_id,
                "publicKeyPem": instance.public_key
            },
            "endpoints": {
                "sharedInbox": instance.inbox_url
            }
        }
        instance.actors[username] = actor
        logger.info(f"Created mock actor: {username}@{instance.domain}")
        return actor
    
    def sign_headers(self, instance: FederationInstance, actor_id: str, 
                    method: str, path: str, headers: Dict[str, str]) -> Dict[str, str]:
        """Sign HTTP headers for federation"""
        # Create signing string
        signing_headers = ["(request-target)", "host", "date", "digest"]
        signing_values = {
            "(request-target)": f"{method.lower()} {path}",
            "host": urlparse(self.target_instance_url).netloc,
            "date": headers.get("Date", ""),
            "digest": headers.get("Digest", "")
        }
        
        signing_string = "\n".join([f"{h}: {signing_values.get(h, headers.get(h, ''))}" 
                                   for h in signing_headers])
        
        # Sign the string
        signature = base64.b64encode(
            instance.private_key.sign(
                signing_string.encode(),
                padding.PKCS1v15(),
                hashes.SHA256()
            )
        ).decode()
        
        # Create signature header
        sig_header = (
            f'keyId="{actor_id}#main-key",'
            f'algorithm="rsa-sha256",'
            f'headers="{" ".join(signing_headers)}",'
            f'signature="{signature}"'
        )
        
        headers["Signature"] = sig_header
        return headers
    
    async def send_activity(self, from_instance: FederationInstance, 
                           from_actor: str, activity: Dict[str, Any],
                           to_inbox: str) -> Tuple[bool, Dict[str, Any]]:
        """Send an activity from a mock instance to the target"""
        actor = from_instance.actors[from_actor]
        
        # Prepare headers
        body = json.dumps(activity).encode()
        digest = base64.b64encode(hashlib.sha256(body).digest()).decode()
        
        headers = {
            "Content-Type": "application/activity+json",
            "Date": datetime.now(timezone.utc).strftime("%a, %d %b %Y %H:%M:%S GMT"),
            "Digest": f"SHA-256={digest}",
            "User-Agent": "Lesser-Federation-Test-Harness/1.0"
        }
        
        # Sign headers
        parsed_url = urlparse(to_inbox)
        headers = self.sign_headers(from_instance, actor["id"], 
                                   "POST", parsed_url.path, headers)
        
        # Send request
        try:
            start_time = time.time()
            async with self.session.post(to_inbox, data=body, headers=headers) as response:
                elapsed = time.time() - start_time
                result = {
                    "success": response.status in [200, 201, 202],
                    "status_code": response.status,
                    "response_body": await response.text(),
                    "elapsed_time": elapsed,
                    "activity_id": activity.get("id"),
                    "activity_type": activity.get("type")
                }
                logger.info(f"Sent {activity['type']} to {to_inbox}: {response.status}")
                return result["success"], result
        except Exception as e:
            logger.error(f"Failed to send activity: {e}")
            return False, {
                "success": False,
                "error": str(e),
                "activity_id": activity.get("id"),
                "activity_type": activity.get("type")
            }
    
    async def test_follow_flow(self, follower_domain: str, follower_username: str,
                              followee_username: str) -> Dict[str, Any]:
        """Test a complete follow flow"""
        logger.info(f"Testing follow flow: {follower_username}@{follower_domain} -> {followee_username}")
        
        # Create mock instance and actor if needed
        if follower_domain not in self.mock_instances:
            instance = self.create_mock_instance(follower_domain)
            self.create_mock_actor(instance, follower_username)
        else:
            instance = self.mock_instances[follower_domain]
            if follower_username not in instance.actors:
                self.create_mock_actor(instance, follower_username)
        
        follower = instance.actors[follower_username]
        
        # Create Follow activity
        follow_activity = {
            "@context": "https://www.w3.org/ns/activitystreams",
            "id": f"{follower['id']}/activities/{int(time.time())}",
            "type": "Follow",
            "actor": follower["id"],
            "object": f"{self.target_instance_url}/users/{followee_username}",
            "published": datetime.now(timezone.utc).isoformat()
        }
        
        # Send to target instance inbox
        inbox_url = f"{self.target_instance_url}/users/{followee_username}/inbox"
        success, result = await self.send_activity(
            instance, follower_username, follow_activity, inbox_url
        )
        
        test_result = {
            "test": "follow_flow",
            "follower": f"{follower_username}@{follower_domain}",
            "followee": f"{followee_username}@{self.target_instance_url}",
            "activity": follow_activity,
            "result": result,
            "success": success,
            "timestamp": datetime.now(timezone.utc).isoformat()
        }
        
        self.test_results.append(test_result)
        return test_result
    
    def export_results(self, filename: str = "federation_test_results.json"):
        """Export test results to a file"""
        with open(filename, 'w') as f:
            json.dump({
                "target_instance": self.target_instance_url,
                "test_timestamp": datetime.now(timezone.utc).isoformat(),
                "mock_instances": {
                    domain: {
                        "actors": list(instance.actors.keys()),
                        "received_activities": len(instance.received_activities)
                    }
                    for domain, instance in self.mock_instances.items()
                },
                "test_results": self.test_results
            }, f, indent=2)
        logger.info(f"Test results exported to {filename}")


async def main():
    """Example usage of the federation test harness"""
    # Target instance to test
    target_url = "https://lesser.example.com"
    
    async with FederationTestHarness(target_url) as harness:
        # Run individual tests
        print("\n=== Testing Individual Federation Flows ===\n")
        
        # Test follow
        result = await harness.test_follow_flow("mastodon.social", "testuser", "localuser")
        print(f"Follow test: {'✓' if result['result']['success'] else '✗'}")
        
        # Export results
        harness.export_results()


if __name__ == "__main__":
    asyncio.run(main())
