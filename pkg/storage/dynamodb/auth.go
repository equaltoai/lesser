package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// Session management

// CreateSession creates a new session in DynamoDB
func (s *dynamoDBStorage) CreateSession(ctx context.Context, session *storage.Session) error {
	item := map[string]types.AttributeValue{
		"PK":           &types.AttributeValueMemberS{Value: s.userPK(session.Username)},
		"SK":           &types.AttributeValueMemberS{Value: "SESSION#" + session.SessionID},
		"GSI1PK":       &types.AttributeValueMemberS{Value: "REFRESHTOKEN#" + session.RefreshToken},
		"GSI1SK":       &types.AttributeValueMemberS{Value: session.SessionID},
		"Type":         &types.AttributeValueMemberS{Value: "Session"},
		"SessionID":    &types.AttributeValueMemberS{Value: session.SessionID},
		"RefreshToken": &types.AttributeValueMemberS{Value: session.RefreshToken},
		"DeviceID":     &types.AttributeValueMemberS{Value: session.DeviceID},
		"DeviceName":   &types.AttributeValueMemberS{Value: session.DeviceName},
		"UserAgent":    &types.AttributeValueMemberS{Value: session.UserAgent},
		"IPAddress":    &types.AttributeValueMemberS{Value: session.IPAddress},
		"AuthMethod":   &types.AttributeValueMemberS{Value: session.AuthMethod},
		"CreatedAt":    &types.AttributeValueMemberS{Value: session.CreatedAt.Format(time.RFC3339)},
		"LastActivity": &types.AttributeValueMemberS{Value: session.LastActivity.Format(time.RFC3339)},
		"ExpiresAt":    &types.AttributeValueMemberS{Value: session.ExpiresAt.Format(time.RFC3339)},
		// TTL for automatic deletion
		"TTL": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", session.ExpiresAt.Unix())},
	}

	if session.PreviousRefreshToken != "" {
		item["PreviousRefreshToken"] = &types.AttributeValueMemberS{Value: session.PreviousRefreshToken}
		item["TokenRotatedAt"] = &types.AttributeValueMemberS{Value: session.TokenRotatedAt.Format(time.RFC3339)}
	}

	input := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	}

	_, err := s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// GetSession retrieves a session by ID
func (s *dynamoDBStorage) GetSession(ctx context.Context, sessionID string) (*storage.Session, error) {
	// We need to find the username first - sessions are stored under USER#username
	// For now, we'll use a GSI query on the session ID
	// In production, you might want to store sessions differently or maintain a session->user mapping

	input := &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("SessionID = :sid AND #type = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sid":  &types.AttributeValueMemberS{Value: sessionID},
			":type": &types.AttributeValueMemberS{Value: "Session"},
		},
		ExpressionAttributeNames: map[string]string{
			"#type": "Type",
		},
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("session not found")
	}

	var session storage.Session
	if err := s.UnmarshalItem(result.Items[0], &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// GetSessionByRefreshToken retrieves a session by refresh token
func (s *dynamoDBStorage) GetSessionByRefreshToken(ctx context.Context, refreshToken string) (*storage.Session, error) {
	// Query GSI1 where REFRESHTOKEN#token is the partition key
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "REFRESHTOKEN#" + refreshToken},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query session by refresh token: %w", err)
	}

	if len(result.Items) == 0 {
		// Check if this is a previous refresh token
		return s.getSessionByPreviousRefreshToken(ctx, refreshToken)
	}

	// Get the session ID from the result
	sessionID := ""
	if v, ok := result.Items[0]["SessionID"]; ok {
		if s, ok := v.(*types.AttributeValueMemberS); ok {
			sessionID = s.Value
		}
	}

	if sessionID == "" {
		return nil, fmt.Errorf("session ID not found in result")
	}

	// Now get the full session
	return s.GetSession(ctx, sessionID)
}

// getSessionByPreviousRefreshToken checks if the token is a rotated token still in grace period
func (s *dynamoDBStorage) getSessionByPreviousRefreshToken(ctx context.Context, refreshToken string) (*storage.Session, error) {
	// Scan for sessions where PreviousRefreshToken matches
	input := &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("PreviousRefreshToken = :token AND #type = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":token": &types.AttributeValueMemberS{Value: refreshToken},
			":type":  &types.AttributeValueMemberS{Value: "Session"},
		},
		ExpressionAttributeNames: map[string]string{
			"#type": "Type",
		},
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to scan for previous refresh token: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("session not found")
	}

	var session storage.Session
	if err := s.UnmarshalItem(result.Items[0], &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// UpdateSession updates an existing session
func (s *dynamoDBStorage) UpdateSession(ctx context.Context, session *storage.Session) error {
	// Build update expression
	update := expression.Set(expression.Name("RefreshToken"), expression.Value(session.RefreshToken)).
		Set(expression.Name("LastActivity"), expression.Value(session.LastActivity.Format(time.RFC3339))).
		Set(expression.Name("ExpiresAt"), expression.Value(session.ExpiresAt.Format(time.RFC3339))).
		Set(expression.Name("TTL"), expression.Value(session.ExpiresAt.Unix())).
		Set(expression.Name("IPAddress"), expression.Value(session.IPAddress))

	// Update GSI1PK if refresh token changed
	update = update.Set(expression.Name("GSI1PK"), expression.Value("REFRESHTOKEN#"+session.RefreshToken))

	if session.PreviousRefreshToken != "" {
		update = update.Set(expression.Name("PreviousRefreshToken"), expression.Value(session.PreviousRefreshToken)).
			Set(expression.Name("TokenRotatedAt"), expression.Value(session.TokenRotatedAt.Format(time.RFC3339)))
	}

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return fmt.Errorf("failed to build update expression: %w", err)
	}

	input := &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(session.Username)},
			"SK": &types.AttributeValueMemberS{Value: "SESSION#" + session.SessionID},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	_, err = s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// DeleteSession deletes a session
func (s *dynamoDBStorage) DeleteSession(ctx context.Context, sessionID string) error {
	// First get the session to find the username
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	input := &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(session.Username)},
			"SK": &types.AttributeValueMemberS{Value: "SESSION#" + sessionID},
		},
	}

	_, err = s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// GetUserSessions returns all sessions for a user
func (s *dynamoDBStorage) GetUserSessions(ctx context.Context, username string) ([]*storage.Session, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: s.userPK(username)},
			":sk": &types.AttributeValueMemberS{Value: "SESSION#"},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query user sessions: %w", err)
	}

	var sessions []*storage.Session
	for _, item := range result.Items {
		var session storage.Session
		if err := s.UnmarshalItem(item, &session); err != nil {
			common.Logger().Error("failed to unmarshal session", zap.Error(err))
			continue
		}
		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// Device management

// CreateDevice creates a new device record
func (s *dynamoDBStorage) CreateDevice(ctx context.Context, device *storage.Device) error {
	item, err := attributevalue.MarshalMap(device)
	if err != nil {
		return fmt.Errorf("failed to marshal device: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: s.userPK(device.Username)}
	item["SK"] = &types.AttributeValueMemberS{Value: "DEVICE#" + device.DeviceID}
	item["Type"] = &types.AttributeValueMemberS{Value: "Device"}

	input := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create device: %w", err)
	}

	return nil
}

// GetDevice retrieves a device by ID
func (s *dynamoDBStorage) GetDevice(ctx context.Context, deviceID string) (*storage.Device, error) {
	// We need to find the device - scan for it
	input := &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("DeviceID = :did AND #type = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":did":  &types.AttributeValueMemberS{Value: deviceID},
			":type": &types.AttributeValueMemberS{Value: "Device"},
		},
		ExpressionAttributeNames: map[string]string{
			"#type": "Type",
		},
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to scan for device: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("device not found")
	}

	var device storage.Device
	if err := s.UnmarshalItem(result.Items[0], &device); err != nil {
		return nil, fmt.Errorf("failed to unmarshal device: %w", err)
	}

	return &device, nil
}

// UpdateDevice updates a device record
func (s *dynamoDBStorage) UpdateDevice(ctx context.Context, device *storage.Device) error {
	update := expression.Set(expression.Name("TrustLevel"), expression.Value(device.TrustLevel)).
		Set(expression.Name("LastSeenAt"), expression.Value(device.LastSeenAt.Format(time.RFC3339))).
		Set(expression.Name("LastIPAddress"), expression.Value(device.LastIPAddress)).
		Set(expression.Name("LastUserAgent"), expression.Value(device.LastUserAgent))

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return fmt.Errorf("failed to build update expression: %w", err)
	}

	input := &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(device.Username)},
			"SK": &types.AttributeValueMemberS{Value: "DEVICE#" + device.DeviceID},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	_, err = s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update device: %w", err)
	}

	return nil
}

// GetUserDevices returns all devices for a user
func (s *dynamoDBStorage) GetUserDevices(ctx context.Context, username string) ([]*storage.Device, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: s.userPK(username)},
			":sk": &types.AttributeValueMemberS{Value: "DEVICE#"},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query user devices: %w", err)
	}

	var devices []*storage.Device
	for _, item := range result.Items {
		var device storage.Device
		if err := s.UnmarshalItem(item, &device); err != nil {
			common.Logger().Error("failed to unmarshal device", zap.Error(err))
			continue
		}
		devices = append(devices, &device)
	}

	return devices, nil
}

// Rate limiting

// RecordLoginAttempt records a login attempt
func (s *dynamoDBStorage) RecordLoginAttempt(ctx context.Context, identifier string, success bool) error {
	now := time.Now()
	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: "RATELIMIT#" + identifier},
		"SK":        &types.AttributeValueMemberS{Value: now.Format(time.RFC3339Nano)},
		"Type":      &types.AttributeValueMemberS{Value: "LoginAttempt"},
		"Success":   &types.AttributeValueMemberBOOL{Value: success},
		"Timestamp": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
		// TTL for automatic cleanup after 24 hours
		"TTL": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", now.Add(24*time.Hour).Unix())},
	}

	input := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	}

	_, err := s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to record login attempt: %w", err)
	}

	return nil
}

// GetLoginAttemptCount returns the number of login attempts since the given time
func (s *dynamoDBStorage) GetLoginAttemptCount(ctx context.Context, identifier string, since time.Time) (int, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND SK > :since"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: "RATELIMIT#" + identifier},
			":since": &types.AttributeValueMemberS{Value: since.Format(time.RFC3339Nano)},
		},
		Select: types.SelectCount,
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to count login attempts: %w", err)
	}

	return int(result.Count), nil
}

// IsRateLimited checks if an identifier is currently rate limited
func (s *dynamoDBStorage) IsRateLimited(ctx context.Context, identifier string) (bool, time.Time, error) {
	// Check if there's an active lockout
	input := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "RATELIMIT#" + identifier},
			"SK": &types.AttributeValueMemberS{Value: "LOCKOUT"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("failed to check rate limit: %w", err)
	}

	if len(result.Item) == 0 {
		return false, time.Time{}, nil
	}

	// Check if lockout is still active
	if v, ok := result.Item["UnlockTime"]; ok {
		if s, ok := v.(*types.AttributeValueMemberS); ok {
			unlockTime, err := time.Parse(time.RFC3339, s.Value)
			if err == nil && time.Now().Before(unlockTime) {
				return true, unlockTime, nil
			}
		}
	}

	return false, time.Time{}, nil
}

// ClearLoginAttempts clears all login attempts for an identifier
func (s *dynamoDBStorage) ClearLoginAttempts(ctx context.Context, identifier string) error {
	// Query all attempts
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "RATELIMIT#" + identifier},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to query login attempts: %w", err)
	}

	// Delete all items
	for _, item := range result.Items {
		deleteInput := &dynamodb.DeleteItemInput{
			TableName: s.getTableName(),
			Key: map[string]types.AttributeValue{
				"PK": item["PK"],
				"SK": item["SK"],
			},
		}

		if _, err := s.client.DeleteItem(ctx, deleteInput); err != nil {
			common.Logger().Error("failed to delete login attempt", zap.Error(err))
		}
	}

	return nil
}

// WebAuthn operations

// StoreWebAuthnCredential stores a WebAuthn credential
func (s *dynamoDBStorage) StoreWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	item, err := attributevalue.MarshalMap(credential)
	if err != nil {
		return fmt.Errorf("failed to marshal credential: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: s.userPK(credential.UserID)}
	item["SK"] = &types.AttributeValueMemberS{Value: "CREDENTIAL#" + credential.ID}
	item["Type"] = &types.AttributeValueMemberS{Value: "WebAuthnCredential"}

	input := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to store credential: %w", err)
	}

	return nil
}

// GetWebAuthnCredential retrieves a WebAuthn credential by ID
func (s *dynamoDBStorage) GetWebAuthnCredential(ctx context.Context, credentialID string) (*storage.WebAuthnCredential, error) {
	// Scan for the credential
	input := &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("ID = :id AND #type = :type"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id":   &types.AttributeValueMemberS{Value: credentialID},
			":type": &types.AttributeValueMemberS{Value: "WebAuthnCredential"},
		},
		ExpressionAttributeNames: map[string]string{
			"#type": "Type",
		},
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to scan for credential: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("credential not found")
	}

	var credential storage.WebAuthnCredential
	if err := s.UnmarshalItem(result.Items[0], &credential); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credential: %w", err)
	}

	return &credential, nil
}

// GetUserWebAuthnCredentials retrieves all WebAuthn credentials for a user
func (s *dynamoDBStorage) GetUserWebAuthnCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: s.userPK(username)},
			":sk": &types.AttributeValueMemberS{Value: "CREDENTIAL#"},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query user credentials: %w", err)
	}

	var credentials []*storage.WebAuthnCredential
	for _, item := range result.Items {
		var credential storage.WebAuthnCredential
		if err := s.UnmarshalItem(item, &credential); err != nil {
			common.Logger().Error("failed to unmarshal credential", zap.Error(err))
			continue
		}
		credentials = append(credentials, &credential)
	}

	return credentials, nil
}

// UpdateWebAuthnCredential updates a WebAuthn credential
func (s *dynamoDBStorage) UpdateWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	update := expression.Set(expression.Name("SignCount"), expression.Value(credential.SignCount)).
		Set(expression.Name("CloneWarning"), expression.Value(credential.CloneWarning)).
		Set(expression.Name("LastUsedAt"), expression.Value(credential.LastUsedAt.Format(time.RFC3339))).
		Set(expression.Name("Name"), expression.Value(credential.Name))

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return fmt.Errorf("failed to build update expression: %w", err)
	}

	input := &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(credential.UserID)},
			"SK": &types.AttributeValueMemberS{Value: "CREDENTIAL#" + credential.ID},
		},
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	_, err = s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update credential: %w", err)
	}

	return nil
}

// DeleteWebAuthnCredential deletes a WebAuthn credential
func (s *dynamoDBStorage) DeleteWebAuthnCredential(ctx context.Context, credentialID string) error {
	// First get the credential to find the user
	credential, err := s.GetWebAuthnCredential(ctx, credentialID)
	if err != nil {
		return err
	}

	input := &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(credential.UserID)},
			"SK": &types.AttributeValueMemberS{Value: "CREDENTIAL#" + credentialID},
		},
	}

	_, err = s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	return nil
}

// WebAuthn challenge operations

// StoreWebAuthnChallenge stores a temporary WebAuthn challenge
func (s *dynamoDBStorage) StoreWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error {
	item, err := attributevalue.MarshalMap(challenge)
	if err != nil {
		return fmt.Errorf("failed to marshal challenge: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: "CHALLENGE#" + challenge.Challenge}
	item["SK"] = &types.AttributeValueMemberS{Value: "WEBAUTHN"}
	item["Type"] = &types.AttributeValueMemberS{Value: "WebAuthnChallenge"}
	// TTL for automatic cleanup
	item["TTL"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", challenge.ExpiresAt.Unix())}

	input := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to store challenge: %w", err)
	}

	return nil
}

// GetWebAuthnChallenge retrieves a WebAuthn challenge
func (s *dynamoDBStorage) GetWebAuthnChallenge(ctx context.Context, challengeID string) (*storage.WebAuthnChallenge, error) {
	input := &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "CHALLENGE#" + challengeID},
			"SK": &types.AttributeValueMemberS{Value: "WEBAUTHN"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get challenge: %w", err)
	}

	if len(result.Item) == 0 {
		return nil, fmt.Errorf("challenge not found")
	}

	var challenge storage.WebAuthnChallenge
	if err := s.UnmarshalItem(result.Item, &challenge); err != nil {
		return nil, fmt.Errorf("failed to unmarshal challenge: %w", err)
	}

	return &challenge, nil
}

// DeleteWebAuthnChallenge deletes a WebAuthn challenge
func (s *dynamoDBStorage) DeleteWebAuthnChallenge(ctx context.Context, challengeID string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "CHALLENGE#" + challengeID},
			"SK": &types.AttributeValueMemberS{Value: "WEBAUTHN"},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete challenge: %w", err)
	}

	return nil
}
