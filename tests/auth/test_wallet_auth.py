#!/usr/bin/env python3
"""
Test script for wallet authentication in Lesser.

Requirements:
    pip install eth-account requests
"""

import requests
from eth_account import Account
from eth_account.messages import encode_defunct

# Configuration
API_BASE_URL = "http://localhost:8000/dev/api/v1"
AUTH_API_URL = "http://localhost:9000/dev"

# Create a test wallet
account = Account.create()
print(f"Test wallet address: {account.address}")
print(f"Test wallet private key: {account.key.hex()}")
print()

def test_wallet_auth():
    """Test wallet authentication flow"""
    
    # Step 1: Create authentication challenge
    print("Step 1: Creating authentication challenge...")
    challenge_response = requests.post(
        f"{AUTH_API_URL}/auth/wallet/challenge",
        json={
            "address": account.address,
            "chainId": 1  # Ethereum mainnet
        }
    )
    
    if challenge_response.status_code != 200:
        print(f"Failed to create challenge: {challenge_response.text}")
        return None
    
    challenge = challenge_response.json()
    print(f"Challenge created: {challenge['id']}")
    print(f"Message to sign: {challenge['message']}")
    print()
    
    # Step 2: Sign the message
    print("Step 2: Signing message with wallet...")
    message = encode_defunct(text=challenge['message'])
    signed_message = account.sign_message(message)
    signature = signed_message.signature.hex()
    print(f"Signature: {signature}")
    print()
    
    # Step 3: Verify signature and authenticate
    print("Step 3: Verifying signature...")
    verify_response = requests.post(
        f"{AUTH_API_URL}/auth/wallet/verify",
        json={
            "challengeId": challenge['id'],
            "address": account.address,
            "signature": signature,
            "message": challenge['message']
        }
    )
    
    if verify_response.status_code != 200:
        print(f"Failed to verify signature: {verify_response.text}")
        return None
    
    verify_result = verify_response.json()
    
    if verify_result.get('authenticated') is False:
        print("Wallet not linked to any account")
        print("Please create an account first and link this wallet")
        return None
    
    print("Authentication successful!")
    print(f"Access token: {verify_result['access_token']}")
    print(f"Username: {verify_result.get('me', 'Unknown')}")
    
    return verify_result['access_token']

def test_wallet_linking(auth_token):
    """Test linking a wallet to an existing account"""
    
    # Create a new wallet to link
    new_wallet = Account.create()
    print(f"\nCreating new wallet to link: {new_wallet.address}")
    
    # Step 1: Create challenge for linking
    print("\nStep 1: Creating challenge for wallet linking...")
    challenge_response = requests.post(
        f"{AUTH_API_URL}/auth/wallet/challenge",
        headers={"Authorization": f"Bearer {auth_token}"},
        json={
            "address": new_wallet.address,
            "chainId": 1
        }
    )
    
    if challenge_response.status_code != 200:
        print(f"Failed to create challenge: {challenge_response.text}")
        return None
    
    challenge = challenge_response.json()
    
    # Step 2: Sign the message with new wallet
    print("Step 2: Signing message with new wallet...")
    message = encode_defunct(text=challenge['message'])
    signed_message = new_wallet.sign_message(message)
    signature = signed_message.signature.hex()
    
    # Step 3: Link the wallet
    print("Step 3: Linking wallet to account...")
    link_response = requests.post(
        f"{AUTH_API_URL}/auth/wallet/link",
        headers={"Authorization": f"Bearer {auth_token}"},
        json={
            "address": new_wallet.address,
            "chainId": 1,
            "walletType": "ethereum",
            "challengeId": challenge['id'],
            "signature": signature,
            "message": challenge['message']
        }
    )
    
    if link_response.status_code != 200:
        print(f"Failed to link wallet: {link_response.text}")
        return None
    
    print("Wallet linked successfully!")
    
    # Step 4: List wallets
    print("\nStep 4: Listing user's wallets...")
    list_response = requests.get(
        f"{AUTH_API_URL}/auth/wallet/list",
        headers={"Authorization": f"Bearer {auth_token}"}
    )
    
    if list_response.status_code == 200:
        wallets_data = list_response.json()
        print(f"User has {wallets_data['count']} wallets:")
        for wallet in wallets_data['wallets']:
            print(f"  - {wallet['address']} (type: {wallet['type']}, linked: {wallet['linkedAt']})")
    
    return new_wallet.address

def test_wallet_unlink(auth_token, wallet_address):
    """Test unlinking a wallet"""
    
    print(f"\nUnlinking wallet {wallet_address}...")
    unlink_response = requests.delete(
        f"{AUTH_API_URL}/auth/wallet/unlink/{wallet_address}",
        headers={"Authorization": f"Bearer {auth_token}"}
    )
    
    if unlink_response.status_code != 200:
        print(f"Failed to unlink wallet: {unlink_response.text}")
        return
    
    print("Wallet unlinked successfully!")

def create_test_account():
    """Create a test account for wallet linking"""
    print("Creating test account...")
    
    # First register a user
    register_response = requests.post(
        f"{API_BASE_URL}/auth/register",
        json={
            "username": f"wallet_test_{account.address[:8].lower()}",
            "email": f"wallet_test_{account.address[:8].lower()}@example.com",
            "password": "testpassword123",
            "agreement": True,
            "locale": "en"
        }
    )
    
    if register_response.status_code != 200:
        print(f"Failed to register: {register_response.text}")
        return None
    
    print("Account created successfully!")
    return register_response.json()['access_token']

if __name__ == "__main__":
    print("=== Lesser Wallet Authentication Test ===\n")
    
    # Try wallet auth (will fail if wallet not linked)
    auth_token = test_wallet_auth()
    
    if not auth_token:
        print("\nCreating account to link wallet...")
        auth_token = create_test_account()
        
        if auth_token:
            # Now link the original wallet
            print("\nLinking original wallet to new account...")
            
            # Create challenge
            challenge_response = requests.post(
                f"{AUTH_API_URL}/auth/wallet/challenge",
                headers={"Authorization": f"Bearer {auth_token}"},
                json={
                    "address": account.address,
                    "chainId": 1
                }
            )
            
            if challenge_response.status_code == 200:
                challenge = challenge_response.json()
                
                # Sign and link
                message = encode_defunct(text=challenge['message'])
                signed_message = account.sign_message(message)
                
                link_response = requests.post(
                    f"{AUTH_API_URL}/auth/wallet/link",
                    headers={"Authorization": f"Bearer {auth_token}"},
                    json={
                        "address": account.address,
                        "chainId": 1,
                        "walletType": "ethereum",
                        "challengeId": challenge['id'],
                        "signature": signed_message.signature.hex(),
                        "message": challenge['message']
                    }
                )
                
                if link_response.status_code == 200:
                    print("Original wallet linked successfully!")
                    print("\nNow you can authenticate with your wallet:")
                    auth_token = test_wallet_auth()
    
    if auth_token:
        # Test wallet management features
        linked_wallet = test_wallet_linking(auth_token)
        
        if linked_wallet:
            test_wallet_unlink(auth_token, linked_wallet)
    
    print("\n=== Test Complete ===") 
