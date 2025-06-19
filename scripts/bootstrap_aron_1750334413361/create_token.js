const crypto = require('crypto');

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
    sub: 'aron',
    iat: now,
    exp: now + 3600, // 1 hour
    nbf: now,
    
    // Lesser custom claims (must match auth.Claims struct)
    username: 'aron',
    scopes: ['read', 'write', 'follow', 'push'],
    client_id: 'tkxavX1G9m0Fng2ZiJ7kQt'
};

const secret = process.env.JWT_SECRET || 'seNNAR+4jKG6vSoxGBZ9GYuBx7scopVcS1fE6enobEI=';
if (!secret) {
    console.error('ERROR: JWT_SECRET environment variable not set');
    process.exit(1);
}

const token = createJWT(payload, secret);

console.log('Bearer ' + token);
console.error('
Token generated successfully!');
console.error('Expires in 1 hour');
console.error('
Test with:');
console.error('curl -H "Authorization: Bearer ' + token + '" https://lesser.host/api/v1/accounts/verify_credentials');
