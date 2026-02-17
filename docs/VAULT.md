# Secrets Management Guide

OnlyAgents uses a unified vault system for all secrets. This guide explains how to set up secrets for different vault types.

## Vault Path Format

Vault paths use a hierarchical format: `category/provider/key`

Examples:
- `llm/openai/api_key`
- `llm/anthropic/api_key`
- `database/postgres/password`
- `slack/bot/token`

## Environment Variables (Default)

The EnvVault translates vault paths to environment variables.

### Translation Rules

**Format:** `PREFIX_CATEGORY_PROVIDER_KEY` (all uppercase, slashes → underscores)

**Examples:**

| Vault Path                | Environment Variable                    | Fallback                |
|---------------------------|-----------------------------------------|-------------------------|
| `llm/openai/api_key`      | `ONLYAGENTS_LLM_OPENAI_API_KEY`         | `OPENAI_API_KEY`        |
| `llm/anthropic/api_key`   | `ONLYAGENTS_LLM_ANTHROPIC_API_KEY`      | `ANTHROPIC_API_KEY`     |
| `database/postgres/password` | `ONLYAGENTS_DATABASE_POSTGRES_PASSWORD` | `POSTGRES_PASSWORD`   |
| `slack/bot/token`         | `ONLYAGENTS_SLACK_BOT_TOKEN`            | `SLACK_BOT_TOKEN`       |

### Setup: .env File

Create a `.env` file in your project root:
```bash
# .env - DO NOT COMMIT THIS FILE!

# OpenAI (either format works)
ONLYAGENTS_LLM_OPENAI_API_KEY=sk-proj-...
# OR
OPENAI_API_KEY=sk-proj-...

# Anthropic
ONLYAGENTS_LLM_ANTHROPIC_API_KEY=sk-ant-...
# OR
ANTHROPIC_API_KEY=sk-ant-...

# Google AI
ONLYAGENTS_LLM_GOOGLE_API_KEY=...
GOOGLE_API_KEY=...

# Database
ONLYAGENTS_DATABASE_POSTGRES_PASSWORD=mysecretpassword

# Slack
ONLYAGENTS_SLACK_BOT_TOKEN=xoxb-...
```

**Add to `.gitignore`:**
```
.env
.env.local
.env.*.local
```

### Setup: System Environment

For production, set environment variables directly:
```bash
export ONLYAGENTS_LLM_OPENAI_API_KEY="sk-proj-..."
export ONLYAGENTS_DATABASE_POSTGRES_PASSWORD="..."
```

## HashiCorp Vault

HashiCorp Vault stores secrets at specific paths in a Key-Value store.

### Configuration
```yaml
vault:
  type: "hashicorp"
  address: "https://vault.example.com:8200"
  token: "${VAULT_TOKEN}"  # Or use AppRole, Kubernetes auth, etc.
  namespace: "my-namespace"  # Optional
  mount_path: "secret"       # KV mount path (default: "secret")
  enable_cache: true
```

### Secret Path Translation

**Format:** `{mount_path}/data/{vault_path}` (for KV v2)

**Examples:**

| Vault Path             | HashiCorp Path                        |
|------------------------|---------------------------------------|
| `llm/openai/api_key`   | `secret/data/llm/openai/api_key`      |
| `database/postgres/password` | `secret/data/database/postgres/password` |

### Setup: Creating Secrets
```bash
# Using Vault CLI
vault kv put secret/llm/openai/api_key value="sk-proj-..."
vault kv put secret/llm/anthropic/api_key value="sk-ant-..."
vault kv put secret/database/postgres/password value="mysecretpassword"

# Or using API
curl -X POST https://vault.example.com:8200/v1/secret/data/llm/openai/api_key \
  -H "X-Vault-Token: $VAULT_TOKEN" \
  -d '{"data": {"value": "sk-proj-..."}}'
```

## AWS Secrets Manager

AWS Secrets Manager stores secrets with a direct name mapping.

### Configuration
```yaml
vault:
  type: "aws"
  aws_region: "us-east-1"
  # Credentials from environment or IAM role
  enable_cache: true
```

### Secret Path Translation

**Format:** Vault path is used directly as the secret name (slashes preserved)

**Examples:**

| Vault Path                | AWS Secret Name                |
|---------------------------|--------------------------------|
| `llm/openai/api_key`      | `llm/openai/api_key`           |
| `database/postgres/password` | `database/postgres/password` |

Or with prefix:

| Vault Path                | AWS Secret Name (with prefix)       |
|---------------------------|-------------------------------------|
| `llm/openai/api_key`      | `onlyagents/llm/openai/api_key`     |

### Setup: Creating Secrets
```bash
# Using AWS CLI
aws secretsmanager create-secret \
  --name "llm/openai/api_key" \
  --secret-string "sk-proj-..."

aws secretsmanager create-secret \
  --name "llm/anthropic/api_key" \
  --secret-string "sk-ant-..."

# With prefix
aws secretsmanager create-secret \
  --name "onlyagents/llm/openai/api_key" \
  --secret-string "sk-proj-..."
```

## GCP Secret Manager

GCP Secret Manager uses project-scoped secret names.

### Configuration
```yaml
vault:
  type: "gcp"
  gcp_project_id: "my-project-123"
  gcp_credentials: "/path/to/service-account.json"  # Or use default credentials
  enable_cache: true
```

### Secret Path Translation

**Format:** Vault path converted to GCP secret name (slashes → hyphens)

**Examples:**

| Vault Path                | GCP Secret Name                     |
|---------------------------|-------------------------------------|
| `llm/openai/api_key`      | `llm-openai-api-key`                |
| `database/postgres/password` | `database-postgres-password`      |

Or with prefix:

| Vault Path                | GCP Secret Name (with prefix)            |
|---------------------------|------------------------------------------|
| `llm/openai/api_key`      | `onlyagents-llm-openai-api-key`          |

### Setup: Creating Secrets
```bash
# Using gcloud CLI
echo -n "sk-proj-..." | gcloud secrets create llm-openai-api-key \
  --data-file=- \
  --replication-policy="automatic"

echo -n "sk-ant-..." | gcloud secrets create llm-anthropic-api-key \
  --data-file=- \
  --replication-policy="automatic"

# With prefix
echo -n "sk-proj-..." | gcloud secrets create onlyagents-llm-openai-api-key \
  --data-file=- \
  --replication-policy="automatic"
```

## Configuration Examples

### Example 1: Development (.env)
```yaml
# agent.yaml
vault:
  type: "env"
  prefix: "ONLYAGENTS_"

llm:
  provider: "openai"
  model: "gpt-4"
  api_key_vault: "llm/openai/api_key"
```
```bash
# .env
OPENAI_API_KEY=sk-proj-...
```

### Example 2: Production (HashiCorp Vault)
```yaml
# agent.yaml
vault:
  type: "hashicorp"
  address: "https://vault.prod.example.com:8200"
  token: "${VAULT_TOKEN}"
  mount_path: "secret"
  enable_cache: true
  audit_log: true

llm:
  provider: "openai"
  model: "gpt-4"
  api_key_vault: "llm/openai/api_key"
```

### Example 3: AWS Deployment
```yaml
# agent.yaml
vault:
  type: "aws"
  aws_region: "us-east-1"
  enable_cache: true

llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  api_key_vault: "onlyagents/llm/anthropic/api_key"
```

## Best Practices

1. **Never commit secrets** to version control
2. **Use different secrets** for dev/staging/production
3. **Enable vault caching** for performance
4. **Enable audit logging** in production
5. **Rotate secrets regularly**
6. **Use IAM roles** in cloud environments (don't hardcode credentials)
7. **Use separate vault paths** for different environments:
   - `dev/llm/openai/api_key`
   - `staging/llm/openai/api_key`
   - `prod/llm/openai/api_key`

## Troubleshooting

### Secret Not Found
```
Error: failed to get API key from vault: secret not found: llm/openai/api_key
```

**For EnvVault:** Check that you have one of these set:
- `ONLYAGENTS_LLM_OPENAI_API_KEY`
- `OPENAI_API_KEY`

**For HashiCorp:** Check the secret exists:
```bash
vault kv get secret/llm/openai/api_key
```

**For AWS:** Check the secret exists:
```bash
aws secretsmanager describe-secret --secret-id "llm/openai/api_key"
```

### Vault Connection Issues
```
Error: failed to initialize vault: vault authentication failed
```

Check:
- Vault address is correct
- Token/credentials are valid
- Network connectivity to vault
- IAM permissions (for AWS/GCP)
