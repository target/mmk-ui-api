# Secret Provider Script Example

This directory contains an example provider script for Merrymaker's dynamic secret refresh functionality. This script demonstrates the OAuth Client Credentials flow, which is one of the most common authentication patterns for automated systems.

## Overview

Provider scripts are executable files that fetch or refresh secret values automatically. They follow a simple contract:

- **Input**: Configuration via environment variables
- **Output**: New secret value to stdout (on success)
- **Errors**: Error messages to stderr with non-zero exit code

## Example Script

### OAuth Token Refresh (`oauth-token.sh`)

**Use Case:** Refresh OAuth access tokens using client credentials flow

**Required Environment Variables:**

- `CLIENT_ID`: OAuth client identifier
- `CLIENT_SECRET`: OAuth client secret
- `TOKEN_URL`: OAuth token endpoint URL
- `SCOPE` (optional): Requested OAuth scopes

**Example Usage:**

```bash
# Set environment variables
export CLIENT_ID="your-client-id"
export CLIENT_SECRET="your-client-secret"
export TOKEN_URL="https://auth.example.com/oauth/token"
export SCOPE="read write"

# Run script
./oauth-token.sh
```

## Installation

1. **Copy Script**: Copy the script to your server
2. **Set Permissions**: Make script executable
3. **Install Dependencies**: Ensure required tools are available
4. **Test Script**: Run script manually before configuring in Merrymaker

```bash
# Copy script to your preferred location
cp oauth-token.sh /opt/scripts/

# Set executable permissions
chmod +x /opt/scripts/oauth-token.sh

# Install dependencies (Ubuntu/Debian)
apt-get update && apt-get install -y curl jq

# Test the script
export CLIENT_ID="test" CLIENT_SECRET="test" TOKEN_URL="https://httpbin.org/post"
/opt/scripts/oauth-token.sh
```

## Configuration in Merrymaker

### Using the Web UI

1. Navigate to **Secrets** → **Create Secret**
2. Fill in basic information (name, type, initial value)
3. Enable **Dynamic Secret Configuration**
4. Set **Provider Script Path** to your script location
5. Configure **Environment Variables** as JSON
6. Set **Refresh Interval** (minimum 60 seconds)

### Using the API

```bash
curl -X POST http://localhost:8080/api/secrets \
  -H "Content-Type: application/json" \
  -d '{
    "name": "oauth_access_token",
    "type": "oauth_token",
    "value": "initial-token",
    "provider_script_path": "/opt/scripts/oauth-token.sh",
    "env_config": "{\"CLIENT_ID\":\"abc123\",\"CLIENT_SECRET\":\"xyz789\",\"TOKEN_URL\":\"https://auth.example.com/token\"}",
    "refresh_interval_seconds": 3600,
    "refresh_enabled": true
  }'
```

## Security Considerations

### File Permissions

The script should be readable and executable only by the Merrymaker service user:

```bash
# Set restrictive permissions
chmod 750 /opt/scripts/oauth-token.sh
chown merrymaker:merrymaker /opt/scripts/oauth-token.sh
```

### Environment Variables

- Store sensitive data in Merrymaker's `env_config` field, not in script files
- Never log environment variables containing secrets
- Validate all input parameters

### Network Security

- Always use HTTPS for external API calls
- Implement proper timeout handling
- Validate SSL certificates (don't use `-k` with curl)

## Troubleshooting

### Common Issues

**Permission Denied:**

```bash
# Check file permissions
ls -la /opt/scripts/oauth-token.sh

# Fix permissions
chmod +x /opt/scripts/oauth-token.sh
```

**Missing Dependencies:**

```bash
# Check if tools are available
which curl jq oathtool

# Install missing tools
apt-get install -y curl jq oathtool
```

**Script Fails:**

```bash
# Run script manually with debug output
bash -x /opt/scripts/oauth-token.sh

# Check environment variables
env | grep -E "(CLIENT_ID|CLIENT_SECRET|TOKEN_URL)"
```

### Debug Mode

The script supports a `DEBUG` environment variable for verbose output:

```bash
export DEBUG="true"
export CLIENT_ID="test"
export CLIENT_SECRET="test"
./oauth-token.sh
```

### QT Token Provider (`qt-token-provider`)

**Use Case:** Fetch a Quantum Tunnel (QT) token by chaining an OAuth2/OIDC ROPC flow with a QT service call.

**Implementation:** A lightweight Go CLI that lives at `cmd/qt-token-provider`. It produces a single static binary (no external runtime dependencies) and follows the secret-provider contract: logs to stderr, prints the refreshed token to stdout, exits non-zero on failure.

**Required Environment Variables:**

- `OAUTH_TOKEN_URL`: OAuth/OIDC token endpoint (set this or provide `OAUTH_DISCOVERY_URL`)
- `OAUTH_QT_CLIENT_ID`: OAuth client identifier for the QT integration
- `OAUTH_QT_CLIENT_SECRET`: OAuth client secret
- `QT_USER`: Service account username for ROPC
- `QT_PASSWORD`: Service account password for ROPC
- `QT_URI`: QT service endpoint that issues tokens
- `QT_API_KEY`: API key associated with the QT service account

**Optional Environment Variables:**

- `OAUTH_SCOPE`: Space-separated scopes (defaults to `profile email openid`)
- `REQUEST_TIMEOUT_SECONDS`: Per-request timeout in seconds (defaults to `10`)
- `DEBUG`: Set to `true` for verbose logging to stderr
- `OAUTH_DISCOVERY_URL`: OAuth/OIDC issuer or discovery document used to derive the token endpoint when `OAUTH_TOKEN_URL` is not provided (accepts either the issuer base URL like `https://auth.example.com` or the full discovery document `https://auth.example.com/.well-known/openid-configuration`)

**Build:**

```bash
go build -o bin/qt-token-provider ./cmd/qt-token-provider
```

**Example Invocation:**

```bash
export OAUTH_TOKEN_URL="https://auth.example.com/oauth/token"
export OAUTH_QT_CLIENT_ID="client-id"
export OAUTH_QT_CLIENT_SECRET="client-secret"
export QT_USER="service-account@example.com"
export QT_PASSWORD="super-secret"
export QT_URI="https://qt.example.com/v1/token"
export QT_API_KEY="api-key"

./bin/qt-token-provider
```

Alternatively, you can supply either the issuer or the full discovery document instead of the token endpoint:

```bash
export OAUTH_DISCOVERY_URL="https://auth.example.com"
export OAUTH_QT_CLIENT_ID="client-id"
export OAUTH_QT_CLIENT_SECRET="client-secret"
export QT_USER="service-account@example.com"
export QT_PASSWORD="super-secret"
export QT_URI="https://qt.example.com/v1/token"
export QT_API_KEY="api-key"

./bin/qt-token-provider
```

```bash
export OAUTH_DISCOVERY_URL="https://auth.example.com/.well-known/openid-configuration"
export OAUTH_QT_CLIENT_ID="client-id"
export OAUTH_QT_CLIENT_SECRET="client-secret"
export QT_USER="service-account@example.com"
export QT_PASSWORD="super-secret"
export QT_URI="https://qt.example.com/v1/token"
export QT_API_KEY="api-key"

./bin/qt-token-provider
```

The binary writes observability logs to stderr and emits only the QT token to stdout so Merrymaker can capture the refreshed secret value cleanly.

## Customization

This script is a template that you can modify for your specific needs:

1. **Error Handling**: Add custom error handling logic
2. **Validation**: Add input validation for your environment
3. **Logging**: Add structured logging for monitoring
4. **Retry Logic**: Add retry mechanisms for network failures
5. **Caching**: Add local caching for rate-limited APIs

### Other Authentication Patterns

You can adapt this script for other authentication flows:

- **API Key Rotation**: Replace OAuth logic with API key rotation calls
- **Multi-Step Authentication**: Add additional steps like MFA verification
- **Certificate Renewal**: Fetch and extract certificates or private keys
- **Database Credentials**: Rotate database passwords via admin APIs

## Contributing

When creating new provider scripts:

1. Follow the provider script contract (see API documentation)
2. Include comprehensive comments explaining the flow
3. Add error handling for common failure scenarios
4. Document required environment variables
5. Test scripts thoroughly before deployment

## Support

For questions about these examples or the secret refresh functionality:

1. Check the main API documentation: `docs/SECRET_REFRESH_API.md`
2. Review the architecture documentation: `docs/secret-refresh-architecture.md`
3. Examine the integration tests: `internal/service/secret_refresh_test.go`
