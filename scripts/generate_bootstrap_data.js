#!/usr/bin/env node

const crypto = require("crypto");
const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");
const bcrypt = require("bcrypt");

// Parse command line arguments
const args = process.argv.slice(2);

if (args.length < 3) {
  console.error("ERROR: Missing required parameters\n");
  console.error(
    "Usage: node generate_bootstrap_data.js <username> <domain> <table_name> [admin]",
  );
  console.error("\nExample:");
  console.error(
    "  node generate_bootstrap_data.js alice example.com prod-table",
  );
  console.error(
    "  node generate_bootstrap_data.js admin example.com prod-table admin",
  );
  console.error("\nParameters:");
  console.error(
    "  username    - Required. The username to create (no defaults for security)",
  );
  console.error(
    "  domain      - Required. Your Lesser domain (e.g., lesser.app)",
  );
  console.error("  table_name  - Required. DynamoDB table name");
  console.error(
    '  admin       - Optional. Pass "admin" to create an admin user',
  );
  process.exit(1);
}

const username = args[0];
const domain = args[1];
const tableName = args[2];
const isAdmin = args[3] === "admin";

// Validate username
if (!username || username.length < 3) {
  console.error("ERROR: Username must be at least 3 characters long");
  process.exit(1);
}

if (username.match(/[^a-zA-Z0-9_-]/)) {
  console.error(
    "ERROR: Username can only contain letters, numbers, underscore and hyphen",
  );
  process.exit(1);
}

console.log("=== Lesser Bootstrap Data Generator ===");
console.log(`Username: ${username}`);
console.log(`Domain: ${domain}`);
console.log(`Table: ${tableName}`);
console.log(`Admin: ${isAdmin}`);
console.log("");

// Generate secure random string
function generateRandom(length) {
  return crypto
    .randomBytes(Math.ceil((length * 3) / 4))
    .toString("base64")
    .slice(0, length)
    .replace(/\+/g, "0")
    .replace(/\//g, "0");
}

// Generate RSA key pair
function generateKeyPair() {
  const { publicKey, privateKey } = crypto.generateKeyPairSync("rsa", {
    modulusLength: 2048,
    publicKeyEncoding: {
      type: "spki",
      format: "pem",
    },
    privateKeyEncoding: {
      type: "pkcs1",
      format: "pem",
    },
  });
  return { publicKey, privateKey };
}

// Generate secure values
const clientId = generateRandom(22);
const clientSecret = generateRandom(43);

// NOTE: Passwordless authentication only - no password generation
console.log(
  "Configuring passwordless authentication (WebAuthn/Crypto Wallet)...",
);

// JWT Secret handling
let jwtSecret = process.env.JWT_SECRET || process.env.LESSER_JWT_SECRET;
let jwtSecretSource = "environment";
if (!jwtSecret) {
  console.log("\n⚠️  WARNING: No JWT_SECRET found in environment!");
  console.log("Generating a random JWT secret for this bootstrap...");
  console.log("You MUST set this same secret in your Lambda environment!");
  jwtSecret = generateRandom(32);
  jwtSecretSource = "generated";
}

// Generate RSA keys
console.log("Generating RSA key pair...");
const { publicKey, privateKey } = generateKeyPair();

// Set role and bio based on admin flag
const role = isAdmin ? "admin" : "user";
const bio = isAdmin ? "Administrator account" : "User account";

// Current timestamp
const timestamp = new Date().toISOString();

// Create output directory
const outputDir = `bootstrap_${username}_${Date.now()}`;
fs.mkdirSync(outputDir, { recursive: true });

const MAX_INT64 = BigInt("9223372036854775807");

function padNumericId(seed) {
  return seed.toString().padStart(12, "0");
}

function encodeDescendingTimestamp(date) {
  const ts = date instanceof Date ? date : new Date(date);
  const nanos = BigInt(ts.getTime()) * BigInt(1_000_000);
  const encoded = MAX_INT64 - nanos;
  return encoded.toString().padStart(19, "0");
}

// Generate Actor data
const actorData = {
  PK: { S: `ACTOR#${username}` },
  SK: { S: "PROFILE" },
  Actor: {
    M: {
      "@context": {
        L: [
          { S: "https://www.w3.org/ns/activitystreams" },
          { S: "https://w3id.org/security/v1" },
        ],
      },
      type: { S: "Person" },
      id: { S: `https://${domain}/users/${username}` },
      inbox: { S: `https://${domain}/users/${username}/inbox` },
      outbox: { S: `https://${domain}/users/${username}/outbox` },
      following: { S: `https://${domain}/users/${username}/following` },
      followers: { S: `https://${domain}/users/${username}/followers` },
      liked: { S: `https://${domain}/users/${username}/liked` },
      preferredUsername: { S: username },
      name: { S: username },
      summary: { S: bio },
      url: { S: `https://${domain}/@${username}` },
      manuallyApprovesFollowers: { BOOL: false },
      discoverable: { BOOL: true },
      publicKey: {
        M: {
          id: { S: `https://${domain}/users/${username}#main-key` },
          owner: { S: `https://${domain}/users/${username}` },
          publicKeyPem: { S: publicKey },
        },
      },
      endpoints: {
        M: {
          sharedInbox: { S: `https://${domain}/inbox` },
        },
      },
    },
  },
  PrivateKey: { S: privateKey },
  Username: { S: username },
  CreatedAt: { S: timestamp },
  UpdatedAt: { S: timestamp },
  NumericID: { S: padNumericId(crypto.randomInt(1, 1_000_000)) },
};

// Generate User data
const userData = {
  PK: { S: `USER#${username}` },
  SK: { S: "METADATA" },
  GSI1PK: { S: "USERS" },
  GSI1SK: { S: `${timestamp}#${username}` },
  GSI2PK: { S: `EMAIL#${username}@${domain}` },
  GSI2SK: { S: `USERNAME#${username}` },
  GSI3PK: { S: `ROLE#${role}` },
  GSI3SK: { S: username },
  GSI4PK: { S: "STATUS#active" },
  GSI4SK: { S: username },
  Username: { S: username },
  Email: { S: `${username}@${domain}` },
  // PasswordHash omitted - passwordless authentication only (WebAuthn/Crypto Wallet)
  RecoveryMethods: { L: [{ S: "webauthn" }] }, // Default to WebAuthn recovery
  DisplayName: { S: isAdmin ? "Administrator" : username },
  CreatedAt: { S: timestamp },
  UpdatedAt: { S: timestamp },
  Approved: { BOOL: true },
  Suspended: { BOOL: false },
  Silenced: { BOOL: false },
  Role: { S: role },
  AllowNSFW: { BOOL: false },
  RequireNSFWWarning: { BOOL: true },
  Locked: { BOOL: false },
  Discoverable: { BOOL: true },
  Metadata: { M: {} },
  Version: { N: "1" },
};

// Generate OAuth Client data
const oauthClientData = {
  PK: { S: `OAUTH_CLIENT#${clientId}` },
  SK: { S: "CLIENT" },
  OAuthClientsPK: { S: "OAUTH_CLIENTS" },
  OAuthClientsSK: {
    S: `CREATED_AT#${encodeDescendingTimestamp(timestamp)}#CLIENT#${clientId}`,
  },
  ClientID: { S: clientId },
  ClientSecret: { S: clientSecret },
  Name: { S: "Bootstrap Client" },
  Website: { S: `https://${domain}` },
  RedirectURIs: {
    L: [
      { S: `https://${domain}/auth/callback` },
      { S: "urn:ietf:wg:oauth:2.0:oob" },
    ],
  },
  Scopes: {
    L: [{ S: "read" }, { S: "write" }, { S: "follow" }, { S: "push" }],
  },
  Confidential: { BOOL: true },
  CreatedAt: { S: timestamp },
  UpdatedAt: { S: timestamp },
};

// Write JSON files
fs.writeFileSync(
  path.join(outputDir, "actor.json"),
  JSON.stringify(actorData, null, 2),
);
fs.writeFileSync(
  path.join(outputDir, "user.json"),
  JSON.stringify(userData, null, 2),
);
fs.writeFileSync(
  path.join(outputDir, "oauth_client.json"),
  JSON.stringify(oauthClientData, null, 2),
);

// Generate deployment script
const deployScript = `#!/bin/bash
set -e

TABLE_NAME="${tableName}"

echo "Deploying to table: $TABLE_NAME"

# Deploy items
echo "Creating actor..."
aws dynamodb put-item --table-name "$TABLE_NAME" --item file://actor.json

echo "Creating user..."
aws dynamodb put-item --table-name "$TABLE_NAME" --item file://user.json

echo "Creating OAuth client..."
aws dynamodb put-item --table-name "$TABLE_NAME" --item file://oauth_client.json

echo "✅ Deployment complete!"
`;

fs.writeFileSync(path.join(outputDir, "deploy.sh"), deployScript);
fs.chmodSync(path.join(outputDir, "deploy.sh"), "755");

// Generate credentials file
const credentials = `=== Lesser Bootstrap Credentials ===
Generated: ${timestamp}

Username: ${username}
Domain: ${domain}
Role: ${role}

OAuth Client:
  Client ID: ${clientId}
  Client Secret: ${clientSecret}

Authentication: Passwordless (WebAuthn/Crypto Wallet)
  - No password required
  - Use WebAuthn or crypto wallet to authenticate
  - JWT tokens can be generated for testing using the scripts below

JWT Secret: ${jwtSecret}
JWT Secret Source: ${jwtSecretSource}

Actor ID: https://${domain}/users/${username}
Profile URL: https://${domain}/@${username}

${
  jwtSecretSource === "generated"
    ? `⚠️  CRITICAL: You MUST set this JWT secret in your Lambda environment:
   AWS Lambda: Set environment variable JWT_SECRET=${jwtSecret}
   Local: export JWT_SECRET=${jwtSecret}

   Without this, authentication WILL NOT WORK!`
    : "✓ Using JWT_SECRET from environment"
}

IMPORTANT: Save these credentials securely!
`;

fs.writeFileSync(path.join(outputDir, "credentials.txt"), credentials);

// Generate JWT token creation script
const tokenScript = `const crypto = require('crypto');

// Generate JWT token that matches Lesser's auth.Claims structure
function createJWT(payload, secret) {
    const header = {
        alg: 'HS256',
        typ: 'JWT'
    };

    const encodedHeader = Buffer.from(JSON.stringify(header)).toString('base64url');
    const encodedPayload = Buffer.from(JSON.stringify(payload)).toString('base64url');

    const signature = crypto
        .createHmac('sha256', secret)
        .update(encodedHeader + '.' + encodedPayload)
        .digest('base64url');

    return encodedHeader + '.' + encodedPayload + '.' + signature;
}

// Payload must match auth.Claims structure exactly
const now = Math.floor(Date.now() / 1000);
const payload = {
    // JWT standard claims
    sub: '${username}',
    iat: now,
    exp: now + 3600, // 1 hour
    nbf: now,

    // Lesser custom claims (must match auth.Claims struct)
    username: '${username}',
    scopes: ['read', 'write', 'follow', 'push'],
    client_id: '${clientId}'
};

const secret = process.env.JWT_SECRET || '${jwtSecret}';
if (!secret) {
    console.error('ERROR: JWT_SECRET environment variable not set');
    process.exit(1);
}

const token = createJWT(payload, secret);

console.log('Bearer ' + token);
console.error('\nToken generated successfully!');
console.error('Expires in 1 hour');
console.error('\nTest with:');
console.error('curl -H "Authorization: Bearer ' + token + '" https://${domain}/api/v1/accounts/verify_credentials');
`;

fs.writeFileSync(path.join(outputDir, "create_token.js"), tokenScript);

// Generate test commands
const testCommands = `#!/bin/bash

# Test commands for Lesser API
DOMAIN="${domain}"
TOKEN="$1"

if [ -z "$TOKEN" ]; then
    echo "Usage: ./test_commands.sh <jwt_token>"
    echo "Generate token using: JWT_SECRET=${jwtSecret} node create_token.js"
    exit 1
fi

echo "Testing with domain: $DOMAIN"
echo "Testing with token: $TOKEN"
echo ""

# Test authentication
echo "1. Testing authentication..."
curl -v -H "Authorization: Bearer $TOKEN" \\
  "https://$DOMAIN/api/v1/accounts/verify_credentials"

echo -e "\\n\\n2. Creating test post..."
curl -v -X POST "https://$DOMAIN/api/v1/statuses" \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"status": "Hello from Lesser!", "visibility": "public"}'

echo -e "\\n\\n3. Getting home timeline..."
curl -v -H "Authorization: Bearer $TOKEN" \\
  "https://$DOMAIN/api/v1/timelines/home"
`;

fs.writeFileSync(path.join(outputDir, "test_commands.sh"), testCommands);
fs.chmodSync(path.join(outputDir, "test_commands.sh"), "755");

// Output summary
console.log("\n✅ Bootstrap data generated successfully!\n");
console.log(`Output directory: ${outputDir}\n`);
console.log("Files created:");
console.log("  - actor.json         : Actor (ActivityPub profile) data");
console.log("  - user.json          : User authentication data");
console.log("  - oauth_client.json  : OAuth client configuration");
console.log("  - credentials.txt    : All credentials (SAVE THIS!)");
console.log("  - deploy.sh          : Deployment script");
console.log("  - create_token.js    : JWT token generator");
console.log("  - test_commands.sh   : API test commands");
console.log("\nNext steps:");
console.log(`1. cd ${outputDir}`);
console.log(`2. ./deploy.sh`);
console.log(`3. Set environment variable: export JWT_SECRET=${jwtSecret}`);
console.log(
  `4. Generate JWT token: JWT_SECRET=${jwtSecret} node create_token.js`,
);
console.log(`5. Test API: ./test_commands.sh <token>`);
console.log(`\n🔐 Authentication: Passwordless (WebAuthn/Crypto Wallet only)`);
console.log("⚠️  Save credentials.txt in a secure location!");
