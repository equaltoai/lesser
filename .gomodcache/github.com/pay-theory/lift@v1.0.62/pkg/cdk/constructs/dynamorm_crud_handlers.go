package constructs

// generateCRUDHandler generates the actual Lambda handler code for CRUD operations
func generateCRUDHandler(operation string) string {
	switch operation {
	case "create":
		return generateCreateHandler()
	case "read":
		return generateReadHandler()
	case "update":
		return generateUpdateHandler()
	case "delete":
		return generateDeleteHandler()
	case "list":
		return generateListHandler()
	case "search":
		return generateSearchHandler()
	default:
		return generateDefaultHandler()
	}
}

// generateCreateHandler generates the CREATE operation handler
func generateCreateHandler() string {
	return `const AWS = require('aws-sdk');
const dynamodb = new AWS.DynamoDB.DocumentClient();

exports.handler = async (event) => {
    console.log('CREATE Event:', JSON.stringify(event, null, 2));

    try {
        // Parse request body
        const body = JSON.parse(event.body || '{}');

        // Validate required fields
        if (!body.id) {
            return {
                statusCode: 400,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ error: 'Missing required field: id' })
            };
        }

        // Add metadata
        const item = {
            ...body,
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString()
        };

        // If multi-tenant, add tenant ID
        if (process.env.ENABLE_MULTI_TENANT === 'true' && event.requestContext?.authorizer?.tenantId) {
            item.tenantId = event.requestContext.authorizer.tenantId;
        }

        // Put item in DynamoDB
        const params = {
            TableName: process.env.TABLE_NAME,
            Item: item,
            ConditionExpression: 'attribute_not_exists(id)'
        };

        await dynamodb.put(params).promise();

        return {
            statusCode: 201,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(item)
        };
    } catch (error) {
        console.error('Error:', error);

        if (error.code === 'ConditionalCheckFailedException') {
            return {
                statusCode: 409,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ error: 'Item already exists' })
            };
        }

        return {
            statusCode: 500,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ error: 'Internal server error' })
        };
    }
};`
}

// generateReadHandler generates the READ operation handler
func generateReadHandler() string {
	return `const AWS = require('aws-sdk');
const dynamodb = new AWS.DynamoDB.DocumentClient();

exports.handler = async (event) => {
    console.log('READ Event:', JSON.stringify(event, null, 2));

    try {
        // Get ID from path parameters
        const id = event.pathParameters?.id;

        if (!id) {
            return {
                statusCode: 400,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ error: 'Missing required parameter: id' })
            };
        }

        // Build query parameters
        const params = {
            TableName: process.env.TABLE_NAME,
            Key: { id }
        };

        // If multi-tenant, add tenant ID to key
        if (process.env.ENABLE_MULTI_TENANT === 'true' && event.requestContext?.authorizer?.tenantId) {
            params.Key.tenantId = event.requestContext.authorizer.tenantId;
        }

        // Get item from DynamoDB
        const result = await dynamodb.get(params).promise();

        if (!result.Item) {
            return {
                statusCode: 404,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ error: 'Item not found' })
            };
        }

        return {
            statusCode: 200,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(result.Item)
        };
    } catch (error) {
        console.error('Error:', error);

        return {
            statusCode: 500,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ error: 'Internal server error' })
        };
    }
};`
}

// generateUpdateHandler generates the UPDATE operation handler
func generateUpdateHandler() string {
	return `const AWS = require('aws-sdk');
const dynamodb = new AWS.DynamoDB.DocumentClient();

exports.handler = async (event) => {
    console.log('UPDATE Event:', JSON.stringify(event, null, 2));

    try {
        // Get ID from path parameters
        const id = event.pathParameters?.id;

        if (!id) {
            return {
                statusCode: 400,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ error: 'Missing required parameter: id' })
            };
        }

        // Parse request body
        const body = JSON.parse(event.body || '{}');

        // Remove id from updates (can't update primary key)
        delete body.id;
        delete body.tenantId;

        // Build update expression
        const updateExpressions = [];
        const expressionAttributeNames = {};
        const expressionAttributeValues = {};

        Object.keys(body).forEach((key, index) => {
            const placeholder = '#attr' + index;
            const valuePlaceholder = ':val' + index;

            updateExpressions.push(placeholder + ' = ' + valuePlaceholder);
            expressionAttributeNames[placeholder] = key;
            expressionAttributeValues[valuePlaceholder] = body[key];
        });

        // Add updatedAt
        updateExpressions.push('#updatedAt = :updatedAt');
        expressionAttributeNames['#updatedAt'] = 'updatedAt';
        expressionAttributeValues[':updatedAt'] = new Date().toISOString();

        // Build update parameters
        const params = {
            TableName: process.env.TABLE_NAME,
            Key: { id },
            UpdateExpression: 'SET ' + updateExpressions.join(', '),
            ExpressionAttributeNames: expressionAttributeNames,
            ExpressionAttributeValues: expressionAttributeValues,
            ConditionExpression: 'attribute_exists(id)',
            ReturnValues: 'ALL_NEW'
        };

        // If multi-tenant, add tenant ID to key
        if (process.env.ENABLE_MULTI_TENANT === 'true' && event.requestContext?.authorizer?.tenantId) {
            params.Key.tenantId = event.requestContext.authorizer.tenantId;
        }

        // Update item in DynamoDB
        const result = await dynamodb.update(params).promise();

        return {
            statusCode: 200,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(result.Attributes)
        };
    } catch (error) {
        console.error('Error:', error);

        if (error.code === 'ConditionalCheckFailedException') {
            return {
                statusCode: 404,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ error: 'Item not found' })
            };
        }

        return {
            statusCode: 500,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ error: 'Internal server error' })
        };
    }
};`
}

// generateDeleteHandler generates the DELETE operation handler
func generateDeleteHandler() string {
	return `const AWS = require('aws-sdk');
const dynamodb = new AWS.DynamoDB.DocumentClient();

exports.handler = async (event) => {
    console.log('DELETE Event:', JSON.stringify(event, null, 2));

    try {
        // Get ID from path parameters
        const id = event.pathParameters?.id;

        if (!id) {
            return {
                statusCode: 400,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ error: 'Missing required parameter: id' })
            };
        }

        // Build delete parameters
        const params = {
            TableName: process.env.TABLE_NAME,
            Key: { id },
            ConditionExpression: 'attribute_exists(id)'
        };

        // If multi-tenant, add tenant ID to key
        if (process.env.ENABLE_MULTI_TENANT === 'true' && event.requestContext?.authorizer?.tenantId) {
            params.Key.tenantId = event.requestContext.authorizer.tenantId;
        }

        // Delete item from DynamoDB
        await dynamodb.delete(params).promise();

        return {
            statusCode: 204,
            headers: { 'Content-Type': 'application/json' }
        };
    } catch (error) {
        console.error('Error:', error);

        if (error.code === 'ConditionalCheckFailedException') {
            return {
                statusCode: 404,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ error: 'Item not found' })
            };
        }

        return {
            statusCode: 500,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ error: 'Internal server error' })
        };
    }
};`
}

// generateListHandler generates the LIST operation handler
func generateListHandler() string {
	return `const AWS = require('aws-sdk');
const dynamodb = new AWS.DynamoDB.DocumentClient();

exports.handler = async (event) => {
    console.log('LIST Event:', JSON.stringify(event, null, 2));

    try {
        // Parse query parameters
        const limit = event.queryStringParameters?.limit ?
            parseInt(event.queryStringParameters.limit) : 50;
        const nextToken = event.queryStringParameters?.nextToken;

        // Build scan parameters
        const params = {
            TableName: process.env.TABLE_NAME,
            Limit: Math.min(limit, 100) // Max 100 items per request
        };

        // If multi-tenant, add filter for tenant ID
        if (process.env.ENABLE_MULTI_TENANT === 'true' && event.requestContext?.authorizer?.tenantId) {
            params.FilterExpression = 'tenantId = :tenantId';
            params.ExpressionAttributeValues = {
                ':tenantId': event.requestContext.authorizer.tenantId
            };
        }

        // Add pagination token if provided
        if (nextToken) {
            try {
                params.ExclusiveStartKey = JSON.parse(Buffer.from(nextToken, 'base64').toString());
            } catch (e) {
                return {
                    statusCode: 400,
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ error: 'Invalid nextToken' })
                };
            }
        }

        // Scan table
        const result = await dynamodb.scan(params).promise();

        // Build response
        const response = {
            items: result.Items || [],
            count: result.Count || 0
        };

        // Add next token if there are more results
        if (result.LastEvaluatedKey) {
            response.nextToken = Buffer.from(
                JSON.stringify(result.LastEvaluatedKey)
            ).toString('base64');
        }

        return {
            statusCode: 200,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(response)
        };
    } catch (error) {
        console.error('Error:', error);

        return {
            statusCode: 500,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ error: 'Internal server error' })
        };
    }
};`
}

// generateSearchHandler generates the SEARCH operation handler
func generateSearchHandler() string {
	return `const AWS = require('aws-sdk');
const dynamodb = new AWS.DynamoDB.DocumentClient();

exports.handler = async (event) => {
    console.log('SEARCH Event:', JSON.stringify(event, null, 2));

    try {
        // Parse request body
        const body = JSON.parse(event.body || '{}');
        const { query, fields, limit = 50, nextToken } = body;

        if (!query) {
            return {
                statusCode: 400,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ error: 'Missing required field: query' })
            };
        }

        // Build scan parameters with filter
        const params = {
            TableName: process.env.TABLE_NAME,
            Limit: Math.min(limit, 100)
        };

        // Build filter expression for search
        const filterExpressions = [];
        const expressionAttributeNames = {};
        const expressionAttributeValues = {
            ':query': query.toLowerCase()
        };

        // Search in specified fields or default searchable fields
        const searchFields = fields || ['name', 'description', 'title'];
        searchFields.forEach((field, index) => {
            const placeholder = '#field' + index;
            expressionAttributeNames[placeholder] = field;
            filterExpressions.push('contains(lower(' + placeholder + '), :query)');
        });

        // If multi-tenant, add tenant filter
        if (process.env.ENABLE_MULTI_TENANT === 'true' && event.requestContext?.authorizer?.tenantId) {
            filterExpressions.push('tenantId = :tenantId');
            expressionAttributeValues[':tenantId'] = event.requestContext.authorizer.tenantId;
        }

        params.FilterExpression = filterExpressions.join(' OR ');
        params.ExpressionAttributeNames = expressionAttributeNames;
        params.ExpressionAttributeValues = expressionAttributeValues;

        // Add pagination token if provided
        if (nextToken) {
            try {
                params.ExclusiveStartKey = JSON.parse(Buffer.from(nextToken, 'base64').toString());
            } catch (e) {
                return {
                    statusCode: 400,
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ error: 'Invalid nextToken' })
                };
            }
        }

        // Scan table with search filter
        const result = await dynamodb.scan(params).promise();

        // Build response
        const response = {
            items: result.Items || [],
            count: result.Count || 0,
            query: query
        };

        // Add next token if there are more results
        if (result.LastEvaluatedKey) {
            response.nextToken = Buffer.from(
                JSON.stringify(result.LastEvaluatedKey)
            ).toString('base64');
        }

        return {
            statusCode: 200,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(response)
        };
    } catch (error) {
        console.error('Error:', error);

        return {
            statusCode: 500,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ error: 'Internal server error' })
        };
    }
};`
}

// generateDefaultHandler generates a default handler for unknown operations
func generateDefaultHandler() string {
	return `exports.handler = async (event) => {
    console.log('Event:', JSON.stringify(event, null, 2));

    return {
        statusCode: 501,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            error: 'Not implemented',
            operation: process.env.CRUD_OPERATION || 'unknown'
        })
    };
};`
}

// GenerateCRUDHandlerCode generates the Lambda handler code for a CRUD operation
// This is exported for use in CDK constructs
func GenerateCRUDHandlerCode(operation string) string {
	return generateCRUDHandler(operation)
}
