//go:build vault_aws

// Untested

package vault

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

func init() {
	registerProvider(ProviderAWS, func(cfg Config) (Vault, error) {
		return NewAWSVault(cfg)
	})
}

// AWSVault implements Vault interface for AWS Secrets Manager
type AWSVault struct {
	client *secretsmanager.Client
	region string
}

// NewAWSVault creates a new AWS Secrets Manager vault
func NewAWSVault(cfg Config) (*AWSVault, error) {
	if cfg.AWSRegion == "" {
		return nil, fmt.Errorf("%w: aws_region is required", ErrInvalidConfiguration)
	}

	ctx := context.Background()

	// Load AWS config
	var awsCfg aws.Config
	var err error

	if cfg.AWSAccessKey != "" && cfg.AWSSecretKey != "" {
		// Use provided credentials
		awsCfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(cfg.AWSRegion),
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(
					cfg.AWSAccessKey,
					cfg.AWSSecretKey,
					"",
				),
			),
		)
	} else {
		// Use default credential chain (env vars, IAM role, etc.)
		awsCfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(cfg.AWSRegion),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(awsCfg)

	return &AWSVault{
		client: client,
		region: cfg.AWSRegion,
	}, nil
}

// GetSecret retrieves a secret from AWS Secrets Manager
// Key format: "llm/anthropic/api_key" -> Secret name: "llm/anthropic/api_key"
// Or structured: "llm/anthropic" with field "api_key" in JSON
func (a *AWSVault) GetSecret(ctx context.Context, key string) (string, error) {
	// Try to get the secret directly
	value, err := a.getSecretValue(ctx, key)
	if err == nil {
		return value, nil
	}

	// If not found, try to parse as path/field
	path, field := a.parseKey(key)
	if field != "" {
		return a.getSecretField(ctx, path, field)
	}

	return "", fmt.Errorf("%w: %s", ErrSecretNotFound, key)
}

// GetSecretWithVersion retrieves a specific version of a secret
func (a *AWSVault) GetSecretWithVersion(ctx context.Context, key, version string) (string, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId:  aws.String(key),
		VersionId: aws.String(version),
	}

	result, err := a.client.GetSecretValue(ctx, input)
	if err != nil {
		return "", fmt.Errorf("%w: %s version %s", ErrSecretVersionNotFound, key, version)
	}

	if result.SecretString == nil {
		return "", fmt.Errorf("secret is not a string: %s", key)
	}

	return *result.SecretString, nil
}

// SetSecret stores a secret in AWS Secrets Manager
func (a *AWSVault) SetSecret(ctx context.Context, key, value string) error {
	// Check if secret exists
	_, err := a.client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(key),
	})

	if err != nil {
		// Secret doesn't exist, create it
		_, err = a.client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
			Name:         aws.String(key),
			SecretString: aws.String(value),
		})
		if err != nil {
			return fmt.Errorf("failed to create secret: %w", err)
		}
	} else {
		// Secret exists, update it
		_, err = a.client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
			SecretId:     aws.String(key),
			SecretString: aws.String(value),
		})
		if err != nil {
			return fmt.Errorf("failed to update secret: %w", err)
		}
	}

	return nil
}

// DeleteSecret removes a secret from AWS Secrets Manager
func (a *AWSVault) DeleteSecret(ctx context.Context, key string) error {
	_, err := a.client.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(key),
		ForceDeleteWithoutRecovery: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// ListSecrets lists secrets in AWS Secrets Manager
func (a *AWSVault) ListSecrets(ctx context.Context, prefix string) ([]string, error) {
	var secrets []string
	var nextToken *string

	for {
		input := &secretsmanager.ListSecretsInput{
			MaxResults: aws.Int32(100),
			NextToken:  nextToken,
		}

		result, err := a.client.ListSecrets(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}

		for _, secret := range result.SecretList {
			if secret.Name != nil {
				name := *secret.Name
				if prefix == "" || matchesPrefix(name, prefix) {
					secrets = append(secrets, name)
				}
			}
		}

		if result.NextToken == nil {
			break
		}
		nextToken = result.NextToken
	}

	return secrets, nil
}

// Close cleans up resources
func (a *AWSVault) Close() error {
	// AWS SDK doesn't require explicit cleanup
	return nil
}

// Name returns the vault type
func (a *AWSVault) Name() string {
	return "aws"
}

// getSecretValue retrieves a secret value directly
func (a *AWSVault) getSecretValue(ctx context.Context, key string) (string, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(key),
	}

	result, err := a.client.GetSecretValue(ctx, input)
	if err != nil {
		return "", err
	}

	if result.SecretString == nil {
		return "", fmt.Errorf("secret is not a string")
	}

	return *result.SecretString, nil
}

// getSecretField retrieves a field from a JSON secret
func (a *AWSVault) getSecretField(ctx context.Context, path, field string) (string, error) {
	secretString, err := a.getSecretValue(ctx, path)
	if err != nil {
		return "", err
	}

	// Try to parse as JSON
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(secretString), &data); err != nil {
		return "", fmt.Errorf("secret is not JSON: %s", path)
	}

	value, ok := data[field]
	if !ok {
		return "", fmt.Errorf("%w: field %s not found in %s", ErrSecretNotFound, field, path)
	}

	valueStr, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("field value is not a string: %s/%s", path, field)
	}

	return valueStr, nil
}

// parseKey splits a key into path and field
func (a *AWSVault) parseKey(key string) (path string, field string) {
	lastSlash := -1
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '/' {
			lastSlash = i
			break
		}
	}

	if lastSlash == -1 {
		return key, ""
	}

	return key[:lastSlash], key[lastSlash+1:]
}

// matchesPrefix checks if a name matches a prefix
func matchesPrefix(name, prefix string) bool {
	if len(prefix) == 0 {
		return true
	}
	return len(name) >= len(prefix) && name[:len(prefix)] == prefix
}

// Example usage:
//
// vault, _ := NewAWSVault(Config{
//     AWSRegion:    "us-east-1",
//     AWSAccessKey: "AKIA...",
//     AWSSecretKey: "...",
// })
//
// // Or use default credentials (IAM role, env vars)
// vault, _ := NewAWSVault(Config{
//     AWSRegion: "us-east-1",
// })
//
// // Store secret
// vault.SetSecret(ctx, "llm/anthropic/api_key", "sk-ant-...")
//
// // Get secret
// key, _ := vault.GetSecret(ctx, "llm/anthropic/api_key")
//
// // Store JSON secret with multiple fields
// jsonSecret := `{"api_key": "sk-ant-...", "model": "claude-3"}`
// vault.SetSecret(ctx, "llm/anthropic", jsonSecret)
// apiKey, _ := vault.GetSecret(ctx, "llm/anthropic/api_key") // Extracts from JSON
//
// // List secrets
// secrets, _ := vault.ListSecrets(ctx, "llm/")
