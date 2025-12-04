#!/bin/bash
#
# OAuth Token Refresh Script
#
# This script implements OAuth 2.0 Client Credentials flow to fetch access tokens.
# It's designed to be used with Merrymaker's dynamic secret refresh functionality.
#
# Required Environment Variables:
#   CLIENT_ID     - OAuth client identifier
#   CLIENT_SECRET - OAuth client secret
#   TOKEN_URL     - OAuth token endpoint URL
#
# Optional Environment Variables:
#   SCOPE         - Requested OAuth scopes (space-separated)
#   DEBUG         - Enable debug output (set to "true")
#
# Output:
#   Success: Writes access token to stdout
#   Failure: Writes error message to stderr and exits with non-zero code
#
# Example Usage:
#   export CLIENT_ID="your-client-id"
#   export CLIENT_SECRET="your-client-secret"
#   export TOKEN_URL="https://auth.example.com/oauth/token"
#   export SCOPE="read write"
#   ./oauth-token.sh
#

set -euo pipefail

# Enable debug logging if requested
DEBUG="${DEBUG:-false}"
debug_log() {
    if [[ "$DEBUG" == "true" ]]; then
        echo "DEBUG: $*" >&2
    fi
}
# Ensure jq is available for JSON parsing
if ! command -v jq >/dev/null 2>&1; then
    echo "ERROR: jq is required" >&2
    exit 1
fi


# Validate required environment variables
if [[ -z "${CLIENT_ID:-}" ]]; then
    echo "ERROR: CLIENT_ID environment variable is required" >&2
    exit 1
fi

if [[ -z "${CLIENT_SECRET:-}" ]]; then
    echo "ERROR: CLIENT_SECRET environment variable is required" >&2
    exit 1
fi

if [[ -z "${TOKEN_URL:-}" ]]; then
    echo "ERROR: TOKEN_URL environment variable is required" >&2
    exit 1
fi

# Validate TOKEN_URL format
if [[ ! "$TOKEN_URL" =~ ^https:// ]]; then
    echo "ERROR: TOKEN_URL must use HTTPS protocol" >&2
    exit 1
fi

# Optional scope parameter
SCOPE="${SCOPE:-}"

debug_log "Starting OAuth token refresh"
debug_log "Client ID: ${CLIENT_ID}"
debug_log "Token URL: ${TOKEN_URL}"
debug_log "Scope: ${SCOPE:-"(none)"}"

# Build request data for client credentials flow using curl URL encoding

debug_log "Making token request to ${TOKEN_URL}"

# Make the token request
# - Use POST method with form data
# - Set appropriate Content-Type header
# - Follow redirects but limit to 3
# - Set reasonable timeout (30 seconds)
# - Fail on HTTP error codes (4xx, 5xx)
response=$(curl -sS -f \
    --max-time 30 \
    --max-redirs 3 \
    -X POST \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -H "Accept: application/json" \
    --data-urlencode "grant_type=client_credentials" \
    --data-urlencode "client_id=${CLIENT_ID}" \
    --data-urlencode "client_secret=${CLIENT_SECRET}" \
    $( [[ -n "$SCOPE" ]] && echo --data-urlencode "scope=${SCOPE}" ) \
    "$TOKEN_URL") || {

    # Capture curl exit code for better error reporting
    curl_exit_code=$?
    case $curl_exit_code in
        6)  echo "ERROR: Could not resolve host: $TOKEN_URL" >&2 ;;
        7)  echo "ERROR: Failed to connect to $TOKEN_URL" >&2 ;;
        22) echo "ERROR: HTTP error response from $TOKEN_URL" >&2 ;;
        28) echo "ERROR: Timeout connecting to $TOKEN_URL" >&2 ;;
        *)  echo "ERROR: Failed to fetch token from $TOKEN_URL (curl exit code: $curl_exit_code)" >&2 ;;
    esac
    exit 1
}

debug_log "Received response from token endpoint"

# Validate that response is valid JSON
if ! echo "$response" | jq empty 2>/dev/null; then
    echo "ERROR: Invalid JSON response from token endpoint" >&2
    debug_log "Response was: $response"
    exit 1
fi

# Extract access token from response
# Use jq to safely parse JSON and extract access_token field
access_token=$(echo "$response" | jq -r '.access_token // empty')

if [[ -z "$access_token" ]] || [[ "$access_token" == "null" ]]; then
    echo "ERROR: No access_token found in response" >&2

    # Try to extract error information if available
    error_description=$(echo "$response" | jq -r '.error_description // .error // "Unknown error"')
    echo "ERROR: OAuth error: $error_description" >&2

    debug_log "Full response: $response"
    exit 1
fi

# Validate token format (basic sanity check)
if [[ ${#access_token} -lt 10 ]]; then
    echo "ERROR: Access token appears to be too short (${#access_token} characters)" >&2
    exit 1
fi

debug_log "Successfully extracted access token (${#access_token} characters)"

# Optional: Extract and log token expiration for debugging
if [[ "$DEBUG" == "true" ]]; then
    expires_in=$(echo "$response" | jq -r '.expires_in // "unknown"')
    token_type=$(echo "$response" | jq -r '.token_type // "unknown"')
    debug_log "Token type: $token_type"
    debug_log "Expires in: $expires_in seconds"
fi

# Output the access token to stdout
# This is the only output that should go to stdout
echo "$access_token"

debug_log "OAuth token refresh completed successfully"
