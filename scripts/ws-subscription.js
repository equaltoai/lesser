#!/usr/bin/env node

/**
 * GraphQL WebSocket validation helper.
 *
 * Example:
 *   node scripts/ws-subscription.js \
 *     --token "$TOKEN" \
 *     --url wss://graphql-ws.dev.lesser.host \
 *     --subscription timeline \
 *     --once
 */

const { parseArgs } = require('node:util');
const process = require('node:process');
const WebSocket = require('ws');

const subscriptions = {
  timeline: {
    query: /* GraphQL */ `
      subscription TimelineUpdates($type: TimelineType!, $listId: ID) {
        timelineUpdates(type: $type, listId: $listId) {
          id
          type
          content
          sensitive
          createdAt
          actor {
            id
            username
          }
          attachments {
            id
            type
            url
          }
        }
      }
    `,
    variables: { type: 'HOME' },
  },
  notifications: {
    query: /* GraphQL */ `
      subscription Notifications {
        notifications {
          id
          type
          createdAt
          actor {
            id
            username
          }
          status {
            id
            content
          }
        }
      }
    `,
    variables: undefined,
  },
};

const argSpec = {
  options: {
    url: { type: 'string', default: process.env.GRAPHQL_WS_URL || 'wss://graphql-ws.dev.lesser.host' },
    token: { type: 'string', default: process.env.GRAPHQL_WS_TOKEN },
    id: { type: 'string', default: 'sub-1' },
    query: { type: 'string' },
    variables: { type: 'string' },
    subscription: { type: 'string' },
    once: { type: 'boolean', default: false },
    timeout: { type: 'string' },
    verbose: { type: 'boolean', default: false },
  },
  allowPositionals: false,
};

const { values } = parseArgs(argSpec);

const url = values.url;
const token = values.token;
const subKey = values.subscription ?? 'timeline';

if (!token) {
  console.error('error: missing auth token. pass via --token or GRAPHQL_WS_TOKEN env var');
  process.exit(1);
}

let query = values.query;
let variables;

if (!query) {
  const preset = subscriptions[subKey];
  if (!preset) {
    console.error(`error: unknown subscription preset "${subKey}". Available: ${Object.keys(subscriptions).join(', ')}`);
    process.exit(1);
  }
  query = preset.query;
  variables = preset.variables;
} else if (values.subscription) {
  console.warn('warning: both --query and --subscription provided; using custom query');
}

if (!variables && subscriptions[subKey]?.variables) {
  variables = subscriptions[subKey].variables;
}

if (values.variables) {
  try {
    variables = JSON.parse(values.variables);
  } catch (err) {
    console.error(`error: failed to parse --variables JSON (${err.message})`);
    process.exit(1);
  }
}

const timeoutMs = values.timeout ? Number(values.timeout) * 1000 : undefined;
if (timeoutMs !== undefined && Number.isNaN(timeoutMs)) {
  console.error('error: --timeout must be numeric seconds');
  process.exit(1);
}

const ws = new WebSocket(url, {
  headers: {
    Authorization: `Bearer ${token}`,
  },
});

let isAcked = false;
let isClosed = false;
let exitTimer;

function logPayload(direction, payload) {
  const prefix = direction === '>' ? '[ws =>' : '[ws <=';
  if (values.verbose) {
    console.log(`${prefix} ${JSON.stringify(payload)}`);
  }
}

function send(message) {
  if (ws.readyState !== WebSocket.OPEN) return;
  logPayload('>', message);
  ws.send(JSON.stringify(message));
}

ws.on('open', () => {
  console.log(`connected to ${url}`);
  send({ type: 'connection_init', payload: { timestamp: new Date().toISOString() } });
});

ws.on('message', (raw) => {
  let message;
  try {
    message = JSON.parse(raw.toString());
  } catch (err) {
    console.warn('ignored non-JSON payload:', raw.toString());
    return;
  }

  logPayload('<', message);

  switch (message.type) {
    case 'connection_ack':
      isAcked = true;
      console.log('connection acknowledged');
      send({
        id: values.id,
        type: 'subscribe',
        payload: {
          query,
          variables,
        },
      });
      if (timeoutMs) {
        exitTimer = setTimeout(() => {
          console.log(`timeout reached (${timeoutMs / 1000}s); closing`);
          send({ id: values.id, type: 'complete' });
          ws.close(1000, 'timeout');
        }, timeoutMs);
      }
      break;

    case 'next':
      console.log('Received data:');
      console.log(JSON.stringify(message.payload, null, 2));
      if (values.once) {
        console.log('--once supplied; completing subscription');
        send({ id: message.id || values.id, type: 'complete' });
        ws.close(1000, 'completed');
      }
      break;

    case 'complete':
      console.log('subscription completed by server');
      ws.close(1000, 'complete');
      break;

    case 'ping':
      send({ type: 'pong' });
      break;

    case 'keepalive':
    case 'ka':
      // ignore keepalives
      break;

    case 'error':
      console.error('server reported error:', JSON.stringify(message.payload, null, 2));
      break;

    default:
      if (!values.verbose) {
        console.log('message:', JSON.stringify(message));
      }
      break;
  }
});

ws.on('close', (code, reason) => {
  if (exitTimer) {
    clearTimeout(exitTimer);
  }
  isClosed = true;
  const textReason = reason?.toString();
  console.log(`connection closed (code=${code}${textReason ? ` reason="` + textReason + `"` : ''})`);
});

ws.on('error', (err) => {
  if (!isClosed) {
    console.error('websocket error:', err);
    process.exitCode = 1;
  }
});

setTimeout(() => {
  if (!isAcked) {
    console.error('error: did not receive connection_ack within 5 seconds');
    ws.close(1011, 'ack timeout');
    process.exitCode = 1;
  }
}, 5000);
