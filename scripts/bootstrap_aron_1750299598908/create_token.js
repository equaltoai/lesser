const crypto = require('crypto');

// Generate JWT token
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

const payload = {
    sub: 'aron',
    username: 'aron',
    client_id: 'IyXI7uRvCQDA1vmpjNURm6',
    scopes: ['read', 'write', 'follow', 'push'],
    iat: Math.floor(Date.now() / 1000),
    exp: Math.floor(Date.now() / 1000) + 3600,
    nbf: Math.floor(Date.now() / 1000)
};

const secret = process.env.JWT_SECRET || 'seNNAR+4jKG6vSoxGBZ9GYuBx7scopVcS1fE6enobEI=';
const token = createJWT(payload, secret);

console.log('Bearer ' + token);
