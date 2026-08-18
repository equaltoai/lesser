/**
 * The cryptographic half of the Lesser API stub.
 *
 * The browser tests drive a real WebAuthn ceremony against Chrome's virtual
 * authenticator, so the stub can — and must — verify what that authenticator
 * actually signed rather than trusting the shape of the JSON it receives.
 * Without this, a serializer that emitted a constant signature would still
 * satisfy every assertion in the suite.
 *
 * Everything here is dependency-free on purpose: a minimal definite-length
 * CBOR reader (enough for an attestation object and a COSE key), COSE ->
 * Node KeyObject conversion for the two algorithms the ceremony offers
 * (ES256 / RS256), and the WebAuthn verification step itself — a signature
 * over `authenticatorData || SHA-256(clientDataJSON)`, which is what
 * go-webauthn checks on the real server.
 */
import { createHash, createPublicKey, verify as verifyWithKey, type KeyObject } from 'node:crypto';

export type CborValue =
  | number
  | string
  | Buffer
  | boolean
  | null
  | CborValue[]
  | Map<CborValue, CborValue>;

interface DecodedCbor {
  value: CborValue;
  offset: number;
}

/** COSE algorithm identifiers advertised in the ceremony's pubKeyCredParams. */
export const ALG_ES256 = -7;
export const ALG_RS256 = -257;

const FLAG_USER_PRESENT = 0x01;
const FLAG_USER_VERIFIED = 0x04;
const FLAG_ATTESTED_CREDENTIAL_DATA = 0x40;

export function base64UrlEncode(input: Buffer): string {
  return input.toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

export function base64UrlDecode(input: string): Buffer {
  return Buffer.from(input.replace(/-/g, '+').replace(/_/g, '/'), 'base64');
}

/** Credential ids travel as base64url; re-encode so padding variants compare equal. */
export function normalizeCredentialId(input: string): string {
  return base64UrlEncode(base64UrlDecode(input));
}

export function sha256(input: Buffer): Buffer {
  return createHash('sha256').update(input).digest();
}

// -------------------------------------------------------------------- CBOR

function readLength(buf: Buffer, offset: number, info: number): { length: number; offset: number } {
  if (info < 24) {
    return { length: info, offset };
  }
  if (info === 24) {
    return { length: buf.readUInt8(offset), offset: offset + 1 };
  }
  if (info === 25) {
    return { length: buf.readUInt16BE(offset), offset: offset + 2 };
  }
  if (info === 26) {
    return { length: buf.readUInt32BE(offset), offset: offset + 4 };
  }
  if (info === 27) {
    const wide = buf.readBigUInt64BE(offset);
    if (wide > BigInt(Number.MAX_SAFE_INTEGER)) {
      throw new Error('CBOR integer is out of range');
    }
    return { length: Number(wide), offset: offset + 8 };
  }
  // 28-30 are reserved and 31 is the indefinite-length form, which no
  // authenticator emits for these structures.
  throw new Error(`CBOR additional information ${info} is not supported`);
}

/**
 * Decode one CBOR item, reporting where it ended. The offset matters: the
 * COSE public key is the tail of the attested credential data, and its length
 * is only knowable by decoding it.
 */
export function decodeCbor(buf: Buffer, start = 0): DecodedCbor {
  if (start >= buf.length) {
    throw new Error('CBOR input is truncated');
  }

  const initial = buf.readUInt8(start);
  const major = initial >> 5;
  const info = initial & 0x1f;
  const head = readLength(buf, start + 1, info);
  let offset = head.offset;
  const length = head.length;

  switch (major) {
    case 0:
      return { value: length, offset };
    case 1:
      return { value: -1 - length, offset };
    case 2:
    case 3: {
      if (offset + length > buf.length) {
        throw new Error('CBOR string is truncated');
      }
      const slice = buf.subarray(offset, offset + length);
      return { value: major === 2 ? slice : slice.toString('utf8'), offset: offset + length };
    }
    case 4: {
      const items: CborValue[] = [];
      for (let i = 0; i < length; i++) {
        const item = decodeCbor(buf, offset);
        items.push(item.value);
        offset = item.offset;
      }
      return { value: items, offset };
    }
    case 5: {
      const entries = new Map<CborValue, CborValue>();
      for (let i = 0; i < length; i++) {
        const key = decodeCbor(buf, offset);
        const value = decodeCbor(buf, key.offset);
        entries.set(key.value, value.value);
        offset = value.offset;
      }
      return { value: entries, offset };
    }
    case 7: {
      if (info === 20) return { value: false, offset };
      if (info === 21) return { value: true, offset };
      if (info === 22) return { value: null, offset };
      throw new Error(`CBOR simple value ${info} is not supported`);
    }
    default:
      throw new Error(`CBOR major type ${major} is not supported`);
  }
}

// ---------------------------------------------------------- authenticator data

export interface AuthenticatorData {
  rpIdHash: Buffer;
  userPresent: boolean;
  userVerified: boolean;
  signCount: number;
  credentialId?: Buffer;
  credentialPublicKey?: Map<CborValue, CborValue>;
}

/** Parse the authenticator data structure (WebAuthn §6.1). */
export function parseAuthenticatorData(data: Buffer): AuthenticatorData {
  if (data.length < 37) {
    throw new Error('authenticator data is too short');
  }

  const flags = data.readUInt8(32);
  const parsed: AuthenticatorData = {
    rpIdHash: data.subarray(0, 32),
    userPresent: (flags & FLAG_USER_PRESENT) !== 0,
    userVerified: (flags & FLAG_USER_VERIFIED) !== 0,
    signCount: data.readUInt32BE(33)
  };

  if ((flags & FLAG_ATTESTED_CREDENTIAL_DATA) === 0) {
    return parsed;
  }

  if (data.length < 55) {
    throw new Error('attested credential data is truncated');
  }
  const credentialIdLength = data.readUInt16BE(53);
  const credentialIdEnd = 55 + credentialIdLength;
  if (data.length < credentialIdEnd) {
    throw new Error('attested credential id is truncated');
  }

  parsed.credentialId = data.subarray(55, credentialIdEnd);
  const key = decodeCbor(data, credentialIdEnd);
  if (!(key.value instanceof Map)) {
    throw new Error('credential public key is not a COSE map');
  }
  parsed.credentialPublicKey = key.value;
  return parsed;
}

// ------------------------------------------------------------------ COSE keys

export interface CredentialPublicKey {
  key: KeyObject;
  alg: number;
}

/** Convert a COSE_Key into a Node public key, for the algorithms we offer. */
export function coseToPublicKey(cose: Map<CborValue, CborValue>): CredentialPublicKey {
  const kty = cose.get(1);
  const alg = cose.get(3);
  if (typeof alg !== 'number') {
    throw new Error('COSE key declares no algorithm');
  }

  if (kty === 2) {
    if (alg !== ALG_ES256) {
      throw new Error(`COSE EC2 algorithm ${alg} was not offered by this ceremony`);
    }
    if (cose.get(-1) !== 1) {
      throw new Error('COSE EC2 key is not on P-256');
    }
    const x = cose.get(-2);
    const y = cose.get(-3);
    if (!Buffer.isBuffer(x) || !Buffer.isBuffer(y)) {
      throw new Error('COSE EC2 key is missing its coordinates');
    }
    return {
      alg,
      key: createPublicKey({
        key: { kty: 'EC', crv: 'P-256', x: base64UrlEncode(x), y: base64UrlEncode(y) },
        format: 'jwk'
      })
    };
  }

  if (kty === 3) {
    if (alg !== ALG_RS256) {
      throw new Error(`COSE RSA algorithm ${alg} was not offered by this ceremony`);
    }
    const n = cose.get(-1);
    const e = cose.get(-2);
    if (!Buffer.isBuffer(n) || !Buffer.isBuffer(e)) {
      throw new Error('COSE RSA key is missing its modulus or exponent');
    }
    return {
      alg,
      key: createPublicKey({
        key: { kty: 'RSA', n: base64UrlEncode(n), e: base64UrlEncode(e) },
        format: 'jwk'
      })
    };
  }

  throw new Error(`COSE key type ${String(kty)} is not supported`);
}

// ----------------------------------------------------------------- signatures

/** The bytes an authenticator signs: authenticatorData || SHA-256(clientDataJSON). */
export function signedBytes(authenticatorData: Buffer, clientDataJSON: Buffer): Buffer {
  return Buffer.concat([authenticatorData, sha256(clientDataJSON)]);
}

/**
 * Verify a WebAuthn signature. A malformed signature makes Node throw rather
 * than return false, and both mean the same thing here: not verified.
 */
export function verifySignature(
  credential: CredentialPublicKey,
  data: Buffer,
  signature: Buffer
): boolean {
  if (signature.length === 0) {
    return false;
  }
  try {
    if (credential.alg === ALG_ES256) {
      return verifyWithKey('sha256', data, { key: credential.key, dsaEncoding: 'der' }, signature);
    }
    return verifyWithKey('sha256', data, credential.key, signature);
  } catch {
    return false;
  }
}

// ---------------------------------------------------------- attestation object

export interface AttestationObject {
  fmt: string;
  attStmt: Map<CborValue, CborValue>;
  authData: Buffer;
}

export function decodeAttestationObject(raw: Buffer): AttestationObject {
  const decoded = decodeCbor(raw);
  if (!(decoded.value instanceof Map)) {
    throw new Error('attestation object is not a CBOR map');
  }

  const fmt = decoded.value.get('fmt');
  const attStmt = decoded.value.get('attStmt');
  const authData = decoded.value.get('authData');

  if (typeof fmt !== 'string') {
    throw new Error('attestation object declares no format');
  }
  if (!(attStmt instanceof Map)) {
    throw new Error('attestation object carries no statement');
  }
  if (!Buffer.isBuffer(authData)) {
    throw new Error('attestation object carries no authenticator data');
  }

  return { fmt, attStmt, authData };
}
