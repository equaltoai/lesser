// Package constructs provides AWS CDK constructs for Lift applications.
//
// This package contains high-level CDK constructs that implement Lift's best practices
// for AWS infrastructure. The constructs include optimized configurations for API
// Gateway, Lambda functions, DynamoDB tables, and other AWS services.
package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2authorizers"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// APIKeyAuthorizerProps defines properties for the API key authorizer.
//
// This struct contains all configurable properties for creating an API key
// authorizer for HTTP APIs. The properties include API key source and parameter
// configuration, validator function, DynamoDB table for API key storage, and
// caching settings.
type APIKeyAuthorizerProps struct {
	// API key parameter source (header or query)
	APIKeySource *string `json:"apiKeySource"`
	// Parameter name (e.g., "X-API-Key" for header or "apiKey" for query)
	APIKeyParameter *string `json:"apiKeyParameter"`
	// Optional function to validate API keys (if not provided, creates one)
	ValidatorFunction awslambda.IFunction `json:"validatorFunction"`
	// DynamoDB table name for storing API keys (optional)
	APIKeyTableName *string `json:"apiKeyTableName"`
	// Cache results for this many seconds (0-3600)
	ResultsCacheTtl *float64 `json:"resultsCacheTtl"`
}

// APIKeyAuthorizer provides API key authentication for HTTP APIs.
//
// This construct creates an API key authorizer for HTTP APIs that validates API
// keys against a DynamoDB table. It includes a Lambda function for validation and
// caching of API key validation results.
type APIKeyAuthorizer struct {
	constructs.Construct
	Authorizer        awsapigatewayv2.IHttpRouteAuthorizer
	ValidatorFunction awslambda.IFunction
}

// NewAPIKeyAuthorizer creates a new API key authorizer.
//
// This function creates a new API key authorizer with the following features:
// - Configurable API key source (header or query parameter)
// - Customizable API key parameter name
// - Optional custom validator function
// - DynamoDB table for API key storage
// - Caching of validation results
//
// Parameters:
//   - scope: The CDK construct scope
//   - id: The construct ID
//   - props: Configuration properties
//
// Returns:
//   - A new APIKeyAuthorizer instance
func NewAPIKeyAuthorizer(scope constructs.Construct, id *string, props *APIKeyAuthorizerProps) *APIKeyAuthorizer {
	this := constructs.NewConstruct(scope, id)

	auth := &APIKeyAuthorizer{
		Construct: this,
	}

	// Set defaults
	if props.APIKeySource == nil {
		props.APIKeySource = jsii.String("header")
	}
	if props.APIKeyParameter == nil {
		props.APIKeyParameter = jsii.String("X-API-Key")
	}
	if props.ResultsCacheTtl == nil {
		props.ResultsCacheTtl = jsii.Number(300) // 5 minutes default
	}

	// Create or use validator function
	if props.ValidatorFunction != nil {
		auth.ValidatorFunction = props.ValidatorFunction
	} else {
		auth.ValidatorFunction = auth.createValidatorFunction(props)
	}

	// Create the Lambda authorizer
	auth.Authorizer = awsapigatewayv2authorizers.NewHttpLambdaAuthorizer(
		jsii.String("APIKeyAuthorizer"),
		auth.ValidatorFunction,
		&awsapigatewayv2authorizers.HttpLambdaAuthorizerProps{
			ResponseTypes: &[]awsapigatewayv2authorizers.HttpLambdaResponseType{
				awsapigatewayv2authorizers.HttpLambdaResponseType_SIMPLE,
			},
			ResultsCacheTtl: awscdk.Duration_Seconds(props.ResultsCacheTtl),
			IdentitySource: &[]*string{
				jsii.String(auth.getIdentitySource(props)),
			},
		},
	)

	return auth
}

// createValidatorFunction creates the Lambda function that validates API keys.
//
// This method creates a Lambda function that validates API keys against a
// DynamoDB table. The function includes caching and usage tracking.
//
// Parameters:
//   - props: Configuration properties
//
// Returns:
//   - A Lambda function that validates API keys
func (auth *APIKeyAuthorizer) createValidatorFunction(props *APIKeyAuthorizerProps) awslambda.IFunction {
	tableName := "api-keys"
	if props.APIKeyTableName != nil {
		tableName = *props.APIKeyTableName
	}

	code := generateAPIKeyValidatorCode(*props.APIKeySource, *props.APIKeyParameter, tableName)

	fn := NewLiftFunction(auth, jsii.String("ValidatorFunction"), &LiftFunctionProps{
		FunctionProps: awslambda.FunctionProps{
			Runtime:    awslambda.Runtime_NODEJS_18_X(),
			Handler:    jsii.String("index.handler"),
			Code:       awslambda.Code_FromInline(jsii.String(code)),
			MemorySize: jsii.Number(256),
			Timeout:    awscdk.Duration_Seconds(jsii.Number(10)),
			Environment: &map[string]*string{
				"API_KEY_TABLE":     jsii.String(tableName),
				"API_KEY_SOURCE":    props.APIKeySource,
				"API_KEY_PARAMETER": props.APIKeyParameter,
			},
		},
	})
	return fn.Function
}

// getIdentitySource builds the identity source string.
//
// This method constructs the identity source string based on the API key source
// (header or query parameter) and parameter name.
//
// Parameters:
//   - props: Configuration properties
//
// Returns:
//   - The identity source string for the authorizer
func (auth *APIKeyAuthorizer) getIdentitySource(props *APIKeyAuthorizerProps) string {
	if *props.APIKeySource == "header" {
		return "$request.header." + *props.APIKeyParameter
	}
	return "$request.querystring." + *props.APIKeyParameter
}

// generateAPIKeyValidatorCode generates the Lambda code for API key validation.
//
// This function generates the JavaScript code for a Lambda function that validates
// API keys against a DynamoDB table. The function includes:
// - API key extraction from request
// - DynamoDB validation
// - Caching of validation results
// - Usage tracking
// - Policy generation
//
// Parameters:
//   - apiKeySource: The source of the API key (header or query)
//   - apiKeyParameter: The parameter name
//   - tableName: The DynamoDB table name
//
// Returns:
//   - The JavaScript code for the validator function
func generateAPIKeyValidatorCode(_, _, _ string) string {
	return `const AWS = require('aws-sdk');
const dynamodb = new AWS.DynamoDB.DocumentClient();

// Cache valid API keys for better performance
const apiKeyCache = new Map();
const CACHE_TTL = 300000; // 5 minutes

exports.handler = async (event) => {
    console.log('Auth event:', JSON.stringify(event, null, 2));

    try {
        // Extract API key from request
        const apiKey = extractAPIKey(event);

        if (!apiKey) {
            console.log('No API key provided');
            return generatePolicy('user', 'Deny', event.methodArn);
        }

        // Check cache first
        const cachedResult = apiKeyCache.get(apiKey);
        if (cachedResult && cachedResult.expires > Date.now()) {
            console.log('Using cached result for API key');
            return generatePolicy(cachedResult.principalId, 'Allow', event.methodArn, cachedResult.context);
        }

        // Validate API key against DynamoDB
        const result = await validateAPIKey(apiKey);

        if (!result.valid) {
            console.log('Invalid API key');
            return generatePolicy('user', 'Deny', event.methodArn);
        }

        // Cache the result
        apiKeyCache.set(apiKey, {
            principalId: result.principalId,
            context: result.context,
            expires: Date.now() + CACHE_TTL
        });

        // Clean up old cache entries
        if (apiKeyCache.size > 1000) {
            const now = Date.now();
            for (const [key, value] of apiKeyCache.entries()) {
                if (value.expires < now) {
                    apiKeyCache.delete(key);
                }
            }
        }

        return generatePolicy(result.principalId, 'Allow', event.methodArn, result.context);
    } catch (error) {
        console.error('Auth error:', error);
        // In case of error, deny access
        return generatePolicy('user', 'Deny', event.methodArn);
    }
};

function extractAPIKey(event) {
    const source = process.env.API_KEY_SOURCE;
    const parameter = process.env.API_KEY_PARAMETER;

    if (source === 'header') {
        return event.headers && event.headers[parameter];
    } else if (source === 'query') {
        return event.queryStringParameters && event.queryStringParameters[parameter];
    }

    return null;
}

async function validateAPIKey(apiKey) {
    try {
        const params = {
            TableName: process.env.API_KEY_TABLE,
            Key: { apiKey: apiKey }
        };

        const result = await dynamodb.get(params).promise();

        if (!result.Item) {
            return { valid: false };
        }

        // Check if key is active and not expired
        const now = new Date().toISOString();
        if (result.Item.status !== 'active' ||
            (result.Item.expiresAt && result.Item.expiresAt < now)) {
            return { valid: false };
        }

        // Update last used timestamp
        await dynamodb.update({
            TableName: process.env.API_KEY_TABLE,
            Key: { apiKey: apiKey },
            UpdateExpression: 'SET lastUsed = :now, usageCount = usageCount + :inc',
            ExpressionAttributeValues: {
                ':now': now,
                ':inc': 1
            }
        }).promise().catch(err => {
            // Don't fail auth if we can't update usage stats
            console.error('Failed to update usage stats:', err);
        });

        return {
            valid: true,
            principalId: result.Item.userId || result.Item.apiKey,
            context: {
                userId: result.Item.userId,
                tenantId: result.Item.tenantId,
                scope: result.Item.scope || 'default',
                keyId: result.Item.keyId
            }
        };
    } catch (error) {
        console.error('Error validating API key:', error);
        return { valid: false };
    }
}

function generatePolicy(principalId, effect, resource, context = {}) {
    const authResponse = {
        principalId: principalId,
        policyDocument: {
            Version: '2012-10-17',
            Statement: [
                {
                    Action: 'execute-api:Invoke',
                    Effect: effect,
                    Resource: resource
                }
            ]
        }
    };

    // Add context if provided (for passing data to Lambda)
    if (Object.keys(context).length > 0) {
        authResponse.context = context;
    }

    return authResponse;
}`
}
