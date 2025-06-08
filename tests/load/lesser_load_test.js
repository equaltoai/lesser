import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const costPerRequest = new Rate('cost_per_request');

// Test configuration
export let options = {
  // Cloud options for Grafana k6 Cloud
  ext: {
    loadimpact: {
      projectID: __ENV.K6_PROJECT_ID, // You can set this via environment variable
      name: 'Lesser Load Test',
      distribution: {
        // You can specify different load zones if needed
        'amazon:us:ashburn': { loadZone: 'amazon:us:ashburn', percent: 100 }
      }
    }
  },
  
  stages: [
    { duration: '30s', target: 10 },    // Ramp up to 10 users
    { duration: '1m', target: 50 },     // Ramp up to 50 users
    { duration: '3m', target: 100 },    // Stay at 100 users
    { duration: '1m', target: 200 },    // Spike to 200 users
    { duration: '3m', target: 100 },    // Back to 100 users
    { duration: '1m', target: 0 },      // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'], // 95% of requests must complete below 500ms
    errors: ['rate<0.1'],             // Error rate must be below 10%
  },
};

const BASE_URL = __ENV.LESSER_URL || 'https://lesser.example.com';
const ACCESS_TOKEN = __ENV.LESSER_TOKEN || '';

const headers = {
  'Authorization': `Bearer ${ACCESS_TOKEN}`,
  'Content-Type': 'application/json',
};

// Test scenarios
export default function () {
  const scenario = Math.random();
  
  if (scenario < 0.3) {
    // 30% - Read timeline
    testHomeTimeline();
  } else if (scenario < 0.5) {
    // 20% - Read public timeline
    testPublicTimeline();
  } else if (scenario < 0.7) {
    // 20% - Create status
    testCreateStatus();
  } else if (scenario < 0.85) {
    // 15% - Search
    testSearch();
  } else {
    // 15% - Notifications
    testNotifications();
  }
  
  sleep(1);
}

function testHomeTimeline() {
  const response = http.get(`${BASE_URL}/api/v1/timelines/home?limit=20`, { headers });
  
  const success = check(response, {
    'home timeline status is 200': (r) => r.status === 200,
    'home timeline has cost headers': (r) => r.headers['X-Cost-Total-Micros'] !== undefined,
    'home timeline loads fast': (r) => r.timings.duration < 200,
  });
  
  errorRate.add(!success);
  
  if (response.headers['X-Cost-Total-Cents']) {
    costPerRequest.add(parseFloat(response.headers['X-Cost-Total-Cents']));
  }
}

function testPublicTimeline() {
  const response = http.get(`${BASE_URL}/api/v1/timelines/public?local=true&limit=20`, { headers });
  
  const success = check(response, {
    'public timeline status is 200': (r) => r.status === 200,
    'public timeline returns array': (r) => Array.isArray(r.json()),
  });
  
  errorRate.add(!success);
}

function testCreateStatus() {
  const payload = JSON.stringify({
    status: `Load test status ${Date.now()} #loadtest`,
    visibility: 'public',
  });
  
  const response = http.post(`${BASE_URL}/api/v1/statuses`, payload, { headers });
  
  const success = check(response, {
    'create status is 200': (r) => r.status === 200,
    'create status returns id': (r) => r.json().id !== undefined,
    'create status has cost headers': (r) => r.headers['X-Cost-Total-Micros'] !== undefined,
  });
  
  errorRate.add(!success);
  
  // Delete the status to avoid spam
  if (success && response.json().id) {
    http.del(`${BASE_URL}/api/v1/statuses/${response.json().id}`, null, { headers });
  }
}

function testSearch() {
  const queries = ['test', 'lesser', 'activitypub', 'federation', '@admin'];
  const query = queries[Math.floor(Math.random() * queries.length)];
  
  const response = http.get(`${BASE_URL}/api/v2/search?q=${query}&resolve=false`, { headers });
  
  const success = check(response, {
    'search status is 200': (r) => r.status === 200,
    'search returns results': (r) => r.json() !== null,
  });
  
  errorRate.add(!success);
}

function testNotifications() {
  const response = http.get(`${BASE_URL}/api/v1/notifications?limit=20`, { headers });
  
  const success = check(response, {
    'notifications status is 200': (r) => r.status === 200,
    'notifications returns array': (r) => Array.isArray(r.json()),
  });
  
  errorRate.add(!success);
}

// Lifecycle hooks
export function setup() {
  // Verify the instance is reachable
  const response = http.get(`${BASE_URL}/api/v1/instance`);
  
  if (response.status !== 200) {
    throw new Error(`Instance not reachable: ${response.status}`);
  }
  
  console.log(`Testing instance: ${response.json().domain}`);
  console.log(`Version: ${response.json().version}`);
  
  return { startTime: Date.now() };
}

export function teardown(data) {
  const duration = Date.now() - data.startTime;
  console.log(`Test completed in ${duration}ms`);
}

// Custom summary
export function handleSummary(data) {
  const customData = {
    'Total Requests': data.metrics.http_reqs.values.count,
    'Average RPS': data.metrics.http_reqs.values.rate,
    'Average Duration': data.metrics.http_req_duration.values.avg,
    'P95 Duration': data.metrics.http_req_duration.values['p(95)'],
    'Error Rate': data.metrics.errors.values.rate,
    'Total Data Received': `${(data.metrics.data_received.values.count / 1024 / 1024).toFixed(2)} MB`,
  };
  
  return {
    'stdout': textSummary(data, { indent: ' ', enableColors: true }),
    'summary.json': JSON.stringify(customData, null, 2),
  };
}

function textSummary(data, options) {
  return `
Lesser Load Test Results
========================
Total Requests: ${data.metrics.http_reqs.values.count}
Average RPS: ${data.metrics.http_reqs.values.rate.toFixed(2)}
Average Duration: ${data.metrics.http_req_duration.values.avg.toFixed(2)}ms
P95 Duration: ${data.metrics.http_req_duration.values['p(95)'].toFixed(2)}ms
Error Rate: ${(data.metrics.errors.values.rate * 100).toFixed(2)}%
Total Data: ${(data.metrics.data_received.values.count / 1024 / 1024).toFixed(2)} MB

Cost Analysis (if available):
Average Cost per Request: ${data.metrics.cost_per_request && data.metrics.cost_per_request.values && data.metrics.cost_per_request.values.avg ? data.metrics.cost_per_request.values.avg.toFixed(6) : 'N/A'} cents
`;
} 