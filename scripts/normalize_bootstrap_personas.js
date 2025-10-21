#!/usr/bin/env node

/**
 * Normalizes bootstrap persona JSON files to conform with the Dynamo attribute naming
 * standard (PascalCase for persisted fields).
 */

const fs = require('fs');
const path = require('path');

const personaDirs = fs
  .readdirSync(process.cwd(), { withFileTypes: true })
  .filter((dirent) => dirent.isDirectory() && dirent.name.startsWith('bootstrap_'))
  .map((dirent) => dirent.name);

if (personaDirs.length === 0) {
  console.error('No bootstrap_* directories found. Nothing to normalize.');
  process.exit(0);
}

const userRenameMap = {
  username: 'Username',
  email: 'Email',
  password_hash: 'PasswordHash',
  display_name: 'DisplayName',
  created_at: 'CreatedAt',
  updated_at: 'UpdatedAt',
  approved: 'Approved',
  suspended: 'Suspended',
  silenced: 'Silenced',
  role: 'Role',
  allow_nsfw: 'AllowNSFW',
  require_nsfw_warning: 'RequireNSFWWarning',
  version: 'Version',
};

const actorRenameMap = {
  username: 'Username',
};

const oauthRenameMap = {
  client_id: 'ClientID',
  client_secret: 'ClientSecret',
  name: 'Name',
  description: 'Description',
  website: 'Website',
  redirect_uris: 'RedirectURIs',
  grant_types: 'GrantTypes',
  scopes: 'Scopes',
  owner_id: 'OwnerID',
  created_at: 'CreatedAt',
  updated_at: 'UpdatedAt',
};

const MAX_INT64 = BigInt('9223372036854775807');

function encodeDescendingTimestamp(isoString) {
  const parsed = new Date(isoString);
  if (Number.isNaN(parsed.getTime())) {
    return null;
  }
  const nanos = BigInt(parsed.getTime()) * BigInt(1_000_000);
  return (MAX_INT64 - nanos).toString().padStart(19, '0');
}

function normalizeFile(filePath, renameMap) {
  if (!fs.existsSync(filePath)) {
    return;
  }

  const raw = fs.readFileSync(filePath, 'utf8');
  let data;
  try {
    data = JSON.parse(raw);
  } catch (err) {
    console.error(`Failed to parse ${filePath}: ${err.message}`);
    return;
  }

  for (const [from, to] of Object.entries(renameMap)) {
    if (Object.prototype.hasOwnProperty.call(data, from)) {
      if (!Object.prototype.hasOwnProperty.call(data, to)) {
        data[to] = data[from];
      }
      delete data[from];
    }
  }

  // Additional normalization for OAuth client records
  if (filePath.endsWith('oauth_client.json')) {
    const clientId = data.ClientID?.S;
    const createdAt = data.CreatedAt?.S;

    if (data.SK?.S && data.SK.S !== 'CLIENT') {
      data.SK.S = 'CLIENT';
    }

    if (!data.OAuthClientsPK) {
      data.OAuthClientsPK = { S: 'OAUTH_CLIENTS' };
    }

    if (!data.OAuthClientsSK && clientId && createdAt) {
      const desc = encodeDescendingTimestamp(createdAt);
      if (desc) {
        data.OAuthClientsSK = { S: `CREATED_AT#${desc}#CLIENT#${clientId}` };
      }
    }

    if (!data.Confidential) {
      data.Confidential = { BOOL: true };
    }

    if (!data.UpdatedAt && createdAt) {
      data.UpdatedAt = { S: createdAt };
    }
  }

  fs.writeFileSync(filePath, JSON.stringify(data, null, 2) + '\n');
}

for (const dir of personaDirs) {
  normalizeFile(path.join(dir, 'user.json'), userRenameMap);
  normalizeFile(path.join(dir, 'actor.json'), actorRenameMap);
  normalizeFile(path.join(dir, 'oauth_client.json'), oauthRenameMap);
}

console.log(`Normalized bootstrap persona data in ${personaDirs.length} directories.`);
