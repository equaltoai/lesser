/**
 * A contract-shaped stand-in for the Lesser API, used by the browser tests.
 *
 * This is deliberately NOT a response fabricator. It mirrors the parts of the
 * real server that the UI's correctness depends on:
 *
 *  - login/begin fails with the real "no passkeys registered for this user"
 *    error when the username owns no credential. That is exactly the failure
 *    the register page used to hit, so a regression back to the login
 *    endpoints fails the tests rather than passing silently.
 *  - signup/finish binds the attestation to the ceremony: it decodes
 *    clientDataJSON and rejects unless type is "webauthn.create" and the
 *    challenge and origin match the ones it issued.
 *  - login/finish does the same for "webauthn.get" assertions.
 *  - /api/v1/accounts enforces the exactly-one-of proof rule, single-use proof
 *    consumption, proof/username binding, and username uniqueness.
 *
 * Error bodies use the server's `Error` contract shape
 * ({ error, error_code?, error_description? }) so the UI's error mapping is
 * exercised against real payloads.
 */
import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http';
import { createHash, randomBytes, randomUUID } from 'node:crypto';
import { existsSync, readFileSync, statSync } from 'node:fs';
import { extname, join, normalize } from 'node:path';
import type { AddressInfo } from 'node:net';

export interface StubAccount {
  id: string;
  username: string;
  credentialIds: string[];
}

interface SignupChallenge {
  username: string;
  challenge: string;
}

interface PasskeyProof {
  id: string;
  username: string;
  ceremonyId: string;
  credentialId: string;
  consumed: boolean;
}

interface WalletChallenge {
  id: string;
  username: string;
  address: string;
  message: string;
  verified: boolean;
  spent: boolean;
}

/** Requests the stub saw, so tests can assert which endpoints the UI called. */
export interface RequestLogEntry {
  method: string;
  path: string;
}

const MIME_TYPES: Record<string, string> = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.woff2': 'font/woff2',
  '.ico': 'image/x-icon'
};

function base64UrlEncode(input: Buffer): string {
  return input.toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function base64UrlDecode(input: string): Buffer {
  const normalized = input.replace(/-/g, '+').replace(/_/g, '/');
  return Buffer.from(normalized, 'base64');
}

export class LesserStubServer {
  private readonly server: Server;
  private readonly distDir: string;

  private readonly accountsByUsername = new Map<string, StubAccount>();
  private readonly signupChallenges = new Map<string, SignupChallenge>();
  private readonly loginChallenges = new Map<string, SignupChallenge>();
  private readonly passkeyProofs = new Map<string, PasskeyProof>();
  private readonly walletChallenges = new Map<string, WalletChallenge>();

  readonly requestLog: RequestLogEntry[] = [];

  /** Forces the next POST /api/v1/accounts to answer 409, for the error-path test. */
  usernameTakenOverride: string | null = null;

  /** Base URL the stub is listening on. Empty until start() resolves. */
  origin = '';

  constructor(distDir: string) {
    this.distDir = distDir;
    this.server = createServer((req, res) => {
      this.handle(req, res).catch((err) => {
        this.sendJson(res, 500, { error: `stub server failure: ${String(err)}` });
      });
    });
  }

  async start(): Promise<string> {
    // Must be "localhost", not 127.0.0.1: WebAuthn rejects an IP literal as an
    // RP ID, while localhost is both a valid RP ID and a secure context.
    await new Promise<void>((resolve) => this.server.listen(0, 'localhost', resolve));
    const { port } = this.server.address() as AddressInfo;
    this.origin = `http://localhost:${port}`;
    return this.origin;
  }

  async stop(): Promise<void> {
    await new Promise<void>((resolve, reject) =>
      this.server.close((err) => (err ? reject(err) : resolve()))
    );
  }

  getAccount(username: string): StubAccount | undefined {
    return this.accountsByUsername.get(username.toLowerCase());
  }

  seedAccount(username: string): void {
    const key = username.toLowerCase();
    this.accountsByUsername.set(key, { id: randomUUID(), username: key, credentialIds: [] });
  }

  pathsCalled(): string[] {
    return this.requestLog.map((entry) => `${entry.method} ${entry.path}`);
  }

  // ---------------------------------------------------------------- routing

  private async handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
    const url = new URL(req.url ?? '/', this.origin);
    const path = url.pathname;
    this.requestLog.push({ method: req.method ?? 'GET', path });

    if (req.method === 'POST') {
      const body = await this.readJsonBody(req);
      switch (path) {
        case '/api/v1/auth/webauthn/signup/begin':
          return this.postSignupBegin(res, body);
        case '/api/v1/auth/webauthn/signup/finish':
          return this.postSignupFinish(res, body);
        case '/api/v1/auth/webauthn/login/begin':
          return this.postLoginBegin(res, body);
        case '/api/v1/auth/webauthn/login/finish':
          return this.postLoginFinish(res, body);
        case '/api/v1/accounts':
          return this.postAccounts(res, body);
        case '/auth/wallet/challenge':
          return this.postWalletChallenge(res, body);
        case '/auth/wallet/verify':
          return this.postWalletVerify(res, body);
        case '/auth/wallet/link':
          return this.postWalletLink(res, body);
        default:
          return this.sendJson(res, 404, { error: 'Not Found', error_code: 'NOT_FOUND' });
      }
    }

    if (req.method === 'GET' && path === '/oauth/authorize') {
      return this.getAuthorize(req, res, url);
    }
    if (req.method === 'GET') {
      return this.serveStatic(res, path);
    }

    return this.sendJson(res, 405, { error: 'Method Not Allowed' });
  }

  // ------------------------------------------------------------- ceremonies

  private postSignupBegin(res: ServerResponse, body: Record<string, unknown>): void {
    const username = String(body.username ?? '').trim().toLowerCase();
    if (!username) {
      return this.sendJson(res, 400, { error: 'username required', error_code: 'BAD_REQUEST' });
    }

    const challenge = base64UrlEncode(randomBytes(32));
    this.signupChallenges.set(challenge, { username, challenge });

    // Shape mirrors go-webauthn's CredentialCreation.Response.
    this.sendJson(res, 200, {
      challenge,
      publicKey: {
        challenge,
        rp: { name: 'Lesser', id: 'localhost' },
        user: {
          id: base64UrlEncode(Buffer.from(username, 'utf8')),
          name: username,
          displayName: username
        },
        pubKeyCredParams: [
          { type: 'public-key', alg: -7 },
          { type: 'public-key', alg: -257 }
        ],
        authenticatorSelection: { userVerification: 'required' },
        timeout: 60000,
        attestation: 'none'
      }
    });
  }

  private postSignupFinish(res: ServerResponse, body: Record<string, unknown>): void {
    const username = String(body.username ?? '').trim().toLowerCase();
    const challenge = String(body.challenge ?? '').trim();
    const response = body.response as Record<string, unknown> | undefined;

    const pending = this.signupChallenges.get(challenge);
    if (!pending || pending.username !== username) {
      return this.sendJson(res, 400, {
        error: 'invalid or expired challenge',
        error_code: 'BAD_REQUEST'
      });
    }

    const attestation = response?.response as Record<string, unknown> | undefined;
    const clientDataError = this.verifyClientData(attestation?.clientDataJSON, 'webauthn.create', challenge);
    if (clientDataError) {
      return this.sendJson(res, 400, { error: clientDataError, error_code: 'BAD_REQUEST' });
    }
    if (typeof attestation?.attestationObject !== 'string' || attestation.attestationObject.length < 16) {
      return this.sendJson(res, 400, {
        error: 'attestation object missing',
        error_code: 'BAD_REQUEST'
      });
    }

    this.signupChallenges.delete(challenge);

    const proof: PasskeyProof = {
      id: randomUUID(),
      username,
      ceremonyId: challenge,
      credentialId: String(response?.id ?? ''),
      consumed: false
    };
    this.passkeyProofs.set(proof.id, proof);

    this.sendJson(res, 200, { passkey_registration_proof: proof.id });
  }

  private postLoginBegin(res: ServerResponse, body: Record<string, unknown>): void {
    const username = String(body.username ?? '').trim().toLowerCase();
    const account = this.accountsByUsername.get(username);

    // The bug this milestone fixes: a brand-new username reaches exactly here.
    if (!account || account.credentialIds.length === 0) {
      return this.sendJson(res, 400, {
        error: 'no passkeys registered for this user',
        error_code: 'BAD_REQUEST'
      });
    }

    const challenge = base64UrlEncode(randomBytes(32));
    this.loginChallenges.set(challenge, { username, challenge });

    this.sendJson(res, 200, {
      challenge,
      publicKey: {
        challenge,
        rpId: 'localhost',
        allowCredentials: account.credentialIds.map((id) => ({ type: 'public-key', id })),
        userVerification: 'required',
        timeout: 60000
      }
    });
  }

  private postLoginFinish(res: ServerResponse, body: Record<string, unknown>): void {
    const username = String(body.username ?? '').trim().toLowerCase();
    const challenge = String(body.challenge ?? '').trim();
    const response = body.response as Record<string, unknown> | undefined;

    const pending = this.loginChallenges.get(challenge);
    if (!pending || pending.username !== username) {
      return this.sendJson(res, 400, {
        error: 'invalid or expired challenge',
        error_code: 'BAD_REQUEST'
      });
    }

    const assertion = response?.response as Record<string, unknown> | undefined;
    const clientDataError = this.verifyClientData(assertion?.clientDataJSON, 'webauthn.get', challenge);
    if (clientDataError) {
      return this.sendJson(res, 401, { error: clientDataError, error_code: 'UNAUTHORIZED' });
    }
    if (typeof assertion?.signature !== 'string' || assertion.signature.length === 0) {
      return this.sendJson(res, 401, { error: 'Unauthorized', error_code: 'UNAUTHORIZED' });
    }

    this.loginChallenges.delete(challenge);

    this.sendJson(res, 200, {
      access_token: `stub-jwt-${username}-${createHash('sha256').update(challenge).digest('hex').slice(0, 12)}`,
      token_type: 'Bearer',
      scope: 'read write',
      expires_in: 3600,
      created_at: Math.floor(Date.now() / 1000),
      refresh_token: `stub-refresh-${username}`
    });
  }

  private postAccounts(res: ServerResponse, body: Record<string, unknown>): void {
    const username = String(body.username ?? '').trim().toLowerCase();
    const walletChallengeId = String(body.wallet_challenge_id ?? '').trim();
    const passkeyProofId = String(body.passkey_registration_proof ?? '').trim();

    if (body.agreement !== true) {
      return this.sendJson(res, 422, {
        error: 'agreement is required',
        error_code: 'UNPROCESSABLE_ENTITY'
      });
    }
    // Exactly one of the two proofs, per AccountRegistrationRequest's oneOf.
    if (Boolean(walletChallengeId) === Boolean(passkeyProofId)) {
      return this.sendJson(res, 422, {
        error: 'exactly one of wallet_challenge_id or passkey_registration_proof is required',
        error_code: 'UNPROCESSABLE_ENTITY'
      });
    }

    if (this.usernameTakenOverride === username || this.accountsByUsername.has(username)) {
      return this.sendJson(res, 409, {
        error: 'Username already exists',
        error_code: 'CONFLICT'
      });
    }

    const credentialIds: string[] = [];

    if (passkeyProofId) {
      const proof = this.passkeyProofs.get(passkeyProofId);
      if (!proof || proof.consumed) {
        return this.sendJson(res, 422, {
          error: 'passkey_registration_proof is invalid or expired',
          error_code: 'UNPROCESSABLE_ENTITY'
        });
      }
      if (proof.username !== username) {
        return this.sendJson(res, 422, {
          error: 'passkey registration proof was created for a different username',
          error_code: 'UNPROCESSABLE_ENTITY'
        });
      }
      proof.consumed = true;
      credentialIds.push(proof.credentialId);
    } else {
      const challenge = this.walletChallenges.get(walletChallengeId);
      if (!challenge || challenge.username !== username || !challenge.verified || challenge.spent) {
        return this.sendJson(res, 422, {
          error: 'wallet_challenge_id is invalid or expired',
          error_code: 'UNPROCESSABLE_ENTITY'
        });
      }
      challenge.spent = true;
    }

    const account: StubAccount = { id: randomUUID(), username, credentialIds };
    this.accountsByUsername.set(username, account);

    this.sendJson(res, 201, { id: account.id, username, created: true });
  }

  // ----------------------------------------------------------------- wallet

  private postWalletChallenge(res: ServerResponse, body: Record<string, unknown>): void {
    const username = String(body.username ?? '').trim().toLowerCase();
    const address = String(body.address ?? '');
    const challenge: WalletChallenge = {
      id: randomUUID(),
      username,
      address,
      message: `Sign in to Lesser as ${username} (${randomUUID()})`,
      verified: false,
      spent: false
    };
    this.walletChallenges.set(challenge.id, challenge);
    this.sendJson(res, 200, { id: challenge.id, message: challenge.message });
  }

  private postWalletVerify(res: ServerResponse, body: Record<string, unknown>): void {
    const challenge = this.walletChallenges.get(String(body.challengeId ?? ''));
    if (!challenge) {
      return this.sendJson(res, 422, {
        error: 'wallet challenge is invalid or expired',
        error_code: 'UNPROCESSABLE_ENTITY'
      });
    }
    if (typeof body.signature !== 'string' || !body.signature.startsWith('0x')) {
      return this.sendJson(res, 401, { error: 'Unauthorized', error_code: 'UNAUTHORIZED' });
    }
    challenge.verified = true;
    this.sendJson(res, 200, { verified: true });
  }

  private postWalletLink(res: ServerResponse, body: Record<string, unknown>): void {
    const username = String(body.username ?? '').trim().toLowerCase();
    if (!this.accountsByUsername.has(username)) {
      return this.sendJson(res, 404, { error: 'account not found', error_code: 'NOT_FOUND' });
    }
    this.sendJson(res, 200, {
      access_token: `stub-jwt-wallet-${username}`,
      token_type: 'Bearer',
      scope: 'read write',
      expires_in: 3600,
      created_at: Math.floor(Date.now() / 1000),
      refresh_token: `stub-refresh-${username}`
    });
  }

  // ------------------------------------------------------------------ oauth

  private getAuthorize(req: IncomingMessage, res: ServerResponse, url: URL): void {
    const authorization = req.headers.authorization ?? '';
    if (!authorization.startsWith('Bearer ')) {
      return this.sendJson(res, 401, { error: 'Unauthorized', error_code: 'UNAUTHORIZED' });
    }
    if (!url.searchParams.get('client_id') || !url.searchParams.get('state')) {
      return this.sendJson(res, 400, {
        error: 'invalid_request',
        error_description: 'client_id and state are required'
      });
    }
    this.sendJson(res, 200, { next_url: `/auth/consent?state=${url.searchParams.get('state')}` });
  }

  // ----------------------------------------------------------------- helpers

  /**
   * Verify the ceremony binding the browser actually produced. This is what
   * makes the tests non-vacuous: a signup ceremony and a login ceremony
   * produce different `type` values over different challenges.
   */
  private verifyClientData(
    clientDataJSON: unknown,
    expectedType: string,
    expectedChallenge: string
  ): string | null {
    if (typeof clientDataJSON !== 'string' || clientDataJSON.length === 0) {
      return 'clientDataJSON missing';
    }

    let parsed: { type?: string; challenge?: string; origin?: string };
    try {
      parsed = JSON.parse(base64UrlDecode(clientDataJSON).toString('utf8'));
    } catch {
      return 'clientDataJSON is not valid JSON';
    }

    if (parsed.type !== expectedType) {
      return `ceremony type mismatch: expected ${expectedType}, got ${parsed.type}`;
    }
    if (parsed.challenge !== expectedChallenge) {
      return 'challenge mismatch';
    }
    if (parsed.origin !== this.origin) {
      return `origin mismatch: ${parsed.origin}`;
    }
    return null;
  }

  private async readJsonBody(req: IncomingMessage): Promise<Record<string, unknown>> {
    const chunks: Buffer[] = [];
    for await (const chunk of req) {
      chunks.push(chunk as Buffer);
    }
    if (chunks.length === 0) {
      return {};
    }
    try {
      return JSON.parse(Buffer.concat(chunks).toString('utf8')) as Record<string, unknown>;
    } catch {
      return {};
    }
  }

  private sendJson(res: ServerResponse, status: number, body: unknown): void {
    const payload = JSON.stringify(body);
    res.writeHead(status, {
      'Content-Type': 'application/json; charset=utf-8',
      'Content-Length': Buffer.byteLength(payload)
    });
    res.end(payload);
  }

  /** Serve the real production build of the auth UI under its /auth/ base path. */
  private serveStatic(res: ServerResponse, path: string): void {
    const relative = path.replace(/^\/auth\/?/, '');
    const candidates = [
      join(this.distDir, relative),
      join(this.distDir, relative, 'index.html'),
      join(this.distDir, `${relative}.html`)
    ];

    for (const candidate of candidates) {
      const resolved = normalize(candidate);
      if (!resolved.startsWith(this.distDir) || !existsSync(resolved) || !statSync(resolved).isFile()) {
        continue;
      }
      const body = readFileSync(resolved);
      res.writeHead(200, {
        'Content-Type': MIME_TYPES[extname(resolved)] ?? 'application/octet-stream',
        'Content-Length': body.byteLength
      });
      res.end(body);
      return;
    }

    res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
    res.end('not found');
  }
}
