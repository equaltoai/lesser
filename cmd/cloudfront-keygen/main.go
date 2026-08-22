// Package main implements a CloudFormation custom resource Lambda function
// that generates RSA-2048 key pairs for CloudFront signed URLs at deployment time.
// The private key is stored in AWS Secrets Manager and the public key is returned
// as a CloudFormation output for manual upload to CloudFront Key Management.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/cfn"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// KeyPairData represents the CloudFront key pair stored in Secrets Manager
type KeyPairData struct {
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
}

type secretsManagerAPI interface {
	CreateSecret(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	PutSecretValue(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
	DescribeSecret(ctx context.Context, params *secretsmanager.DescribeSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error)
}

type cloudFrontAPI interface {
	ListPublicKeys(ctx context.Context, params *cloudfront.ListPublicKeysInput, optFns ...func(*cloudfront.Options)) (*cloudfront.ListPublicKeysOutput, error)
	GetPublicKeyConfig(ctx context.Context, params *cloudfront.GetPublicKeyConfigInput, optFns ...func(*cloudfront.Options)) (*cloudfront.GetPublicKeyConfigOutput, error)
	UpdatePublicKey(ctx context.Context, params *cloudfront.UpdatePublicKeyInput, optFns ...func(*cloudfront.Options)) (*cloudfront.UpdatePublicKeyOutput, error)
	CreatePublicKey(ctx context.Context, params *cloudfront.CreatePublicKeyInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreatePublicKeyOutput, error)

	ListKeyGroups(ctx context.Context, params *cloudfront.ListKeyGroupsInput, optFns ...func(*cloudfront.Options)) (*cloudfront.ListKeyGroupsOutput, error)
	GetKeyGroupConfig(ctx context.Context, params *cloudfront.GetKeyGroupConfigInput, optFns ...func(*cloudfront.Options)) (*cloudfront.GetKeyGroupConfigOutput, error)
	UpdateKeyGroup(ctx context.Context, params *cloudfront.UpdateKeyGroupInput, optFns ...func(*cloudfront.Options)) (*cloudfront.UpdateKeyGroupOutput, error)
	CreateKeyGroup(ctx context.Context, params *cloudfront.CreateKeyGroupInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateKeyGroupOutput, error)
}

var (
	loadAWSConfigFn             = config.LoadDefaultConfig
	newSecretsManagerClientFn   = func(cfg aws.Config) secretsManagerAPI { return secretsmanager.NewFromConfig(cfg) }
	newCloudFrontClientFn       = func(cfg aws.Config) cloudFrontAPI { return cloudfront.NewFromConfig(cfg) }
	rsaGenerateKeyFn            = rsa.GenerateKey
	ensureCloudFrontResourcesFn = ensureCloudFrontResources
)

// handler processes CloudFormation custom resource events for CloudFront key pair generation
func handler(ctx context.Context, event cfn.Event) (physicalResourceID string, data map[string]interface{}, err error) {
	log.Printf("Processing CloudFormation event: %s for resource: %s", event.RequestType, event.LogicalResourceID)

	secretName, ok := event.ResourceProperties["SecretName"].(string)
	if !ok || secretName == "" {
		return "", nil, fmt.Errorf("SecretName property is required")
	}

	keyName, _ := event.ResourceProperties["KeyName"].(string)
	if keyName == "" {
		keyName = fmt.Sprintf("lesser-%s-key", event.LogicalResourceID)
	}

	keyGroupName, _ := event.ResourceProperties["KeyGroupName"].(string)
	if keyGroupName == "" {
		keyGroupName = fmt.Sprintf("lesser-%s-keygroup", event.LogicalResourceID)
	}

	// For Delete events, don't delete the secret as it may still be in use
	if event.RequestType == cfn.RequestDelete {
		log.Printf("DELETE event - leaving secret %s in place", secretName)
		return event.PhysicalResourceID, nil, nil
	}

	// For Create and Update, generate or regenerate the key pair
	log.Printf("Generating RSA-2048 key pair for secret: %s", secretName)

	// Generate RSA-2048 key pair
	privateKey, err := rsaGenerateKeyFn(rand.Reader, 2048)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Encode private key to PEM format (PKCS#1)
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// Encode public key to PEM format (PKIX)
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	// Prepare secret data as JSON
	keyData := KeyPairData{
		PrivateKey: string(privateKeyPEM),
		PublicKey:  string(publicKeyPEM),
	}
	secretValue, err := json.Marshal(keyData) //nolint:gosec // G117: intentional private-key serialization — this tool exists to generate and store a CloudFront key pair in Secrets Manager
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal key pair data: %w", err)
	}

	// Load AWS SDK config
	cfg, err := loadAWSConfigFn(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := newSecretsManagerClientFn(cfg)

	// Try to create the secret
	log.Printf("Storing key pair in Secrets Manager: %s", secretName)
	createOutput, err := client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         &secretName,
		Description:  stringPtr("CloudFront RSA-2048 private key for signed URL generation (auto-generated)"),
		SecretString: stringPtr(string(secretValue)),
	})

	var secretArn string
	if err != nil {
		// Check if secret already exists using errors.As for safe type checking
		var resourceExists *types.ResourceExistsException
		if errors.As(err, &resourceExists) {
			// Secret exists, update it instead
			log.Printf("Secret exists, updating: %s", secretName)
			_, err = client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
				SecretId:     &secretName,
				SecretString: stringPtr(string(secretValue)),
			})
			if err != nil {
				return "", nil, fmt.Errorf("failed to update secret: %w", err)
			}

			// Get the ARN for existing secret
			descOutput, err := client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
				SecretId: &secretName,
			})
			if err != nil {
				return "", nil, fmt.Errorf("failed to describe secret: %w", err)
			}
			secretArn = *descOutput.ARN
		} else {
			// Some other error (AccessDenied, Throttling, etc.)
			return "", nil, fmt.Errorf("failed to create secret: %w", err)
		}
	} else {
		secretArn = *createOutput.ARN
		log.Printf("Created secret with ARN: %s", secretArn)
	}

	// Ensure CloudFront resources exist and are updated
	cfClient := newCloudFrontClientFn(cfg)

	trimmedPublicKey := strings.TrimSpace(string(publicKeyPEM))

	publicKeyID, keyGroupID, err := ensureCloudFrontResourcesFn(ctx, cfClient, keyName, keyGroupName, trimmedPublicKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to ensure CloudFront resources: %w", err)
	}

	// Return outputs for CloudFormation
	return secretArn, map[string]interface{}{
		"SecretArn":   secretArn,
		"PublicKey":   trimmedPublicKey,
		"PublicKeyId": publicKeyID,
		"KeyGroupId":  keyGroupID,
	}, nil
}

func stringPtr(s string) *string {
	return &s
}

func ensureCloudFrontResources(ctx context.Context, client cloudFrontAPI, keyName, keyGroupName, encodedKey string) (string, string, error) {
	publicKeyID, err := upsertPublicKey(ctx, client, keyName, encodedKey)
	if err != nil {
		return "", "", err
	}

	keyGroupID, err := upsertKeyGroup(ctx, client, keyGroupName, publicKeyID)
	if err != nil {
		return "", "", err
	}

	return publicKeyID, keyGroupID, nil
}

func upsertPublicKey(ctx context.Context, client cloudFrontAPI, keyName, encodedKey string) (string, error) {
	existingID, existingConfig, etag, err := findPublicKeyByName(ctx, client, keyName)
	if err != nil {
		return "", err
	}

	comment := fmt.Sprintf("Managed by Lesser CDK (%s)", keyName)

	if existingConfig != nil && existingID != "" && etag != nil {
		cfg := existingConfig
		cfg.EncodedKey = aws.String(encodedKey)
		cfg.Comment = aws.String(comment)

		_, err = client.UpdatePublicKey(ctx, &cloudfront.UpdatePublicKeyInput{
			Id:              aws.String(existingID),
			IfMatch:         etag,
			PublicKeyConfig: cfg,
		})
		if err != nil {
			return "", fmt.Errorf("failed to update CloudFront public key %s: %w", keyName, err)
		}
		return existingID, nil
	}

	callerRef := fmt.Sprintf("%s-%d", keyName, time.Now().UnixNano())

	out, err := client.CreatePublicKey(ctx, &cloudfront.CreatePublicKeyInput{
		PublicKeyConfig: &cftypes.PublicKeyConfig{
			CallerReference: aws.String(callerRef),
			Name:            aws.String(keyName),
			EncodedKey:      aws.String(encodedKey),
			Comment:         aws.String(comment),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create CloudFront public key %s: %w", keyName, err)
	}

	return aws.ToString(out.PublicKey.Id), nil
}

func upsertKeyGroup(ctx context.Context, client cloudFrontAPI, keyGroupName, publicKeyID string) (string, error) {
	existingID, existingConfig, etag, err := findKeyGroupByName(ctx, client, keyGroupName)
	if err != nil {
		return "", err
	}

	items := []string{publicKeyID}
	comment := fmt.Sprintf("Managed by Lesser CDK (%s)", keyGroupName)

	if existingConfig != nil && existingID != "" && etag != nil {
		cfg := &cftypes.KeyGroupConfig{
			Name:    existingConfig.Name,
			Items:   items,
			Comment: aws.String(comment),
		}
		_, err = client.UpdateKeyGroup(ctx, &cloudfront.UpdateKeyGroupInput{
			Id:             aws.String(existingID),
			IfMatch:        etag,
			KeyGroupConfig: cfg,
		})
		if err != nil {
			return "", fmt.Errorf("failed to update CloudFront key group %s: %w", keyGroupName, err)
		}
		return existingID, nil
	}

	out, err := client.CreateKeyGroup(ctx, &cloudfront.CreateKeyGroupInput{
		KeyGroupConfig: &cftypes.KeyGroupConfig{
			Name:    aws.String(keyGroupName),
			Items:   items,
			Comment: aws.String(comment),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create CloudFront key group %s: %w", keyGroupName, err)
	}

	return aws.ToString(out.KeyGroup.Id), nil
}

func findPublicKeyByName(ctx context.Context, client cloudFrontAPI, keyName string) (string, *cftypes.PublicKeyConfig, *string, error) {
	params := &cloudfront.ListPublicKeysInput{}
	for {
		out, err := client.ListPublicKeys(ctx, params)
		if err != nil {
			return "", nil, nil, fmt.Errorf("failed to list CloudFront public keys: %w", err)
		}

		if out.PublicKeyList != nil {
			for _, summary := range out.PublicKeyList.Items {
				if aws.ToString(summary.Name) == keyName {
					cfgResp, err := client.GetPublicKeyConfig(ctx, &cloudfront.GetPublicKeyConfigInput{Id: summary.Id})
					if err != nil {
						return "", nil, nil, fmt.Errorf("failed to get CloudFront public key config: %w", err)
					}
					return aws.ToString(summary.Id), cfgResp.PublicKeyConfig, cfgResp.ETag, nil
				}
			}
		}

		if out.PublicKeyList == nil || out.PublicKeyList.NextMarker == nil || aws.ToString(out.PublicKeyList.NextMarker) == "" {
			break
		}
		params.Marker = out.PublicKeyList.NextMarker
	}

	return "", nil, nil, nil
}

func findKeyGroupByName(ctx context.Context, client cloudFrontAPI, keyGroupName string) (string, *cftypes.KeyGroupConfig, *string, error) {
	params := &cloudfront.ListKeyGroupsInput{}
	for {
		out, err := client.ListKeyGroups(ctx, params)
		if err != nil {
			return "", nil, nil, fmt.Errorf("failed to list CloudFront key groups: %w", err)
		}

		if out.KeyGroupList != nil {
			for _, summary := range out.KeyGroupList.Items {
				if summary.KeyGroup != nil && summary.KeyGroup.KeyGroupConfig != nil && aws.ToString(summary.KeyGroup.KeyGroupConfig.Name) == keyGroupName {
					cfgResp, err := client.GetKeyGroupConfig(ctx, &cloudfront.GetKeyGroupConfigInput{Id: summary.KeyGroup.Id})
					if err != nil {
						return "", nil, nil, fmt.Errorf("failed to get CloudFront key group config: %w", err)
					}
					return aws.ToString(summary.KeyGroup.Id), cfgResp.KeyGroupConfig, cfgResp.ETag, nil
				}
			}
		}

		if out.KeyGroupList == nil || out.KeyGroupList.NextMarker == nil || aws.ToString(out.KeyGroupList.NextMarker) == "" {
			break
		}
		params.Marker = out.KeyGroupList.NextMarker
	}

	return "", nil, nil, nil
}

func main() {
	lambda.Start(cfn.LambdaWrap(handler))
}
