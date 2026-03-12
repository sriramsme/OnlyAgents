//go:build vault_gcp

// Untested

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func init() {
	registerProvider(ProviderGCP, func(cfg Config) (Vault, error) {
		return NewGCPVault(cfg)
	})
}

// GCPVault implements Vault interface for GCP Secret Manager
type GCPVault struct {
	client    *secretmanager.Client
	projectID string
}

// NewGCPVault creates a new GCP Secret Manager vault
func NewGCPVault(cfg Config) (*GCPVault, error) {
	if cfg.GCPProjectID == "" {
		return nil, fmt.Errorf("%w: gcp_project_id is required", ErrInvalidConfiguration)
	}

	ctx := context.Background()

	var opts []option.ClientOption
	if cfg.GCPCredentials != "" {
		opts = append(opts, option.WithCredentialsFile(cfg.GCPCredentials))
	}
	// If no credentials file, uses Application Default Credentials

	client, err := secretmanager.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP client: %w", err)
	}

	return &GCPVault{
		client:    client,
		projectID: cfg.GCPProjectID,
	}, nil
}

// GetSecret retrieves a secret from GCP Secret Manager
// Key format: "llm/anthropic/api_key" -> Secret name: "llm-anthropic-api-key"
// GCP doesn't allow slashes in names, so we convert them to hyphens
func (g *GCPVault) GetSecret(ctx context.Context, key string) (string, error) {
	secretName := g.toSecretName(key)

	// Try to get the secret directly
	value, err := g.getLatestSecretVersion(ctx, secretName)
	if err == nil {
		return value, nil
	}

	// If not found, try to parse as structured secret
	path, field := g.parseKey(key)
	if field != "" {
		secretName = g.toSecretName(path)
		jsonValue, err := g.getLatestSecretVersion(ctx, secretName)
		if err != nil {
			return "", fmt.Errorf("%w: %s", ErrSecretNotFound, key)
		}

		return g.extractField(jsonValue, field)
	}

	return "", fmt.Errorf("%w: %s", ErrSecretNotFound, key)
}

// GetSecretWithVersion retrieves a specific version of a secret
func (g *GCPVault) GetSecretWithVersion(ctx context.Context, key, version string) (string, error) {
	secretName := g.toSecretName(key)
	versionPath := fmt.Sprintf("projects/%s/secrets/%s/versions/%s",
		g.projectID, secretName, version)

	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: versionPath,
	}

	result, err := g.client.AccessSecretVersion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("%w: %s version %s", ErrSecretVersionNotFound, key, version)
	}

	return string(result.Payload.Data), nil
}

// SetSecret stores a secret in GCP Secret Manager
func (g *GCPVault) SetSecret(ctx context.Context, key, value string) error {
	secretName := g.toSecretName(key)
	secretPath := fmt.Sprintf("projects/%s/secrets/%s", g.projectID, secretName)

	// Check if secret exists
	_, err := g.client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{
		Name: secretPath,
	})
	if err != nil {
		// Secret doesn't exist, create it
		createReq := &secretmanagerpb.CreateSecretRequest{
			Parent:   fmt.Sprintf("projects/%s", g.projectID),
			SecretId: secretName,
			Secret: &secretmanagerpb.Secret{
				Replication: &secretmanagerpb.Replication{
					Replication: &secretmanagerpb.Replication_Automatic_{
						Automatic: &secretmanagerpb.Replication_Automatic{},
					},
				},
			},
		}

		_, err = g.client.CreateSecret(ctx, createReq)
		if err != nil {
			return fmt.Errorf("failed to create secret: %w", err)
		}
	}

	// Add new version
	addReq := &secretmanagerpb.AddSecretVersionRequest{
		Parent: secretPath,
		Payload: &secretmanagerpb.SecretPayload{
			Data: []byte(value),
		},
	}

	_, err = g.client.AddSecretVersion(ctx, addReq)
	if err != nil {
		return fmt.Errorf("failed to add secret version: %w", err)
	}

	return nil
}

// DeleteSecret removes a secret from GCP Secret Manager
func (g *GCPVault) DeleteSecret(ctx context.Context, key string) error {
	secretName := g.toSecretName(key)
	secretPath := fmt.Sprintf("projects/%s/secrets/%s", g.projectID, secretName)

	req := &secretmanagerpb.DeleteSecretRequest{
		Name: secretPath,
	}

	err := g.client.DeleteSecret(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// ListSecrets lists secrets in GCP Secret Manager
func (g *GCPVault) ListSecrets(ctx context.Context, prefix string) ([]string, error) {
	var secrets []string

	req := &secretmanagerpb.ListSecretsRequest{
		Parent: fmt.Sprintf("projects/%s", g.projectID),
	}

	it := g.client.ListSecrets(ctx, req)
	for {
		secret, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}

		// Extract secret name from full path
		// "projects/PROJECT/secrets/NAME" -> "NAME"
		parts := strings.Split(secret.Name, "/")
		if len(parts) > 0 {
			name := parts[len(parts)-1]
			vaultKey := g.fromSecretName(name)

			if prefix == "" || strings.HasPrefix(vaultKey, prefix) {
				secrets = append(secrets, vaultKey)
			}
		}
	}

	return secrets, nil
}

// Close cleans up resources
func (g *GCPVault) Close() error {
	return g.client.Close()
}

// Name returns the vault type
func (g *GCPVault) Name() string {
	return "gcp"
}

// getLatestSecretVersion retrieves the latest version of a secret
func (g *GCPVault) getLatestSecretVersion(ctx context.Context, secretName string) (string, error) {
	versionPath := fmt.Sprintf("projects/%s/secrets/%s/versions/latest",
		g.projectID, secretName)

	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: versionPath,
	}

	result, err := g.client.AccessSecretVersion(ctx, req)
	if err != nil {
		return "", err
	}

	return string(result.Payload.Data), nil
}

// toSecretName converts vault key to GCP secret name
// "llm/anthropic/api_key" -> "llm-anthropic-api-key"
// GCP secret names must be: [a-zA-Z0-9-_]
func (g *GCPVault) toSecretName(key string) string {
	// Replace slashes with hyphens
	name := strings.ReplaceAll(key, "/", "-")
	// Replace underscores with hyphens
	name = strings.ReplaceAll(name, "_", "-")
	// Convert to lowercase (GCP requirement)
	name = strings.ToLower(name)
	return name
}

// fromSecretName converts GCP secret name back to vault key
// "llm-anthropic-api-key" -> "llm/anthropic/api_key"
func (g *GCPVault) fromSecretName(secretName string) string {
	// Convert hyphens back to slashes
	// Note: This is lossy since we can't distinguish between original hyphens and slashes
	return strings.ReplaceAll(secretName, "-", "/")
}

// parseKey splits a key into path and field
func (g *GCPVault) parseKey(key string) (path string, field string) {
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

// extractField extracts a field from a JSON secret value
func (g *GCPVault) extractField(jsonValue, field string) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonValue), &data); err != nil {
		return "", fmt.Errorf("secret is not JSON")
	}

	value, ok := data[field]
	if !ok {
		return "", fmt.Errorf("%w: field %s not found", ErrSecretNotFound, field)
	}

	valueStr, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("field value is not a string")
	}

	return valueStr, nil
}

// Example usage:
//
// vault, _ := NewGCPVault(Config{
//     GCPProjectID:   "my-project",
//     GCPCredentials: "/path/to/credentials.json",
// })
//
// // Or use Application Default Credentials
// vault, _ := NewGCPVault(Config{
//     GCPProjectID: "my-project",
// })
//
// // Store secret
// vault.SetSecret(ctx, "llm/anthropic/api_key", "sk-ant-...")
//
// // Get secret
// key, _ := vault.GetSecret(ctx, "llm/anthropic/api_key")
// // Secret name in GCP: "llm-anthropic-api-key"
//
// // Store JSON secret with multiple fields
// jsonSecret := `{"api_key": "sk-ant-...", "model": "claude-3"}`
// vault.SetSecret(ctx, "llm/anthropic", jsonSecret)
// apiKey, _ := vault.GetSecret(ctx, "llm/anthropic/api_key") // Extracts from JSON
//
// // List secrets
// secrets, _ := vault.ListSecrets(ctx, "llm/")
