#!/usr/bin/env python3
"""Test script for Lesser WebSocket streaming functionality"""

import asyncio
import json
import logging
import sys
import time
from datetime import datetime
from typing import Optional, Dict, Any

import websockets
from websockets.client import WebSocketClientProtocol

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class StreamingTestClient:
    def __init__(self, instance_url: str, access_token: str):
        self.instance_url = instance_url.rstrip('/')
        self.access_token = access_token
        # Extract domain from instance URL and construct WebSocket URL
        domain = instance_url.replace('https://', '').replace('http://', '').split('/')[0]
        self.ws_url = f"wss://ws.{domain}"
        self.websocket: Optional[WebSocketClientProtocol] = None
        
    async def connect(self):
        """Connect to the WebSocket endpoint"""
        logger.info(f"Connecting to {self.ws_url}")
        
        # Add access token as query parameter
        url_with_token = f"{self.ws_url}?access_token={self.access_token}"
        
        try:
            self.websocket = await websockets.connect(url_with_token)
            logger.info("Connected successfully")
            return True
        except Exception as e:
            logger.error(f"Failed to connect: {e}")
            return False
    
    async def disconnect(self):
        """Disconnect from the WebSocket"""
        if self.websocket:
            await self.websocket.close()
            logger.info("Disconnected")
    
    async def subscribe(self, stream: str):
        """Subscribe to a stream"""
        if not self.websocket:
            logger.error("Not connected")
            return False
        
        message = {
            "type": "subscribe",
            "stream": stream
        }
        
        try:
            await self.websocket.send(json.dumps(message))
            logger.info(f"Sent subscribe request for stream: {stream}")
            return True
        except Exception as e:
            logger.error(f"Failed to subscribe: {e}")
            return False
    
    async def unsubscribe(self, stream: str):
        """Unsubscribe from a stream"""
        if not self.websocket:
            logger.error("Not connected")
            return False
        
        message = {
            "type": "unsubscribe",
            "stream": stream
        }
        
        try:
            await self.websocket.send(json.dumps(message))
            logger.info(f"Sent unsubscribe request for stream: {stream}")
            return True
        except Exception as e:
            logger.error(f"Failed to unsubscribe: {e}")
            return False
    
    async def ping(self):
        """Send a ping message"""
        if not self.websocket:
            logger.error("Not connected")
            return False
        
        message = {"type": "ping"}
        
        try:
            await self.websocket.send(json.dumps(message))
            logger.info("Sent ping")
            return True
        except Exception as e:
            logger.error(f"Failed to send ping: {e}")
            return False
    
    async def listen_for_messages(self, duration: int = 60):
        """Listen for messages for a specified duration"""
        if not self.websocket:
            logger.error("Not connected")
            return
        
        logger.info(f"Listening for messages for {duration} seconds...")
        start_time = time.time()
        message_count = 0
        
        try:
            while time.time() - start_time < duration:
                # Set a timeout so we can check the duration periodically
                try:
                    message = await asyncio.wait_for(
                        self.websocket.recv(), 
                        timeout=1.0
                    )
                    
                    data = json.loads(message)
                    message_count += 1
                    
                    logger.info(f"Received message #{message_count}:")
                    logger.info(f"  Type: {data.get('type', data.get('event'))}")
                    logger.info(f"  Stream: {data.get('stream', 'N/A')}")
                    
                    if 'payload' in data:
                        logger.info(f"  Payload: {json.dumps(data['payload'], indent=2)}")
                    
                except asyncio.TimeoutError:
                    # No message received in 1 second, continue
                    continue
                except websockets.exceptions.ConnectionClosed:
                    logger.warning("Connection closed by server")
                    break
                
        except Exception as e:
            logger.error(f"Error while listening: {e}")
        
        logger.info(f"Received {message_count} messages in {duration} seconds")


async def test_streaming(instance_url: str, access_token: str):
    """Test the streaming functionality"""
    client = StreamingTestClient(instance_url, access_token)
    
    # Test 1: Connect to WebSocket
    logger.info("\n=== Test 1: Connect to WebSocket ===")
    if not await client.connect():
        logger.error("Failed to connect, aborting tests")
        return
    
    # Test 2: Subscribe to public timeline
    logger.info("\n=== Test 2: Subscribe to public timeline ===")
    await client.subscribe("public")
    
    # Wait for subscription confirmation
    await asyncio.sleep(2)
    
    # Test 3: Subscribe to user timeline
    logger.info("\n=== Test 3: Subscribe to user timeline ===")
    await client.subscribe("user")
    
    # Test 4: Subscribe to notifications
    logger.info("\n=== Test 4: Subscribe to notifications ===")
    await client.subscribe("user:notification")
    
    # Test 5: Send ping
    logger.info("\n=== Test 5: Send ping ===")
    await client.ping()
    
    # Test 6: Listen for messages
    logger.info("\n=== Test 6: Listen for messages ===")
    logger.info("Now post something to generate events...")
    await client.listen_for_messages(duration=30)
    
    # Test 7: Unsubscribe from public timeline
    logger.info("\n=== Test 7: Unsubscribe from public timeline ===")
    await client.unsubscribe("public")
    
    # Test 8: Disconnect
    logger.info("\n=== Test 8: Disconnect ===")
    await client.disconnect()


async def test_multiple_connections(instance_url: str, access_token: str, num_connections: int = 5):
    """Test multiple concurrent connections"""
    logger.info(f"\n=== Testing {num_connections} concurrent connections ===")
    
    clients = []
    
    # Create and connect multiple clients
    for i in range(num_connections):
        client = StreamingTestClient(instance_url, access_token)
        if await client.connect():
            await client.subscribe("public")
            clients.append(client)
            logger.info(f"Client {i+1} connected and subscribed")
        else:
            logger.error(f"Client {i+1} failed to connect")
    
    # Let them listen for a bit
    logger.info(f"All {len(clients)} clients listening...")
    await asyncio.sleep(10)
    
    # Disconnect all
    for i, client in enumerate(clients):
        await client.disconnect()
        logger.info(f"Client {i+1} disconnected")


def main():
    if len(sys.argv) != 3:
        print("Usage: python test_streaming.py <instance_url> <access_token>")
        print("Example: python test_streaming.py https://lesser.example.com your_access_token")
        sys.exit(1)
    
    instance_url = sys.argv[1]
    access_token = sys.argv[2]
    
    logger.info("Starting WebSocket streaming tests")
    logger.info(f"Instance: {instance_url}")
    
    # Run basic tests
    asyncio.run(test_streaming(instance_url, access_token))
    
    # Run concurrent connection test
    asyncio.run(test_multiple_connections(instance_url, access_token))
    
    logger.info("\nAll tests completed!")


if __name__ == "__main__":
    main() 