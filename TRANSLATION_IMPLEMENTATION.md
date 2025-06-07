# Translation Service Implementation

## Overview

Lesser now supports real-time translation of status content using AWS Translate. The implementation provides:

1. **Auto-detection** of source language
2. **User preference** based target language selection
3. **DynamoDB caching** for cost efficiency
4. **HTML content** preservation (basic)
5. **Graceful fallback** to mock translation when AWS Translate is not configured

## Configuration

### Environment Variables

```bash
# Enable AWS Translate integration
TRANSLATION_ENABLED=true

# AWS credentials (standard AWS SDK environment variables)
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=your-key-id
AWS_SECRET_ACCESS_KEY=your-secret-key
```

### IAM Permissions Required

The Lambda function needs the following AWS Translate permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "translate:TranslateText",
        "translate:ListLanguages"
      ],
      "Resource": "*"
    }
  ]
}
```

## API Endpoints

### Translate Status

**POST** `/api/v1/statuses/:id/translate`

Translates a status to the user's preferred language.

**Response:**
```json
{
  "content": "Translated text content",
  "spoiler_text": "Translated spoiler text",
  "detected_source_language": "es",
  "provider": "AWS Translate"
}
```

### Get Translation Languages

**GET** `/api/v1/instance/translation_languages`

Returns the list of supported translation languages.

**Response:**
```json
[
  {
    "code": "en",
    "name": "English"
  },
  {
    "code": "es", 
    "name": "Spanish"
  }
  // ... more languages
]
```

## Implementation Details

### Translation Service (`pkg/translation/aws_translate.go`)

- Uses AWS SDK v2 for Go
- Implements DynamoDB caching with TTL (7 days for translations, 24 hours for language list)
- Handles HTML content (basic tag stripping)
- Auto-detects source language when not specified
- Uses MD5 hash of text for cache keys to keep DynamoDB item size reasonable

### DynamoDB Cache Schema

```
# Translation Cache
PK: CACHE#TRANSLATION
SK: translation:<text_hash>:<source_lang>:<target_lang>
Attributes:
  - TranslatedText: String
  - DetectedLanguage: String
  - CachedAt: ISO8601 timestamp
  - TTL: Unix timestamp (7 days from creation)

# Language List Cache  
PK: CACHE#LANGUAGES
SK: SUPPORTED
Attributes:
  - Languages: List of {Code, Name} objects
  - CachedAt: ISO8601 timestamp
  - TTL: Unix timestamp (24 hours from creation)
```

### Handler Integration (`cmd/api/handlers/translation.go`)

- Checks `TRANSLATION_ENABLED` environment variable
- Falls back to mock translation when disabled
- Uses user's language preference from profile
- Translates both content and spoiler text
- Graceful error handling with appropriate HTTP responses

## Cost Considerations

AWS Translate pricing (as of 2025):
- **First 2 million characters/month**: Free tier
- **Beyond free tier**: $15 per million characters

DynamoDB caching significantly reduces costs:
- Translations are cached for 7 days
- Popular content only needs to be translated once
- Cache hits avoid AWS Translate API calls

For a small instance with 100 active users:
- Average status: ~200 characters
- If 10% of statuses are translated: ~20 translations/user/day
- With 50% cache hit rate: ~10 API calls/user/day
- Monthly cost: ~$0.30/user (well within free tier)

## Future Enhancements

1. **Advanced HTML Preservation**: Use proper HTML parser to maintain formatting
2. **Batch Translation**: Optimize for timeline translation requests
3. **Custom Terminology**: Support instance-specific terminology
4. **Language Detection**: Store detected language with status metadata
5. **Alternative Providers**: Support for DeepL, Google Translate, LibreTranslate
6. **Translation Metrics**: Track cache hit rates and popular translation pairs

## Testing

To test the translation service:

```bash
# Set environment variable
export TRANSLATION_ENABLED=true

# Ensure AWS credentials are configured
aws configure

# Test translation
curl -X POST https://your-instance.com/api/v1/statuses/123/translate \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Monitoring

Monitor translation usage via CloudWatch:
- Track API calls to AWS Translate
- Monitor translation latency
- Track DynamoDB cache hit/miss rates
- Set up billing alerts for translation costs

## Security Considerations

1. **Content Filtering**: Consider filtering sensitive content before translation
2. **Rate Limiting**: Implement per-user translation limits
3. **Cost Controls**: Set AWS budget alerts
4. **Data Residency**: Be aware of data processing locations for compliance
5. **Cache Key Security**: MD5 hashes prevent cache poisoning attacks 