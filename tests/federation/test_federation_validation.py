#!/usr/bin/env python3
"""
Federation Validation Test Suite for Lesser
Tests ActivityPub compliance and federation capabilities
"""

import requests
from urllib.parse import urlparse
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.hazmat.primitives import serialization
import logging

logger = logging.getLogger(__name__)


class FederationValidator:
    """Validate ActivityPub federation compliance"""
    
    def __init__(self, instance_url: str):
        self.instance_url = instance_url.rstrip('/')
        self.domain = urlparse(instance_url).hostname
        self.session = requests.Session()
        
    def test_webfinger(self, username: str):
        """Test WebFinger endpoint"""
        logger.info(f"\n🔍 Testing WebFinger for {username}@{self.domain}")
        
        resource = f"acct:{username}@{self.domain}"
        response = self.session.get(
            f"{self.instance_url}/.well-known/webfinger",
            params={'resource': resource},
            headers={'Accept': 'application/jrd+json'}
        )
        
        if response.status_code != 200:
            logger.error(f"❌ WebFinger failed: {response.status_code}")
            return False
            
        data = response.json()
        
        # Validate WebFinger response
        checks = {
            'subject': data.get('subject') == resource,
            'links': any(link.get('rel') == 'self' and 
                        link.get('type') == 'application/activity+json' 
                        for link in data.get('links', []))
        }
        
        if all(checks.values()):
            logger.info("✅ WebFinger response valid")
            
            # Extract actor URL
            for link in data.get('links', []):
                if link.get('rel') == 'self':
                    return link.get('href')
        else:
            logger.error(f"❌ WebFinger validation failed: {checks}")
            
        return None
        
    def test_actor_endpoint(self, actor_url: str):
        """Test actor endpoint"""
        logger.info(f"\n👤 Testing actor endpoint: {actor_url}")
        
        response = self.session.get(
            actor_url,
            headers={'Accept': 'application/activity+json'}
        )
        
        if response.status_code != 200:
            logger.error(f"❌ Actor fetch failed: {response.status_code}")
            return None
            
        actor = response.json()
        
        # Validate required fields
        required_fields = [
            '@context', 'id', 'type', 'preferredUsername',
            'inbox', 'outbox', 'publicKey'
        ]
        
        missing = [field for field in required_fields if field not in actor]
        if missing:
            logger.error(f"❌ Missing required fields: {missing}")
            return None
            
        # Validate publicKey
        public_key = actor.get('publicKey', {})
        if not all(k in public_key for k in ['id', 'owner', 'publicKeyPem']):
            logger.error("❌ Invalid publicKey structure")
            return None
            
        logger.info("✅ Actor endpoint valid")
        return actor
        
    def test_outbox(self, actor: dict):
        """Test outbox endpoint"""
        outbox_url = actor.get('outbox')
        if not outbox_url:
            logger.error("❌ No outbox URL")
            return False
            
        logger.info(f"\n📤 Testing outbox: {outbox_url}")
        
        response = self.session.get(
            outbox_url,
            headers={'Accept': 'application/activity+json'}
        )
        
        if response.status_code == 401:
            logger.info("✅ Outbox requires authentication (valid)")
            return True
        elif response.status_code == 200:
            data = response.json()
            if data.get('type') in ['OrderedCollection', 'OrderedCollectionPage']:
                logger.info("✅ Outbox returns valid collection")
                return True
                
        logger.error(f"❌ Outbox test failed: {response.status_code}")
        return False
        
    def test_inbox_discovery(self, actor: dict):
        """Test inbox discovery"""
        inbox_url = actor.get('inbox')
        shared_inbox = actor.get('endpoints', {}).get('sharedInbox')
        
        logger.info(f"\n📥 Testing inbox discovery")
        
        if inbox_url:
            logger.info(f"✅ Inbox URL found: {inbox_url}")
        else:
            logger.error("❌ No inbox URL")
            return False
            
        if shared_inbox:
            logger.info(f"✅ Shared inbox found: {shared_inbox}")
        else:
            logger.warning("⚠️  No shared inbox (optional)")
            
        return True
        
    def test_nodeinfo(self):
        """Test NodeInfo endpoints"""
        logger.info(f"\n🌐 Testing NodeInfo")
        
        # Test .well-known/nodeinfo
        response = self.session.get(f"{self.instance_url}/.well-known/nodeinfo")
        
        if response.status_code != 200:
            logger.error(f"❌ NodeInfo discovery failed: {response.status_code}")
            return False
            
        data = response.json()
        links = data.get('links', [])
        
        # Find NodeInfo 2.0 link
        nodeinfo_url = None
        for link in links:
            if '2.0' in link.get('rel', ''):
                nodeinfo_url = link.get('href')
                break
                
        if not nodeinfo_url:
            logger.error("❌ No NodeInfo 2.0 link found")
            return False
            
        # Fetch NodeInfo
        response = self.session.get(nodeinfo_url)
        if response.status_code != 200:
            logger.error(f"❌ NodeInfo fetch failed: {response.status_code}")
            return False
            
        nodeinfo = response.json()
        
        # Validate NodeInfo
        if nodeinfo.get('software', {}).get('name') and \
           nodeinfo.get('protocols') and \
           'activitypub' in nodeinfo.get('protocols', []):
            logger.info("✅ NodeInfo valid")
            logger.info(f"   Software: {nodeinfo['software']['name']} {nodeinfo['software'].get('version', 'unknown')}")
            logger.info(f"   Protocols: {', '.join(nodeinfo['protocols'])}")
            return True
            
        logger.error("❌ NodeInfo validation failed")
        return False
        
    def test_http_signatures(self, actor: dict):
        """Test HTTP signature verification readiness"""
        logger.info(f"\n🔏 Testing HTTP signature support")
        
        public_key_pem = actor.get('publicKey', {}).get('publicKeyPem')
        if not public_key_pem:
            logger.error("❌ No public key PEM found")
            return False
            
        try:
            # Try to load the public key
            from cryptography.hazmat.backends import default_backend
            public_key = serialization.load_pem_public_key(
                public_key_pem.encode('utf-8'),
                backend=default_backend()
            )
            logger.info("✅ Public key is valid PEM format")
            
            # Check key type and size
            if isinstance(public_key, rsa.RSAPublicKey):
                key_size = public_key.key_size
                if key_size >= 2048:
                    logger.info(f"✅ RSA key size adequate: {key_size} bits")
                else:
                    logger.warning(f"⚠️  RSA key size small: {key_size} bits")
            else:
                logger.info("✅ Non-RSA public key accepted")
                
            return True
            
        except Exception as e:
            logger.error(f"❌ Public key validation failed: {e}")
            return False
            
    def test_collections(self, actor: dict):
        """Test collection endpoints"""
        logger.info(f"\n📚 Testing collections")
        
        collections = {
            'followers': actor.get('followers'),
            'following': actor.get('following'),
            'liked': actor.get('liked'),
            'featured': actor.get('featured')
        }
        
        for name, url in collections.items():
            if not url:
                logger.info(f"⏭️  {name} collection not present (optional)")
                continue
                
            response = self.session.get(
                url,
                headers={'Accept': 'application/activity+json'}
            )
            
            if response.status_code == 200:
                data = response.json()
                if data.get('type') in ['OrderedCollection', 'OrderedCollectionPage', 'Collection']:
                    logger.info(f"✅ {name} collection valid")
                else:
                    logger.error(f"❌ {name} collection invalid type: {data.get('type')}")
            elif response.status_code == 401:
                logger.info(f"✅ {name} collection requires auth (valid)")
            else:
                logger.error(f"❌ {name} collection failed: {response.status_code}")
                
    def test_media_types(self):
        """Test content negotiation"""
        logger.info(f"\n📋 Testing content negotiation")
        
        # Test actor endpoint with different Accept headers
        test_url = f"{self.instance_url}/users/aron"  # Assuming aron exists
        
        tests = [
            ('application/activity+json', 'ActivityPub'),
            ('application/ld+json; profile="https://www.w3.org/ns/activitystreams"', 'JSON-LD'),
            ('text/html', 'HTML')
        ]
        
        for accept_header, name in tests:
            response = self.session.get(
                test_url,
                headers={'Accept': accept_header}
            )
            
            if response.status_code == 200:
                content_type = response.headers.get('Content-Type', '')
                if 'json' in accept_header and 'json' in content_type:
                    logger.info(f"✅ {name} content negotiation works")
                elif 'html' in accept_header and 'html' in content_type:
                    logger.info(f"✅ {name} content negotiation works")
                else:
                    logger.warning(f"⚠️  {name} returned unexpected content-type: {content_type}")
            else:
                logger.error(f"❌ {name} request failed: {response.status_code}")
                
    def test_instance_actor(self):
        """Test instance actor (for relays)"""
        logger.info(f"\n🏛️  Testing instance actor")
        
        instance_actor_url = f"{self.instance_url}/actor"
        response = self.session.get(
            instance_actor_url,
            headers={'Accept': 'application/activity+json'}
        )
        
        if response.status_code == 200:
            actor = response.json()
            if actor.get('type') in ['Application', 'Service']:
                logger.info("✅ Instance actor present")
                return True
        elif response.status_code == 404:
            logger.info("⏭️  No instance actor (optional)")
            return True
            
        logger.error(f"❌ Instance actor test failed: {response.status_code}")
        return False
        
    def run_all_tests(self, username: str = 'aron'):
        """Run all federation tests"""
        logger.info(f"🚀 Starting Federation Validation for {self.instance_url}")
        logger.info("=" * 50)
        
        results = {
            'total': 0,
            'passed': 0,
            'failed': 0,
            'warnings': 0
        }
        
        # Test WebFinger
        results['total'] += 1
        actor_url = self.test_webfinger(username)
        if actor_url:
            results['passed'] += 1
        else:
            results['failed'] += 1
            logger.error("Cannot continue without actor URL")
            return results
            
        # Test actor endpoint
        results['total'] += 1
        actor = self.test_actor_endpoint(actor_url)
        if actor:
            results['passed'] += 1
        else:
            results['failed'] += 1
            return results
            
        # Run remaining tests
        tests = [
            self.test_outbox,
            self.test_inbox_discovery,
            self.test_http_signatures,
            self.test_collections
        ]
        
        for test in tests:
            results['total'] += 1
            try:
                if test(actor):
                    results['passed'] += 1
                else:
                    results['failed'] += 1
            except Exception as e:
                logger.error(f"❌ Test failed with error: {e}")
                results['failed'] += 1
                
        # Instance-level tests
        instance_tests = [
            self.test_nodeinfo,
            self.test_media_types,
            self.test_instance_actor
        ]
        
        for test in instance_tests:
            results['total'] += 1
            try:
                if test():
                    results['passed'] += 1
                else:
                    results['failed'] += 1
            except Exception as e:
                logger.error(f"❌ Test failed with error: {e}")
                results['failed'] += 1
                
        # Summary
        logger.info(f"\n📊 Federation Validation Summary")
        logger.info("=" * 50)
        logger.info(f"Total Tests: {results['total']}")
        logger.info(f"✅ Passed: {results['passed']}")
        logger.info(f"❌ Failed: {results['failed']}")
        
        compliance = (results['passed'] / results['total']) * 100
        logger.info(f"\n🎯 ActivityPub Compliance: {compliance:.1f}%")
        
        if compliance == 100:
            logger.info("🏆 Full ActivityPub compliance achieved!")
        elif compliance >= 90:
            logger.info("👍 High ActivityPub compliance")
        elif compliance >= 70:
            logger.info("⚠️  Moderate ActivityPub compliance")
        else:
            logger.error("❌ Low ActivityPub compliance")
            
        return results


def main():
    """Main test runner"""
    import argparse
    
    parser = argparse.ArgumentParser(description='Federation Validation for Lesser')
    parser.add_argument('instance_url', help='Instance URL to test')
    parser.add_argument('--username', default='aron', help='Username to test (default: aron)')
    
    args = parser.parse_args()
    
    # Configure logging
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(levelname)s - %(message)s'
    )
    
    validator = FederationValidator(args.instance_url)
    results = validator.run_all_tests(args.username)
    
    # Exit with appropriate code
    import sys
    sys.exit(0 if results['failed'] == 0 else 1)


if __name__ == '__main__':
    main() 
