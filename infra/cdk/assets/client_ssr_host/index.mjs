import { createWriteStream } from "node:fs";
import { mkdir, stat } from "node:fs/promises";
import path from "node:path";
import { pipeline } from "node:stream/promises";
import { pathToFileURL } from "node:url";

import { GetObjectCommand, HeadObjectCommand, S3Client } from "@aws-sdk/client-s3";

const artifactBucket = process.env.LESSER_CLIENT_ARTIFACT_BUCKET || "";
const manifestKey = process.env.LESSER_CLIENT_INSTALL_KEY || "install/current.json";
const stageDomain = process.env.LESSER_STAGE_DOMAIN || "";
const basePath = process.env.LESSER_CLIENT_BASE_PATH || "/l";
const manifestPollMs = Number.parseInt(process.env.LESSER_CLIENT_MANIFEST_POLL_MS || "1000", 10);

const s3 = new S3Client({});

let manifestCheckedAt = 0;
let manifestEtag = "";
let cachedManifest = null;
let loadedInstallId = "";
let loadedModule = null;

export async function handler(event, context) {
  try {
    const manifest = await loadCurrentManifest();
    if (!manifest) {
      return placeholderResponse(event);
    }

    const mod = await loadInstalledModule(manifest);
    const exportName = manifest?.server?.export_name || "handler";
    const entrypoint = mod?.[exportName];
    if (typeof entrypoint !== "function") {
      throw new Error(`install module does not export function ${exportName}`);
    }

    const response = await entrypoint(event, context);
    return normalizeResponse(response);
  } catch (error) {
    console.error("client_ssr_host_error", {
      message: error instanceof Error ? error.message : String(error),
      stack: error instanceof Error ? error.stack : undefined,
    });
    return {
      statusCode: 503,
      headers: {
        "content-type": "text/html; charset=utf-8",
        "cache-control": "no-store",
        ...securityHeaders(),
      },
      body: errorPage(),
    };
  }
}

async function loadCurrentManifest() {
  if (!artifactBucket) {
    return null;
  }

  const now = Date.now();
  if (cachedManifest && now-manifestCheckedAt < manifestPollMs) {
    return cachedManifest;
  }

  manifestCheckedAt = now;

  let head;
  try {
    head = await s3.send(new HeadObjectCommand({
      Bucket: artifactBucket,
      Key: manifestKey,
    }));
  } catch (error) {
    if (isS3NotFound(error)) {
      cachedManifest = null;
      manifestEtag = "";
      return null;
    }
    throw error;
  }

  const nextEtag = String(head.ETag || "");
  if (cachedManifest && manifestEtag !== "" && nextEtag === manifestEtag) {
    return cachedManifest;
  }

  const result = await s3.send(new GetObjectCommand({
    Bucket: artifactBucket,
    Key: manifestKey,
  }));
  const body = await streamToString(result.Body);
  cachedManifest = JSON.parse(body);
  manifestEtag = nextEtag;
  return cachedManifest;
}

async function loadInstalledModule(manifest) {
  const installId = String(manifest?.install_id || "");
  if (!installId) {
    throw new Error("install manifest is missing install_id");
  }
  if (loadedModule && loadedInstallId === installId) {
    return loadedModule;
  }

  const server = manifest?.server || {};
  const root = String(server.root || "");
  const entry = String(server.entry || "");
  const files = Array.isArray(server.files) ? server.files : [];
  if (!root || !entry || files.length === 0) {
    throw new Error("install manifest is missing server.root, server.entry, or server.files");
  }

  const targetRoot = path.join("/tmp", "lesser-client", sanitizePathSegment(installId));
  await mkdir(targetRoot, { recursive: true });

  for (const relativePath of files) {
    await ensureFileDownloaded(targetRoot, root, String(relativePath));
  }

  const entryPath = path.join(targetRoot, entry);
  loadedModule = await import(pathToFileURL(entryPath).href);
  loadedInstallId = installId;
  return loadedModule;
}

async function ensureFileDownloaded(targetRoot, serverRoot, relativePath) {
  const localPath = path.join(targetRoot, relativePath);
  try {
    const info = await stat(localPath);
    if (info.isFile()) {
      return;
    }
  } catch (_error) {
  }

  await mkdir(path.dirname(localPath), { recursive: true });
  const result = await s3.send(new GetObjectCommand({
    Bucket: artifactBucket,
    Key: joinS3Path(serverRoot, relativePath),
  }));
  await pipeline(result.Body, createWriteStream(localPath));
}

async function normalizeResponse(response) {
  if (response instanceof Response) {
    return responseToLambda(response);
  }

  if (response && typeof response === "object" && "statusCode" in response) {
    return response;
  }

  if (response && typeof response === "object" && "status" in response && "body" in response) {
    return {
      statusCode: response.status,
      headers: response.headers || {},
      body: response.body,
      isBase64Encoded: Boolean(response.isBase64Encoded),
      cookies: Array.isArray(response.cookies) ? response.cookies : undefined,
    };
  }

  throw new Error("install handler returned an unsupported response value");
}

async function responseToLambda(response) {
  const headers = {};
  for (const [key, value] of response.headers.entries()) {
    headers[key] = value;
  }

  const contentType = headers["content-type"] || "";
  const bodyBytes = Buffer.from(await response.arrayBuffer());
  const isBinary = !isTextContentType(contentType);

  return {
    statusCode: response.status,
    headers,
    body: isBinary ? bodyBytes.toString("base64") : bodyBytes.toString("utf8"),
    isBase64Encoded: isBinary,
  };
}

function placeholderResponse(event) {
  const origin = publicOrigin(event);
  const escapedOrigin = escapeHtml(origin);
  const authLink = origin ? `<a href="${escapedOrigin}/auth">${escapedOrigin}/auth</a>` : "<code>/auth</code>";
  const apiLink = origin ? `<a href="${escapedOrigin}/">${escapedOrigin}/</a>` : "<code>/</code>";

  return {
    statusCode: 200,
    headers: {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "no-store",
      ...securityHeaders(),
    },
    body: `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Lesser</title>
  </head>
  <body>
    <h1>Lesser is deployed</h1>
    <p>The FaceTheory client has not been installed yet.</p>
    <p>Client base path: <code>${escapeHtml(basePath)}</code></p>
    <p>Auth UI: ${authLink}</p>
    <p>API: ${apiLink}</p>
    <p>Setup status: <code>GET /setup/status</code></p>
  </body>
</html>`,
  };
}

function errorPage() {
  return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Lesser</title>
  </head>
  <body>
    <h1>Client unavailable</h1>
    <p>The installed FaceTheory client could not be loaded.</p>
  </body>
</html>`;
}

function isS3NotFound(error) {
  const name = String(error?.name || "");
  const code = String(error?.Code || error?.code || "");
  const statusCode = Number(error?.$metadata?.httpStatusCode || 0);
  return name === "NotFound" || name === "NoSuchKey" || code === "NotFound" || code === "NoSuchKey" || statusCode === 404;
}

async function streamToString(body) {
  const chunks = [];
  for await (const chunk of body) {
    chunks.push(typeof chunk === "string" ? Buffer.from(chunk) : Buffer.from(chunk));
  }
  return Buffer.concat(chunks).toString("utf8");
}

function isTextContentType(contentType) {
  const clean = String(contentType || "").toLowerCase();
  return clean.startsWith("text/") ||
    clean.includes("json") ||
    clean.includes("javascript") ||
    clean.includes("xml") ||
    clean.includes("svg");
}

function sanitizePathSegment(value) {
  return String(value || "").replace(/[^a-zA-Z0-9._-]+/g, "-");
}

function joinS3Path(root, suffix) {
  return [root, suffix]
    .map((part) => String(part || "").replace(/^\/+|\/+$/g, ""))
    .filter(Boolean)
    .join("/");
}

function securityHeaders() {
  return {
    "content-security-policy": "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'; object-src 'none'",
    "x-content-type-options": "nosniff",
    "x-frame-options": "DENY",
    "referrer-policy": "strict-origin-when-cross-origin",
    "cross-origin-resource-policy": "same-origin",
    "permissions-policy": "camera=(), geolocation=(), microphone=(), payment=(), usb=()",
  };
}

function publicOrigin(event) {
  const host = sanitizeHost(
    headerValue(event, "x-lesser-forwarded-host") ||
    headerValue(event, "host") ||
    stageDomain,
  );
  return host ? `https://${host}` : "";
}

function headerValue(event, name) {
  const headers = event?.headers || {};
  const want = String(name || "").toLowerCase();
  for (const [key, value] of Object.entries(headers)) {
    if (String(key || "").toLowerCase() === want) {
      return Array.isArray(value) ? value[0] : value;
    }
  }
  return "";
}

function sanitizeHost(value) {
  const first = String(value || "").split(",")[0].trim();
  if (!first || first.length > 253) {
    return "";
  }
  if (first.includes("/") || first.includes("\\") || first.includes("@")) {
    return "";
  }
  if (!/^(?:[a-zA-Z0-9.-]+|\[[0-9a-fA-F:.]+])(?::[0-9]{1,5})?$/.test(first)) {
    return "";
  }
  return first.toLowerCase();
}

function escapeHtml(value) {
  return String(value || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}
