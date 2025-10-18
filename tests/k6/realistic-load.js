// Realistic Load Test Scenarios for Lesser ActivityPub Implementation
// This test simulates real user journeys and federation patterns

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';
import { randomString, randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

// Custom metrics
const errorRate = new Rate('error_rate');
const responseTime = new Trend('response_time');
const federationLatency = new Trend('federation_latency');
const dbOperations = new Counter('db_operations');
const costTracker = new Counter('estimated_cost_microcents');

// Test configuration
const BASE_URL = __ENV.BASE_URL || 'https://your-lesser-instance.com';
const TEST_DURATION = __ENV.TEST_DURATION || '10m';
const RAMP_UP_TIME = __ENV.RAMP_UP_TIME || '2m';
const RAMP_DOWN_TIME = __ENV.RAMP_DOWN_TIME || '1m';

// Test scenarios configuration
export let options = {
  scenarios: {
    // Scenario 1: User Registration and First Post
    user_onboarding: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: RAMP_UP_TIME, target: 5 },
        { duration: TEST_DURATION, target: 5 },
        { duration: RAMP_DOWN_TIME, target: 0 },
      ],
      exec: 'userOnboardingJourney',
      tags: { scenario: 'onboarding' },
    },

    // Scenario 2: Active User Timeline Browsing
    timeline_browsing: {
      executor: 'ramping-vus',
      startVUs: 5,
      stages: [
        { duration: RAMP_UP_TIME, target: 20 },
        { duration: TEST_DURATION, target: 20 },
        { duration: RAMP_DOWN_TIME, target: 0 },
      ],
      exec: 'timelineBrowsingJourney',
      tags: { scenario: 'timeline' },
    },

    // Scenario 3: Content Creation and Interaction
    content_interaction: {
      executor: 'ramping-vus',
      startVUs: 3,
      stages: [
        { duration: RAMP_UP_TIME, target: 15 },
        { duration: TEST_DURATION, target: 15 },
        { duration: RAMP_DOWN_TIME, target: 0 },
      ],
      exec: 'contentInteractionJourney',
      tags: { scenario: 'interaction' },
    },

    // Scenario 4: Federation Activity
    federation_flow: {
      executor: 'ramping-vus',
      startVUs: 2,
      stages: [
        { duration: RAMP_UP_TIME, target: 8 },
        { duration: TEST_DURATION, target: 8 },
        { duration: RAMP_DOWN_TIME, target: 0 },
      ],
      exec: 'federationJourney',
      tags: { scenario: 'federation' },
    },

    // Scenario 5: Media Upload and Processing
    media_processing: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: RAMP_UP_TIME, target: 5 },
        { duration: TEST_DURATION, target: 5 },
        { duration: RAMP_DOWN_TIME, target: 0 },
      ],
      exec: 'mediaProcessingJourney',
      tags: { scenario: 'media' },
    },

    // Scenario 6: Search and Discovery
    search_discovery: {
      executor: 'ramping-vus',
      startVUs: 2,
      stages: [
        { duration: RAMP_UP_TIME, target: 10 },
        { duration: TEST_DURATION, target: 10 },
        { duration: RAMP_DOWN_TIME, target: 0 },
      ],
      exec: 'searchDiscoveryJourney',
      tags: { scenario: 'search' },
    },

    // Scenario 7: Constant Background Load
    background_load: {
      executor: 'constant-vus',
      vus: 10,
      duration: TEST_DURATION,
      exec: 'backgroundLoad',
      tags: { scenario: 'background' },
    },
  },
  thresholds: {
    // Response time thresholds
    http_req_duration: ['p(95)<2000', 'p(99)<5000'], // 95% under 2s, 99% under 5s
    
    // Error rate thresholds
    error_rate: ['rate<0.05'], // Less than 5% error rate
    
    // Federation latency (should be reasonable for ActivityPub)
    federation_latency: ['p(95)<3000'],
    
    // Overall request success rate
    http_req_failed: ['rate<0.1'], // Less than 10% failures
  },
};

// Authentication helper
function authenticate(username, password) {
  const payload = {
    username: username,
    password: password,
  };

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'User-Agent': 'k6-load-test/1.0',
    },
    tags: { endpoint: 'auth' },
  };

  const response = http.post(`${BASE_URL}/api/v1/auth/login`, JSON.stringify(payload), params);
  
  const success = check(response, {
    'auth successful': (r) => r.status === 200,
    'auth response time OK': (r) => r.timings.duration < 1000,
  });

  if (success && response.json('access_token')) {
    return response.json('access_token');
  }
  
  errorRate.add(!success);
  return null;
}

// Create test user
function createTestUser() {
  const username = `testuser_${randomString(8)}`;
  const email = `${username}@test.local`;
  const password = randomString(12);

  const payload = {
    username: username,
    email: email,
    password: password,
    password_confirmation: password,
    agreement: true,
  };

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'User-Agent': 'k6-load-test/1.0',
    },
    tags: { endpoint: 'registration' },
  };

  const response = http.post(`${BASE_URL}/api/v1/accounts`, JSON.stringify(payload), params);
  
  const success = check(response, {
    'user creation successful': (r) => r.status === 200 || r.status === 201,
    'user creation response time OK': (r) => r.timings.duration < 2000,
  });

  errorRate.add(!success);
  responseTime.add(response.timings.duration);

  if (success) {
    return { username, password, email };
  }
  
  return null;
}

// Scenario 1: User Onboarding Journey
export function userOnboardingJourney() {
  // Step 1: Create account
  const user = createTestUser();
  if (!user) {
    console.log('Failed to create user, skipping onboarding journey');
    return;
  }

  sleep(randomIntBetween(1, 3));

  // Step 2: Authenticate
  const token = authenticate(user.username, user.password);
  if (!token) {
    console.log('Failed to authenticate, skipping rest of onboarding');
    return;
  }

  const authHeaders = {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json',
    'User-Agent': 'k6-load-test/1.0',
  };

  sleep(randomIntBetween(2, 5));

  // Step 3: Update profile
  const profileUpdate = {
    display_name: `Test User ${randomString(6)}`,
    note: `This is a test profile created during load testing. Random: ${randomString(20)}`,
    bot: false,
    discoverable: true,
  };

  const profileResponse = http.patch(
    `${BASE_URL}/api/v1/accounts/update_credentials`,
    JSON.stringify(profileUpdate),
    { headers: authHeaders, tags: { endpoint: 'profile_update' } }
  );

  check(profileResponse, {
    'profile update successful': (r) => r.status === 200,
    'profile update response time OK': (r) => r.timings.duration < 1500,
  });

  sleep(randomIntBetween(3, 7));

  // Step 4: First post
  const firstPost = {
    status: `Hello world! This is my first post from the load test. Random content: ${randomString(50)}`,
    visibility: 'public',
  };

  const postResponse = http.post(
    `${BASE_URL}/api/v1/statuses`,
    JSON.stringify(firstPost),
    { headers: authHeaders, tags: { endpoint: 'first_post' } }
  );

  check(postResponse, {
    'first post successful': (r) => r.status === 200,
    'first post response time OK': (r) => r.timings.duration < 2000,
  });

  responseTime.add(postResponse.timings.duration);
  
  // Step 5: View own timeline
  sleep(randomIntBetween(5, 10));
  
  const timelineResponse = http.get(
    `${BASE_URL}/api/v1/timelines/home?limit=20`,
    { headers: authHeaders, tags: { endpoint: 'home_timeline' } }
  );

  check(timelineResponse, {
    'timeline load successful': (r) => r.status === 200,
    'timeline response time OK': (r) => r.timings.duration < 1000,
  });

  responseTime.add(timelineResponse.timings.duration);
  dbOperations.add(1); // Estimate DB operations
  costTracker.add(randomIntBetween(50, 200)); // Estimate cost in microcents
}

// Scenario 2: Timeline Browsing Journey
export function timelineBrowsingJourney() {
  // Use pre-created test account credentials
  const testUsers = [
    { username: 'loadtest1', password: 'loadtest123' },
    { username: 'loadtest2', password: 'loadtest123' },
    { username: 'loadtest3', password: 'loadtest123' },
  ];

  const user = testUsers[randomIntBetween(0, testUsers.length - 1)];
  const token = authenticate(user.username, user.password);
  
  if (!token) {
    console.log('Failed to authenticate for timeline browsing');
    return;
  }

  const authHeaders = {
    'Authorization': `Bearer ${token}`,
    'User-Agent': 'k6-load-test/1.0',
  };

  // Browse different timelines
  const timelines = [
    'home',
    'public',
    'public?local=true',
  ];

  for (let timeline of timelines) {
    const response = http.get(
      `${BASE_URL}/api/v1/timelines/${timeline}&limit=${randomIntBetween(10, 40)}`,
      { headers: authHeaders, tags: { endpoint: `timeline_${timeline.split('?')[0]}` } }
    );

    const success = check(response, {
      [`${timeline} timeline successful`]: (r) => r.status === 200,
      [`${timeline} timeline response time OK`]: (r) => r.timings.duration < 1500,
    });

    errorRate.add(!success);
    responseTime.add(response.timings.duration);
    dbOperations.add(1);
    costTracker.add(randomIntBetween(20, 100));

    sleep(randomIntBetween(2, 5));
  }

  // Check notifications
  const notificationResponse = http.get(
    `${BASE_URL}/api/v1/notifications?limit=15`,
    { headers: authHeaders, tags: { endpoint: 'notifications' } }
  );

  check(notificationResponse, {
    'notifications successful': (r) => r.status === 200,
    'notifications response time OK': (r) => r.timings.duration < 1000,
  });

  responseTime.add(notificationResponse.timings.duration);
  dbOperations.add(1);
}

// Scenario 3: Content Interaction Journey
export function contentInteractionJourney() {
  const user = { username: 'loadtest1', password: 'loadtest123' };
  const token = authenticate(user.username, user.password);
  
  if (!token) return;

  const authHeaders = {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json',
    'User-Agent': 'k6-load-test/1.0',
  };

  // Create a post
  const newStatus = {
    status: `Load test post ${Date.now()}: ${randomString(100)}. #loadtest #k6 #activitypub`,
    visibility: 'public',
  };

  const postResponse = http.post(
    `${BASE_URL}/api/v1/statuses`,
    JSON.stringify(newStatus),
    { headers: authHeaders, tags: { endpoint: 'create_status' } }
  );

  const postSuccess = check(postResponse, {
    'post creation successful': (r) => r.status === 200,
    'post creation response time OK': (r) => r.timings.duration < 2000,
  });

  if (postSuccess && postResponse.json('id')) {
    const statusId = postResponse.json('id');
    
    sleep(randomIntBetween(3, 8));

    // Get public timeline to find posts to interact with
    const timelineResponse = http.get(
      `${BASE_URL}/api/v1/timelines/public?limit=20`,
      { headers: authHeaders, tags: { endpoint: 'public_timeline' } }
    );

    if (timelineResponse.status === 200) {
      const posts = timelineResponse.json();
      
      if (posts && posts.length > 0) {
        // Interact with a few random posts
        const interactionCount = randomIntBetween(1, 3);
        
        for (let i = 0; i < interactionCount && i < posts.length; i++) {
          const post = posts[i];
          const interactionType = randomIntBetween(1, 3);

          switch (interactionType) {
            case 1: // Favourite
              const favResponse = http.post(
                `${BASE_URL}/api/v1/statuses/${post.id}/favourite`,
                null,
                { headers: authHeaders, tags: { endpoint: 'favourite' } }
              );
              check(favResponse, {
                'favourite successful': (r) => r.status === 200,
              });
              break;

            case 2: // Boost/Reblog
              const boostResponse = http.post(
                `${BASE_URL}/api/v1/statuses/${post.id}/reblog`,
                null,
                { headers: authHeaders, tags: { endpoint: 'reblog' } }
              );
              check(boostResponse, {
                'reblog successful': (r) => r.status === 200,
              });
              break;

            case 3: // Reply
              const reply = {
                status: `@${post.account.username} This is a test reply! ${randomString(30)}`,
                in_reply_to_id: post.id,
              };
              const replyResponse = http.post(
                `${BASE_URL}/api/v1/statuses`,
                JSON.stringify(reply),
                { headers: authHeaders, tags: { endpoint: 'reply' } }
              );
              check(replyResponse, {
                'reply successful': (r) => r.status === 200,
              });
              break;
          }

          sleep(randomIntBetween(2, 5));
          dbOperations.add(1);
          costTracker.add(randomIntBetween(30, 150));
        }
      }
    }
  }

  responseTime.add(postResponse.timings.duration);
  errorRate.add(!postSuccess);
}

// Scenario 4: Federation Journey
export function federationJourney() {
  // Simulate federation activities
  const user = { username: 'loadtest2', password: 'loadtest123' };
  const token = authenticate(user.username, user.password);
  
  if (!token) return;

  const authHeaders = {
    'Authorization': `Bearer ${token}`,
    'User-Agent': 'k6-load-test/1.0',
  };

  // Check webfinger endpoint
  const webfingerResponse = http.get(
    `${BASE_URL}/.well-known/webfinger?resource=acct:${user.username}@${BASE_URL.replace('https://', '')}`,
    { tags: { endpoint: 'webfinger' } }
  );

  const webfingerSuccess = check(webfingerResponse, {
    'webfinger successful': (r) => r.status === 200,
    'webfinger response time OK': (r) => r.timings.duration < 1000,
  });

  federationLatency.add(webfingerResponse.timings.duration);

  // Check user's outbox (ActivityPub)
  const outboxResponse = http.get(
    `${BASE_URL}/users/${user.username}/outbox`,
    { 
      headers: { 'Accept': 'application/activity+json' },
      tags: { endpoint: 'outbox' }
    }
  );

  check(outboxResponse, {
    'outbox accessible': (r) => r.status === 200,
    'outbox response time OK': (r) => r.timings.duration < 1500,
  });

  federationLatency.add(outboxResponse.timings.duration);

  // Search for remote users (federation test)
  const searchQuery = `@test@remote-instance.example.com`;
  const searchResponse = http.get(
    `${BASE_URL}/api/v1/search?q=${encodeURIComponent(searchQuery)}&type=accounts&resolve=true`,
    { headers: authHeaders, tags: { endpoint: 'federation_search' } }
  );

  check(searchResponse, {
    'federation search completed': (r) => r.status === 200 || r.status === 404,
    'federation search response time OK': (r) => r.timings.duration < 3000,
  });

  federationLatency.add(searchResponse.timings.duration);
  errorRate.add(!webfingerSuccess);
  dbOperations.add(2);
  costTracker.add(randomIntBetween(100, 400));

  sleep(randomIntBetween(5, 10));
}

// Scenario 5: Media Processing Journey
export function mediaProcessingJourney() {
  const user = { username: 'loadtest3', password: 'loadtest123' };
  const token = authenticate(user.username, user.password);
  
  if (!token) return;

  const authHeaders = {
    'Authorization': `Bearer ${token}`,
    'User-Agent': 'k6-load-test/1.0',
  };

  // Create a small test image (base64 encoded 1x1 pixel PNG)
  const testImageData = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg==';
  
  const formData = {
    file: http.file(Buffer.from(testImageData, 'base64'), 'test.png', 'image/png'),
    description: 'Test image for load testing',
  };

  const uploadResponse = http.post(
    `${BASE_URL}/api/v1/media`,
    formData,
    { 
      headers: { 'Authorization': `Bearer ${token}` },
      tags: { endpoint: 'media_upload' }
    }
  );

  const uploadSuccess = check(uploadResponse, {
    'media upload successful': (r) => r.status === 200 || r.status === 202,
    'media upload response time OK': (r) => r.timings.duration < 3000,
  });

  if (uploadSuccess && uploadResponse.json('id')) {
    const mediaId = uploadResponse.json('id');
    
    sleep(randomIntBetween(3, 7));

    // Create a post with the uploaded media
    const statusWithMedia = {
      status: `Load test post with media! ${randomString(50)}`,
      media_ids: [mediaId],
      visibility: 'public',
    };

    const postResponse = http.post(
      `${BASE_URL}/api/v1/statuses`,
      JSON.stringify(statusWithMedia),
      { 
        headers: authHeaders,
        tags: { endpoint: 'status_with_media' }
      }
    );

    check(postResponse, {
      'post with media successful': (r) => r.status === 200,
      'post with media response time OK': (r) => r.timings.duration < 2500,
    });

    responseTime.add(postResponse.timings.duration);
  }

  responseTime.add(uploadResponse.timings.duration);
  errorRate.add(!uploadSuccess);
  dbOperations.add(2);
  costTracker.add(randomIntBetween(200, 800)); // Media operations are more expensive
}

// Scenario 6: Search and Discovery Journey
export function searchDiscoveryJourney() {
  const user = { username: 'loadtest1', password: 'loadtest123' };
  const token = authenticate(user.username, user.password);
  
  if (!token) return;

  const authHeaders = {
    'Authorization': `Bearer ${token}`,
    'User-Agent': 'k6-load-test/1.0',
  };

  const searchQueries = [
    'test',
    '#loadtest',
    '@loadtest1',
    'activitypub',
    'federation',
    'hello world',
  ];

  for (let query of searchQueries) {
    const searchResponse = http.get(
      `${BASE_URL}/api/v1/search?q=${encodeURIComponent(query)}&limit=20`,
      { headers: authHeaders, tags: { endpoint: 'search' } }
    );

    const success = check(searchResponse, {
      [`search for "${query}" successful`]: (r) => r.status === 200,
      [`search for "${query}" response time OK`]: (r) => r.timings.duration < 2000,
    });

    errorRate.add(!success);
    responseTime.add(searchResponse.timings.duration);
    dbOperations.add(1);
    costTracker.add(randomIntBetween(50, 250));

    sleep(randomIntBetween(2, 5));
  }

  // Check trending hashtags
  const trendsResponse = http.get(
    `${BASE_URL}/api/v1/trends/tags?limit=10`,
    { headers: authHeaders, tags: { endpoint: 'trends' } }
  );

  check(trendsResponse, {
    'trends successful': (r) => r.status === 200,
    'trends response time OK': (r) => r.timings.duration < 1500,
  });

  responseTime.add(trendsResponse.timings.duration);
}

// Scenario 7: Background Load
export function backgroundLoad() {
  // Simulate basic user activity - checking timelines, notifications
  const user = { username: 'loadtest2', password: 'loadtest123' };
  const token = authenticate(user.username, user.password);
  
  if (!token) return;

  const authHeaders = {
    'Authorization': `Bearer ${token}`,
    'User-Agent': 'k6-load-test/1.0',
  };

  // Randomly choose an endpoint to hit
  const endpoints = [
    { url: '/api/v1/timelines/home?limit=10', name: 'home_timeline_bg' },
    { url: '/api/v1/notifications?limit=5', name: 'notifications_bg' },
    { url: '/api/v1/accounts/verify_credentials', name: 'verify_credentials_bg' },
    { url: '/api/v1/timelines/public?local=true&limit=15', name: 'local_timeline_bg' },
  ];

  const endpoint = endpoints[randomIntBetween(0, endpoints.length - 1)];
  
  const response = http.get(
    `${BASE_URL}${endpoint.url}`,
    { headers: authHeaders, tags: { endpoint: endpoint.name } }
  );

  const success = check(response, {
    'background load successful': (r) => r.status === 200,
    'background load response time OK': (r) => r.timings.duration < 1000,
  });

  errorRate.add(!success);
  responseTime.add(response.timings.duration);
  dbOperations.add(0.5); // Lighter DB load
  costTracker.add(randomIntBetween(10, 50));

  sleep(randomIntBetween(5, 15)); // Longer sleep for background load
}

// Test teardown
export function teardown(data) {
  console.log('Load test completed');
  console.log(`Total estimated cost: ${costTracker.count} microcents`);
  console.log(`Total DB operations: ${dbOperations.count}`);
}

// Setup function to prepare test environment
export function setup() {
  console.log(`Starting realistic load test against ${BASE_URL}`);
  console.log(`Test duration: ${TEST_DURATION}`);
  console.log(`Ramp up time: ${RAMP_UP_TIME}`);
  
  // Verify that the instance is accessible
  const healthCheck = http.get(`${BASE_URL}/health`);
  if (healthCheck.status !== 200) {
    console.error(`Health check failed: ${healthCheck.status}`);
    throw new Error('Instance is not accessible');
  }
  
  console.log('Health check passed, starting load test...');
}