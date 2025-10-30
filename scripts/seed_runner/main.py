import sys
import json
import os
import subprocess
import time
from pathlib import Path
from typing import Dict, Optional, Tuple, List

import jwt
import requests


BASE_URL = os.environ.get("LESSER_BASE_URL", "https://dev.lesser.host")
GRAPHQL_ENDPOINT = os.environ.get("LESSER_GRAPHQL_ENDPOINT", f"{BASE_URL}/api/graphql")
# Allow overriding via env; default to root only unless specified
REST_PREFIXES = [
    prefix.strip()
    for prefix in os.environ.get("LESSER_REST_PREFIXES", "").split(",")
    if prefix.strip() != ""
]
if not REST_PREFIXES:
    REST_PREFIXES = [""]
REQUEST_TIMEOUT = float(os.environ.get("LESSER_REQUEST_TIMEOUT", "15"))
SESSION = requests.Session()

BOOTSTRAP_ROOT = Path(__file__).resolve().parents[2]
JWT_SECRET_CACHE: Optional[str] = None


def main() -> None:
    if len(sys.argv) > 1 and sys.argv[1] == "get_token":
        token = build_admin_token()
        if token:
            print(token)
        return

    print("Starting API-driven bootstrap process...")

    bootstrap_dirs = sorted(
        [
            p
            for p in BOOTSTRAP_ROOT.iterdir()
            if p.is_dir() and p.name.startswith("bootstrap_")
        ]
    )
    if not bootstrap_dirs:
        print("No bootstrap_* directories found. Nothing to seed.")
        return

    admin_dir = next(
        (d for d in bootstrap_dirs if d.name.startswith("bootstrap_admin")), None
    )
    if admin_dir is None:
        print("Unable to find admin bootstrap directory. Aborting.")
        sys.exit(1)

    admin_token = build_admin_token(admin_dir)
    if not admin_token:
        print("Could not mint admin token. Aborting.")
        sys.exit(1)

    errors: List[str] = []

    for directory in bootstrap_dirs:
        try:
            process_bootstrap_directory(directory, admin_token)
        except Exception as exc:  # pylint: disable=broad-except
            message = f"[error] {directory.name}: {exc}"
            print(message)
            errors.append(message)
        else:
            # Space out requests to avoid API Gateway / Lambda throttling during bootstrap bursts.
            time.sleep(float(os.environ.get("LESSER_BOOTSTRAP_SLEEP", "2")))

    print("Bootstrap process complete.")
    if errors:
        print("\nEncountered issues during bootstrap:")
        for err in errors:
            print(f"  - {err}")
        sys.exit(1)


def build_admin_token(admin_dir: Optional[Path] = None) -> Optional[str]:
    """
    Mint a Bearer token for the admin persona using the shared JWT secret.
    """
    secret = get_jwt_secret()
    if not secret:
        return None

    client_id = "4NQBEFCFIwtk9jd0r4u2Wa"
    if admin_dir:
        oauth_path = admin_dir / "oauth_client.json"
        if oauth_path.exists():
            with oauth_path.open("r", encoding="utf-8") as handle:
                oauth_data = json.load(handle)
                client_id = oauth_data.get("ClientID", {}).get("S", client_id)

    return generate_token(
        secret, "admin", client_id, scopes=["read", "write", "follow", "push", "admin"]
    )


def get_jwt_secret() -> Optional[str]:
    """
    Retrieve the JWT secret from AWS Secrets Manager (cached for repeated use).
    """
    global JWT_SECRET_CACHE  # pylint: disable=global-statement
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

    except (subprocess.CalledProcessError, FileNotFoundError) as exc:
        print(f"Failed to retrieve JWT secret: {exc}")
        JWT_SECRET_CACHE = None
    return JWT_SECRET_CACHE


def generate_token(
    secret: str, username: str, client_id: str, scopes: Optional[List[str]] = None
) -> str:
    now = int(time.time())
    payload = {
        "sub": username,
        "iat": now,
        "exp": now + 3600,
        "username": username,
        "scopes": scopes or ["read", "write", "follow", "push"],
        "client_id": client_id,
    }
    token = jwt.encode(payload, secret, algorithm="HS256")
    if isinstance(token, bytes):
        token = token.decode("utf-8")
    return f"Bearer {token}"


def process_bootstrap_directory(directory: Path, admin_token: str) -> None:
    print(f"Processing directory: {directory.name}")

    user_data = load_json(
        directory / "user.json", "user.json missing; skipping directory."
    )
    oauth_data = load_json(
        directory / "oauth_client.json",
        "oauth_client.json missing; skipping directory.",
    )
    actor_data = load_json(
        directory / "actor.json", "actor.json missing; skipping directory."
    )
    if not all([user_data, oauth_data, actor_data]):
        return

    username = user_data["Username"]["S"]
    role = user_data.get("Role", {}).get("S", "user")

    # NOTE: Passwordless authentication only - no password sent to registration endpoint
    if not ensure_account_exists(username, user_data, admin_token):
        raise RuntimeError(
            f"Unable to ensure account {username} exists via REST API (registration failed)."
        )

    ensure_account_state(username, user_data, role, admin_token)

    ensure_oauth_client(oauth_data, admin_token)

    secret = get_jwt_secret()
    if secret:
        user_token = generate_token(secret, username, oauth_data["ClientID"]["S"])
        update_profile(username, actor_data, user_data, user_token)
    else:
        print("  Skipping profile update (missing JWT secret).")


def load_json(path: Path, error_message: str) -> Optional[Dict]:
    if not path.exists():
        print(f"  {error_message}")
        return None
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def extract_initial_password(directory: Path, username: str) -> str:
    """
    Extract password from bootstrap credentials.txt file.
    No fallback password - if credentials.txt is missing or invalid, registration will fail.
    This ensures standard deployments don't create accounts with default passwords.
    """
    creds_path = directory / "credentials.txt"
    if creds_path.exists():
        with creds_path.open("r", encoding="utf-8") as handle:
            for line in handle:
                if "Initial Password:" in line:
                    # Extract from "Initial Password: <password>" format
                    password = line.split("Initial Password:", 1)[1].strip()
                    if password:
                        return password

    # No default password - fail explicitly
    raise ValueError(
        f"No valid password found in {creds_path}. "
        "Bootstrap credentials must be generated before seeding. "
        "Standard deployments should not use default passwords."
    )


def ensure_account_exists(username: str, user_data: Dict, admin_token: str) -> bool:
    if user_exists(username, admin_token):
        print(f"  User {username} already exists.")
        return True

    print(f"  Creating user {username} via passwordless registration endpoint...")
    payload = {
        "username": username,
        "email": user_data["Email"]["S"],
        # No password field - passwordless authentication only (WebAuthn/crypto wallet)
        "agreement": True,
        "locale": user_data.get("Locale", {}).get("S", "en"),
        "reason": "bootstrap seeding",
    }

    max_attempts = int(os.environ.get("LESSER_REGISTRATION_RETRIES", "3"))
    delay = float(os.environ.get("LESSER_REGISTRATION_RETRY_DELAY", "2"))

    for attempt in range(1, max_attempts + 1):
        resp = rest_request(
            "POST",
            "/api/v1/accounts",
            json=payload,
            headers={"Content-Type": "application/json"},
        )
        if resp is None:
            print(
                f"    Failed to submit registration for {username} (no reachable endpoint)."
            )
            return False

        if resp.status_code in (200, 201, 202):
            print(f"    Registration accepted for {username}.")
            return True

        if resp.status_code in (409, 422) and "already" in resp.text.lower():
            print(
                f"    Registration indicates {username} already exists ({resp.status_code})."
            )
            return True

        print(
            f"    Registration failed for {username}: {resp.status_code} {resp.text}"
        )
        if resp.status_code >= 500 and attempt < max_attempts:
            time.sleep(delay)
            continue
        return False

    return False


def ensure_account_state(
    username: str, user_data: Dict, role: str, admin_token: str
) -> None:
    approved = user_data.get("Approved", {}).get("BOOL", False)
    if approved:
        approve_user(username, admin_token)

    suspended = user_data.get("Suspended", {}).get("BOOL", False)
    if suspended:
        set_account_action(username, "suspend", admin_token)
    elif approved:
        set_account_action(username, "enable", admin_token)

    if role == "moderator":
        promote_moderator(username, admin_token)
    elif role == "admin":
        # No public endpoint to promote to admin; log a reminder.
        print(
            "    Note: promote-to-admin is not exposed via API. Ensure admin role is set manually if required."
        )


def user_exists(username: str, admin_token: str) -> bool:
    headers = {"Authorization": admin_token}
    for identifier in (f"user-{username}", username):
        resp = rest_request(
            "GET", f"/api/v1/admin/accounts/{identifier}", headers=headers
        )
        if resp is not None and resp.status_code == 200:
            return True
        if resp is not None and resp.status_code == 403:
            print("    Admin token lacks permission to query admin endpoints (403).")
            break
    return False


def approve_user(username: str, admin_token: str) -> None:
    response = set_account_action(username, "approve", admin_token)
    if response is not None and response.status_code in (200, 204):
        print(f"    Approved account {username}.")


def set_account_action(
    username: str, action: str, admin_token: str
) -> Optional[requests.Response]:
    headers = {"Authorization": admin_token, "Content-Type": "application/json"}
    payload = {"type": action, "send_email_notification": False}

    for identifier in (f"user-{username}", username):
        resp = rest_request(
            "POST",
            f"/api/v1/admin/accounts/{identifier}/action",
            json=payload,
            headers=headers,
        )
        if resp is not None and resp.status_code in (200, 204):
            return resp
    return resp if "resp" in locals() else None


def promote_moderator(username: str, admin_token: str) -> None:
    headers = {"Authorization": admin_token, "Content-Type": "application/json"}
    for identifier in (f"user-{username}", username):
        resp = rest_request(
            "POST",
            f"/api/v1/admin/moderation/reviewers/{identifier}/promote",
            headers=headers,
        )
        if resp is not None and resp.status_code == 200:
            print(f"    Promoted {username} to moderator.")
            return
    print(f"    Unable to promote {username} to moderator (endpoint returned non-200).")


def ensure_oauth_client(oauth_data: Dict, admin_token: str) -> None:
    client_id = oauth_data["ClientID"]["S"]
    client_name = oauth_data.get("Name", {}).get("S", "Bootstrap Client")

    payload = {
        "client_name": client_name,
        "redirect_uris": " ".join(uri["S"] for uri in oauth_data["RedirectURIs"]["L"]),
        "scopes": " ".join(scope["S"] for scope in oauth_data["Scopes"]["L"]),
        "website": oauth_data.get("Website", {}).get("S", ""),
    }

    print(f"  Registering OAuth client {client_id} with name '{client_name}'...")

    resp = rest_request(
        "POST",
        "/api/v1/apps",
        json=payload,
        headers={"Content-Type": "application/json"},
    )

    if resp is None:
        print(f"  Unable to reach OAuth registration endpoint for {client_id}.")
        return

    if resp.status_code in (200, 201):
        print(f"  Registered OAuth client {client_id}.")
    elif resp.status_code == 409 or resp.status_code == 422:
        print(f"  OAuth client {client_id} already exists.")
    else:
        print(
            f"  Failed to register OAuth client {client_id}: {resp.status_code} {resp.text}"
        )


def update_profile(
    username: str, actor_data: Dict, user_data: Dict, user_token: str
) -> None:
    actor = actor_data.get("Actor", {}).get("M", {})
    summary = actor.get("summary", {}).get("S", "")
    display_name = actor.get("name", {}).get("S", username)
    locked = user_data.get("Locked", {}).get("BOOL")
    discoverable = user_data.get("Discoverable", {}).get("BOOL")

    mutation = """
    mutation UpdateProfile($input: UpdateProfileInput!) {
      updateProfile(input: $input) {
        id
      }
    }
    """
    variables = {
        "input": {
            "displayName": display_name,
            "bio": summary,
        }
    }
    if locked is not None:
        variables["input"]["locked"] = locked
    if discoverable is not None:
        variables["input"]["discoverable"] = discoverable

    max_attempts = int(os.environ.get("LESSER_PROFILE_RETRIES", "3"))
    delay = float(os.environ.get("LESSER_PROFILE_RETRY_DELAY", "2"))

    for attempt in range(1, max_attempts + 1):
        resp = SESSION.post(
            GRAPHQL_ENDPOINT,
            headers={"Authorization": user_token, "Content-Type": "application/json"},
            json={"query": mutation, "variables": variables},
            timeout=REQUEST_TIMEOUT,
        )

        try:
            payload = resp.json()
        except ValueError:
            payload = {"errors": [{"message": resp.text}]}

        if resp.status_code == 200 and "errors" not in payload:
            print(f"  Updated profile for {username}.")
            return

        print(
            f"  Failed to update profile for {username}: {resp.status_code} {payload}"
        )
        if resp.status_code >= 500 and attempt < max_attempts:
            time.sleep(delay)



def rest_request(method: str, path: str, **kwargs) -> Optional[requests.Response]:
    last_response: Optional[requests.Response] = None
    last_error: Optional[Exception] = None

    for prefix in REST_PREFIXES:
        trimmed_prefix = prefix.rstrip("/")
        url = f"{BASE_URL}{trimmed_prefix}{path}"
        try:
            response = SESSION.request(method, url, timeout=REQUEST_TIMEOUT, **kwargs)
            response.seed_url = url  # type: ignore[attr-defined]
            last_response = response
            # If route exists (non-404) or this is our final attempt, return.
            if response.status_code != 404 or prefix == REST_PREFIXES[-1]:
                return response
        except requests.RequestException as exc:
            last_error = exc
            continue

    if last_response is not None:
        return last_response
    if last_error:
        print(f"  Request {method} {path} failed: {last_error}")
    return None


if __name__ == "__main__":
    main()
