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

	"github.com/aws/aws-lambda-go/cfn"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// KeyPairData represents the CloudFront key pair stored in Secrets Manager
type KeyPairData struct {
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
}

// handler processes CloudFormation custom resource events for CloudFront key pair generation
func handler(ctx context.Context, event cfn.Event) (physicalResourceID string, data map[string]interface{}, err error) {
	log.Printf("Processing CloudFormation event: %s for resource: %s", event.RequestType, event.LogicalResourceID)

	secretName, ok := event.ResourceProperties["SecretName"].(string)
	if !ok || secretName == "" {
		return "", nil, fmt.Errorf("SecretName property is required")
	}

	// For Delete events, don't delete the secret as it may still be in use
	if event.RequestType == cfn.RequestDelete {
		log.Printf("DELETE event - leaving secret %s in place", secretName)
		return event.PhysicalResourceID, nil, nil
	}

	// For Create and Update, generate or regenerate the key pair
	log.Printf("Generating RSA-2048 key pair for secret: %s", secretName)

	// Generate RSA-2048 key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
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
	secretValue, err := json.Marshal(keyData)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal key pair data: %w", err)
	}

	// Load AWS SDK config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)

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

	// Return outputs for CloudFormation
	return secretArn, map[string]interface{}{
		"SecretArn": secretArn,
		"PublicKey": string(publicKeyPEM),
	}, nil
}

func stringPtr(s string) *string {
	return &s
}

func main() {
	lambda.Start(cfn.LambdaWrap(handler))
}
