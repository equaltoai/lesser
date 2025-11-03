#!/usr/bin/env python3
"""
Lesser Test Data Generator
Generates realistic ActivityPub test data for development and testing
"""

import json
import random
import uuid
import requests
from datetime import datetime, timedelta, timezone
from typing import List, Dict, Any, Optional
import sys

# Test account password (test-only; do NOT use in production)
TEST_ACCOUNT_PASSWORD = "Test123!@#"

try:
    from faker import Faker

    fake = Faker()
except ImportError:
    print("Please install faker: python -m pip install faker")
    sys.exit(1)


class LesserTestDataGenerator:
    """Generate realistic test data for Lesser ActivityPub implementation"""

    def __init__(
        self,
        instance_url: str = "https://lesser.example.com",
        auth_token: Optional[str] = None,
    ):
        self.instance_url = instance_url.rstrip("/")
        self.auth_token = auth_token
        self.headers = {}
        if auth_token:
            self.headers["Authorization"] = f"Bearer {auth_token}"

        # Track generated data for relationships
        self.actors = []
        self.objects = []
        self.activities = []

    def generate_actor(self, actor_type: str = "Person") -> Dict[str, Any]:
        """Generate a realistic ActivityPub actor"""
        username = fake.user_name().lower()
        display_name = fake.name()

        actor = {
            "id": f"{self.instance_url}/users/{username}",
            "type": actor_type,
            "preferredUsername": username,
            "name": display_name,
            "summary": fake.text(max_nb_chars=500),
            "inbox": f"{self.instance_url}/users/{username}/inbox",
            "outbox": f"{self.instance_url}/users/{username}/outbox",
            "followers": f"{self.instance_url}/users/{username}/followers",
            "following": f"{self.instance_url}/users/{username}/following",
            "liked": f"{self.instance_url}/users/{username}/liked",
            "publicKey": {
                "id": f"{self.instance_url}/users/{username}#main-key",
                "owner": f"{self.instance_url}/users/{username}",
                "publicKeyPem": self._generate_fake_public_key(),
            },
            "icon": {
                "type": "Image",
                "mediaType": "image/png",
                "url": f"https://picsum.photos/seed/{username}/400/400",
            },
            "image": {
                "type": "Image",
                "mediaType": "image/jpeg",
                "url": f"https://picsum.photos/seed/{username}-banner/1500/500",
            },
            "manuallyApprovesFollowers": random.choice([True, False]),
            "discoverable": random.choice([True, False]),
            "published": fake.date_time_between(
                start_date="-2y", end_date="now"
            ).isoformat()
            + "Z",
            "url": f"{self.instance_url}/@{username}",
            "endpoints": {"sharedInbox": f"{self.instance_url}/inbox"},
        }

        # Add optional fields randomly
        if random.random() > 0.5:
            actor["location"] = fake.city() + ", " + fake.country()

        if random.random() > 0.7:
            actor["attachment"] = [
                {
                    "type": "PropertyValue",
                    "name": "Website",
                    "value": f'<a href="{fake.url()}" rel="me nofollow noopener noreferrer" target="_blank">{fake.domain_name()}</a>',
                },
                {
                    "type": "PropertyValue",
                    "name": "Pronouns",
                    "value": random.choice(
                        ["he/him", "she/her", "they/them", "ze/zir"]
                    ),
                },
            ]

        self.actors.append(actor)
        return actor

    def generate_note(
        self,
        author: Optional[Dict[str, Any]] = None,
        in_reply_to: Optional[str] = None,
        content_length: str = "medium",
    ) -> Dict[str, Any]:
        """Generate a realistic Note object"""
        if not author and self.actors:
            author = random.choice(self.actors)
        elif not author:
            author = self.generate_actor()

        # Generate content based on length
        if content_length == "short":
            content = fake.sentence(nb_words=random.randint(5, 15))
        elif content_length == "long":
            content = fake.text(max_nb_chars=2000)
        else:  # medium
            content = fake.text(max_nb_chars=500)

        # Add hashtags randomly
        if random.random() > 0.6:
            hashtags = [f"#{fake.word()}" for _ in range(random.randint(1, 5))]
            content += "\n\n" + " ".join(hashtags)

        # Add mentions randomly
        mentions = []
        if random.random() > 0.7 and self.actors:
            mentioned = random.sample(self.actors, min(3, len(self.actors)))
            for actor in mentioned:
                username = actor["preferredUsername"]
                content = f"@{username} " + content
                mentions.append(
                    {"type": "Mention", "href": actor["id"], "name": f"@{username}"}
                )

        note_id = f"{self.instance_url}/objects/{uuid.uuid4()}"
        published = fake.date_time_between(start_date="-30d", end_date="now")

        note = {
            "id": note_id,
            "type": "Note",
            "published": published.isoformat() + "Z",
            "attributedTo": author["id"],
            "content": f"<p>{content}</p>",
            "to": ["https://www.w3.org/ns/activitystreams#Public"],
            "cc": [author["followers"]],
            "sensitive": (
                random.choice([True, False]) if random.random() > 0.9 else False
            ),
            "summary": fake.sentence() if random.random() > 0.95 else None,
            "inReplyTo": in_reply_to,
            "url": f"{self.instance_url}/@{author['preferredUsername']}/{note_id.split('/')[-1]}",
            "tag": mentions,
        }

        # Add media attachments randomly
        if random.random() > 0.6:
            attachments = []
            for i in range(random.randint(1, 4)):
                attachment = {
                    "type": "Document",
                    "mediaType": random.choice(
                        ["image/jpeg", "image/png", "image/gif", "video/mp4"]
                    ),
                    "url": f"https://picsum.photos/seed/{note_id}-{i}/800/600",
                    "name": fake.sentence(nb_words=5),
                }
                attachments.append(attachment)
            note["attachment"] = attachments

        # Add location randomly
        if random.random() > 0.8:
            note["location"] = {
                "type": "Place",
                "name": fake.city(),
                "latitude": float(fake.latitude()),
                "longitude": float(fake.longitude()),
            }

        self.objects.append(note)
        return note

    def generate_activity(
        self,
        activity_type: str = "Create",
        actor: Optional[Dict[str, Any]] = None,
        object: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """Generate an activity"""
        if not actor and self.actors:
            actor = random.choice(self.actors)
        elif not actor:
            actor = self.generate_actor()

        activity_id = f"{self.instance_url}/activities/{uuid.uuid4()}"
        published = fake.date_time_between(start_date="-7d", end_date="now")

        activity = {
            "id": activity_id,
            "type": activity_type,
            "actor": actor["id"],
            "published": published.isoformat() + "Z",
            "to": ["https://www.w3.org/ns/activitystreams#Public"],
            "cc": [actor["followers"]],
        }

        # Handle different activity types
        if activity_type == "Create":
            if not object:
                object = self.generate_note(author=actor)
            activity["object"] = object

        elif activity_type == "Like":
            if not object and self.objects:
                object = random.choice(self.objects)
            elif not object:
                object = self.generate_note()
            activity["object"] = object["id"] if isinstance(object, dict) else object

        elif activity_type == "Announce":
            if not object and self.objects:
                object = random.choice(self.objects)
            elif not object:
                object = self.generate_note()
            activity["object"] = object["id"] if isinstance(object, dict) else object

        elif activity_type == "Follow":
            if not object and self.actors:
                object = random.choice(
                    [a for a in self.actors if a["id"] != actor["id"]]
                )
            elif not object:
                object = self.generate_actor()
            activity["object"] = object["id"] if isinstance(object, dict) else object

        elif activity_type == "Delete":
            if not object and self.objects:
                object = random.choice(self.objects)
            activity["object"] = {
                "id": object["id"] if isinstance(object, dict) else object,
                "type": "Tombstone",
            }

        elif activity_type == "Update":
            if not object and self.actors:
                object = random.choice(self.actors)
                # Modify some fields
                object["summary"] = fake.text(max_nb_chars=500)
                object["name"] = fake.name()
            activity["object"] = object

        self.activities.append(activity)
        return activity

    def generate_conversation_thread(
        self, depth: int = 5, participants: int = 3
    ) -> List[Dict[str, Any]]:
        """Generate a realistic conversation thread"""
        # Create participants
        thread_actors = [self.generate_actor() for _ in range(participants)]

        # Create initial post
        original_post = self.generate_note(author=thread_actors[0])
        thread = [original_post]

        current_parent = original_post
        for i in range(depth - 1):
            # Pick a random participant to reply
            replier = random.choice(thread_actors)
            reply = self.generate_note(author=replier, in_reply_to=current_parent["id"])
            thread.append(reply)

            # Sometimes branch the conversation
            if random.random() > 0.7 and i < depth - 2:
                # Another participant replies to the same parent
                side_replier = random.choice([a for a in thread_actors if a != replier])
                side_reply = self.generate_note(
                    author=side_replier, in_reply_to=current_parent["id"]
                )
                thread.append(side_reply)

            current_parent = reply

        return thread

    def generate_follow_network(
        self, actors_count: int = 10, follow_probability: float = 0.3
    ) -> Dict[str, Any]:
        """Generate a network of actors with follow relationships"""
        # Generate actors if needed
        while len(self.actors) < actors_count:
            self.generate_actor()

        network_actors = self.actors[:actors_count]
        follow_activities = []

        # Generate follow relationships
        for actor in network_actors:
            for potential_followee in network_actors:
                if (
                    actor["id"] != potential_followee["id"]
                    and random.random() < follow_probability
                ):
                    follow = self.generate_activity(
                        activity_type="Follow", actor=actor, object=potential_followee
                    )
                    follow_activities.append(follow)

        return {
            "actors": network_actors,
            "follow_activities": follow_activities,
            "stats": {
                "total_actors": len(network_actors),
                "total_follows": len(follow_activities),
                "average_following": len(follow_activities) / len(network_actors),
            },
        }

    def generate_timeline_data(
        self, days: int = 7, posts_per_day: int = 20
    ) -> List[Dict[str, Any]]:
        """Generate timeline data with realistic posting patterns"""
        timeline = []

        # Ensure we have enough actors
        if len(self.actors) < 10:
            for _ in range(10 - len(self.actors)):
                self.generate_actor()

        now = datetime.now(timezone.utc)

        for day in range(days):
            day_start = now - timedelta(days=day)

            # Vary posts throughout the day (peak hours)
            for hour in range(24):
                hour_start = day_start.replace(hour=hour, minute=0, second=0)

                # More posts during peak hours (9-11am, 2-4pm, 7-9pm)
                if hour in [9, 10, 11, 14, 15, 16, 19, 20, 21]:
                    post_count = random.randint(2, 5)
                elif hour in range(1, 6):  # Fewer posts at night
                    post_count = random.randint(0, 1)
                else:
                    post_count = random.randint(1, 3)

                for _ in range(post_count):
                    actor = random.choice(self.actors)

                    # Vary content types
                    content_type = random.choices(
                        ["note", "reply", "share", "media"],
                        weights=[0.5, 0.2, 0.2, 0.1],
                    )[0]

                    if content_type == "note":
                        note = self.generate_note(author=actor)
                        activity = self.generate_activity("Create", actor, note)
                    elif content_type == "reply" and self.objects:
                        parent = random.choice(self.objects)
                        note = self.generate_note(
                            author=actor, in_reply_to=parent["id"]
                        )
                        activity = self.generate_activity("Create", actor, note)
                    elif content_type == "share" and self.objects:
                        activity = self.generate_activity("Announce", actor)
                    else:  # media
                        note = self.generate_note(author=actor)
                        # Ensure it has media
                        if "attachment" not in note:
                            note["attachment"] = [
                                {
                                    "type": "Document",
                                    "mediaType": "image/jpeg",
                                    "url": f"https://picsum.photos/seed/{uuid.uuid4()}/800/600",
                                    "name": fake.sentence(nb_words=5),
                                }
                            ]
                        activity = self.generate_activity("Create", actor, note)

                    # Override the published time
                    minutes = random.randint(0, 59)
                    seconds = random.randint(0, 59)
                    activity["published"] = (
                        hour_start.replace(minute=minutes, second=seconds).isoformat()
                        + "Z"
                    )

                    timeline.append(activity)

        # Sort by published date (newest first)
        timeline.sort(key=lambda x: x["published"], reverse=True)
        return timeline

    def _generate_fake_public_key(self) -> str:
        """Generate a fake RSA public key for testing"""
        # This is a dummy public key for testing only
        return """-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA0Rdj53hR4AdsiRcqt1Zd
oGRwKRQHtpLRBgDKDoQHn/6hodZrH5I8fDVjiCfpJ8qKMgV5MZEHgNFPqiJLwKdt
twxPEAQZMmVGBMKvndGOEAG+LZzJFKkEdLxUaJy/9gVnnfvdIrmMFUw1VPBxAAuK
IjhQKEMLVC1hfFji7JpBYdWrZKpDYdBH2gKGGUt4kGDUgfBR3wvRz0nBEZfBLyvz
ZKqFWCsFmIdIIR3dDtJQ6wmKlPc7nCCqPhDNCQeYDaIBmg0P4UKaLhEkEjmAGLT8
TAITOkZNaLV9vjJPclVBvJhLlixn9sDVwoBMkiVwmxNvNRHXa0n5w8As1YLjSXf6
0wIDAQAB
-----END PUBLIC KEY-----"""

    def create_test_account(
        self, username: str, email: str, password: str
    ) -> Dict[str, Any]:
        """Create a test account via the API"""
        if self.instance_url.startswith("http://"):
            print(f"WARNING: Instance URL uses insecure HTTP: {self.instance_url}")
            print(
                "Account creation over HTTP is not recommended and has been skipped for security reasons."
            )
            return {
                "username": username,
                "email": email,
                "status": "skipped",
                "reason": "Insecure HTTP endpoint - use HTTPS",
            }
        if not self.instance_url.startswith("https://"):
            print(
                f"Skipping API call - instance URL not a valid HTTPS endpoint: {self.instance_url}"
            )
            return {
                "username": username,
                "email": email,
                "status": "skipped",
                "reason": "Not a valid HTTPS endpoint",
            }

        try:
            response = requests.post(
                f"{self.instance_url}/api/v1/accounts",
                json={
                    "username": username,
                    "email": email,
                    "password": password,
                    "agreement": True,
                    "locale": "en",
                },
            )
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            print(f"Failed to create account: {e}")
            return None

    def populate_instance(
        self,
        accounts: int = 5,
        posts_per_account: int = 10,
        follow_probability: float = 0.3,
    ) -> Dict[str, Any]:
        """Populate an instance with test data"""
        results = {
            "accounts_created": 0,
            "posts_created": 0,
            "follows_created": 0,
            "errors": [],
        }

        # Create accounts
        for i in range(accounts):
            username = f"testuser{i}_{fake.user_name().lower()}"
            email = f"{username}@example.com"
            password = TEST_ACCOUNT_PASSWORD

            account = self.create_test_account(username, email, password)
            if account and account.get("status") != "skipped":
                results["accounts_created"] += 1

                # Generate posts for this account
                actor = self.generate_actor()
                actor["id"] = f"{self.instance_url}/users/{username}"
                actor["preferredUsername"] = username

                for _ in range(posts_per_account):
                    self.generate_note(author=actor)
                    # In a real implementation, you'd POST this to the API
                    results["posts_created"] += 1
            elif account and account.get("status") == "skipped":
                results["errors"].append(f"Skipped {username}: {account.get('reason')}")

        return results

    def export_test_data(self, filename: str = "test_data.json") -> None:
        """Export generated test data to a JSON file"""
        data = {
            "actors": self.actors,
            "objects": self.objects,
            "activities": self.activities,
            "metadata": {
                "generated_at": datetime.now(timezone.utc).isoformat() + "Z",
                "instance_url": self.instance_url,
                "counts": {
                    "actors": len(self.actors),
                    "objects": len(self.objects),
                    "activities": len(self.activities),
                },
            },
        }

        with open(filename, "w") as f:
            json.dump(data, f, indent=2)

        print(f"Test data exported to {filename}")


def main():
    """Example usage of the test data generator"""
    generator = LesserTestDataGenerator("https://lesser.example.com")

    print("Generating test data...")

    # Generate a variety of actors
    print("\n1. Generating actors...")
    for i in range(10):
        actor = generator.generate_actor()
        print(f"  - Created actor: @{actor['preferredUsername']}")

    # Generate conversation threads
    print("\n2. Generating conversation threads...")
    for i in range(3):
        thread = generator.generate_conversation_thread(depth=5, participants=3)
        print(f"  - Created thread with {len(thread)} posts")

    # Generate follow network
    print("\n3. Generating follow network...")
    network = generator.generate_follow_network(actors_count=10, follow_probability=0.3)
    print(
        f"  - Created network with {network['stats']['total_follows']} follow relationships"
    )

    # Generate timeline data
    print("\n4. Generating timeline data...")
    timeline = generator.generate_timeline_data(days=7, posts_per_day=20)
    print(f"  - Created {len(timeline)} timeline activities")

    # Export data
    print("\n5. Exporting test data...")
    generator.export_test_data("lesser_test_data.json")

    print("\nTest data generation complete!")
    print(f"\nSummary:")
    print(f"  - Total actors: {len(generator.actors)}")
    print(f"  - Total objects: {len(generator.objects)}")
    print(f"  - Total activities: {len(generator.activities)}")


if __name__ == "__main__":
    main()
