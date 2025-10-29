#!/usr/bin/env python3
"""
Stage-aware validation script for Lesser's streaming (WebSocket) endpoint.

Environment variables:
  GRAPHQL_STAGE  -> deployment stage (default: dev)
  GRAPHQL_DOMAIN -> root domain (default: lesser.host)
  GRAPHQL_TOKEN  -> bearer token used for authenticated connections
"""

import asyncio
import json
import logging
import os
import sys
import time
from typing import List, Optional

import websockets
from websockets.client import WebSocketClientProtocol

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)

BASE_DOMAIN = os.getenv("GRAPHQL_DOMAIN", "lesser.host")
DEFAULT_STAGE = os.getenv("GRAPHQL_STAGE", "dev")


class StreamingTestClient:
    """Helper for interacting with the streaming WebSocket endpoint."""

    def __init__(self, stage: str, access_token: str, base_domain: str = BASE_DOMAIN):
        self.stage = stage
        self.base_domain = base_domain
        self.access_token = access_token
        self.instance_url = f"https://{stage}.{base_domain}"
        self.ws_url = f"wss://stream.{stage}.{base_domain}"
        self.websocket: Optional[WebSocketClientProtocol] = None

    async def connect(self) -> bool:
        """Connect to the WebSocket endpoint."""
        logger.info("Connecting to %s", self.ws_url)
        headers: List[tuple[str, str]] = []
        url = self.ws_url

        if self.access_token:
            headers.append(("Authorization", f"Bearer {self.access_token}"))
            url = f"{url}?access_token={self.access_token}"

        try:
            self.websocket = await websockets.connect(
                url,
                additional_headers=headers or None,
                ping_interval=20,
                close_timeout=5,
            )
            logger.info("Connected successfully")
            return True
        except Exception as exc:
            logger.error("Failed to connect: %s", exc)
            return False

    async def disconnect(self) -> None:
        """Disconnect from the WebSocket."""
        if self.websocket:
            await self.websocket.close()
            logger.info("Disconnected")

    async def subscribe(self, stream: str) -> bool:
        """Subscribe to a named stream."""
        if not self.websocket:
            logger.error("Not connected")
            return False

        message = {"type": "subscribe", "stream": stream}
        try:
            await self.websocket.send(json.dumps(message))
            logger.info("Sent subscribe request for stream: %s", stream)
            return True
        except Exception as exc:
            logger.error("Failed to subscribe: %s", exc)
            return False

    async def unsubscribe(self, stream: str) -> bool:
        """Unsubscribe from a stream."""
        if not self.websocket:
            logger.error("Not connected")
            return False

        message = {"type": "unsubscribe", "stream": stream}
        try:
            await self.websocket.send(json.dumps(message))
            logger.info("Sent unsubscribe request for stream: %s", stream)
            return True
        except Exception as exc:
            logger.error("Failed to unsubscribe: %s", exc)
            return False

    async def ping(self) -> bool:
        """Send a ping message."""
        if not self.websocket:
            logger.error("Not connected")
            return False

        try:
            await self.websocket.send(json.dumps({"type": "ping"}))
            logger.info("Sent ping")
            return True
        except Exception as exc:
            logger.error("Failed to send ping: %s", exc)
            return False

    async def listen_for_messages(self, duration: int = 10) -> None:
        """Listen for messages for a specified duration."""
        if not self.websocket:
            logger.error("Not connected")
            return

        logger.info("Listening for messages for %s seconds...", duration)
        start_time = time.time()
        message_count = 0

        try:
            while time.time() - start_time < duration:
                try:
                    message = await asyncio.wait_for(self.websocket.recv(), timeout=1.0)
                    data = json.loads(message)
                    message_count += 1
                    logger.info(
                        "Received message #%s type=%s stream=%s",
                        message_count,
                        data.get("type", data.get("event")),
                        data.get("stream", "N/A"),
                    )
                except asyncio.TimeoutError:
                    continue
                except websockets.exceptions.ConnectionClosed:
                    logger.warning("Connection closed by server")
                    break
        except Exception as exc:
            logger.error("Error while listening: %s", exc)

        logger.info("Received %s messages in %s seconds", message_count, duration)


async def test_streaming(stage: str, access_token: str) -> None:
    """Run a basic streaming validation flow."""
    client = StreamingTestClient(stage, access_token)

    logger.info("\n=== Test 1: Connect to WebSocket ===")
    if not await client.connect():
        logger.error("Failed to connect, aborting tests")
        return

    logger.info("\n=== Test 2: Subscribe to public timeline ===")
    await client.subscribe("public")
    await asyncio.sleep(1)

    logger.info("\n=== Test 3: Subscribe to user timeline ===")
    await client.subscribe("user")

    logger.info("\n=== Test 4: Subscribe to notifications ===")
    await client.subscribe("user:notification")

    logger.info("\n=== Test 5: Send ping ===")
    await client.ping()

    logger.info("\n=== Test 6: Listen for messages ===")
    await client.listen_for_messages(duration=10)

    logger.info("\n=== Test 7: Unsubscribe from public timeline ===")
    await client.unsubscribe("public")

    logger.info("\n=== Test 8: Disconnect ===")
    await client.disconnect()


async def test_multiple_connections(stage: str, access_token: str, num_connections: int = 3) -> None:
    """Test multiple concurrent connections."""
    logger.info("\n=== Testing %s concurrent connections ===", num_connections)
    clients: List[StreamingTestClient] = []

    for idx in range(num_connections):
        client = StreamingTestClient(stage, access_token)
        if await client.connect():
            await client.subscribe("public")
            clients.append(client)
            logger.info("Client %s connected and subscribed", idx + 1)
        else:
            logger.error("Client %s failed to connect", idx + 1)

    if clients:
        logger.info("All %s clients listening...", len(clients))
        await asyncio.sleep(5)

    for idx, client in enumerate(clients):
        await client.disconnect()
        logger.info("Client %s disconnected", idx + 1)


def parse_args() -> tuple[str, str]:
    """Parse CLI arguments for stage and token."""
    stage = DEFAULT_STAGE
    token = os.getenv("GRAPHQL_TOKEN")

    if len(sys.argv) == 2:
        stage = sys.argv[1]
    elif len(sys.argv) == 3:
        stage = sys.argv[1]
        token = sys.argv[2]
    elif len(sys.argv) > 3:
        print("Usage: python test_streaming.py [stage] [access_token]")
        print("Example: python test_streaming.py dev <access_token>")
        sys.exit(1)

    if not token:
        print("Error: access token is required (pass as argument or set GRAPHQL_TOKEN).")
        sys.exit(1)

    return stage, token


def main() -> None:
    stage, access_token = parse_args()

    logger.info("Starting WebSocket streaming tests")
    logger.info("Stage: %s  Domain: %s", stage, BASE_DOMAIN)

    asyncio.run(test_streaming(stage, access_token))
    asyncio.run(test_multiple_connections(stage, access_token))

    logger.info("\nAll tests completed!")


if __name__ == "__main__":
    main()
