#!/bin/bash

set -euo pipefail

: "${ENV_SECRETS_DIR:=/run/secrets}"
: "${ENV_SECRETS_DEBUG:=}"

# Output to stderr for proper logging
function env_secret_debug() {
	if [[ -n "${ENV_SECRETS_DEBUG}" ]]; then
		printf "\033[1m[SECRETS]\033[0m %s\n" "$*" >&2
	fi
}

function env_secret_error() {
	printf "\033[1;31m[SECRETS ERROR]\033[0m %s\n" "$*" >&2
}

# usage: env_secret_expand VAR
# Expands: VAR=DOCKER-SECRET->secret-name
env_secret_expand() {
	local var="$1"
	local val="${!var:-}"

	# Match DOCKER-SECRET-> pattern (improved regex)
	if [[ "$val" =~ ^DOCKER-SECRET-\>([a-zA-Z0-9._-]+)$ ]]; then
		local secret_name="${BASH_REMATCH[1]}"
		local secret_path="${ENV_SECRETS_DIR}/${secret_name}"

		env_secret_debug "Processing secret for $var: $secret_name"

		# Verify secret file exists and is readable
		if [[ ! -f "$secret_path" ]]; then
			env_secret_error "Secret file not found: $secret_path"
			return 1
		fi

		if [[ ! -r "$secret_path" ]]; then
			env_secret_error "Secret file not readable: $secret_path"
			return 1
		fi

		# Read secret content
		if ! val=$(<"$secret_path"); then
			env_secret_error "Failed to read secret: $secret_path"
			return 1
		fi

		# Trim common trailing newline/carriage return added by secret stores
		val="${val%$'\n'}"
		val="${val%$'\r'}"

		# Validate secret is not empty
		if [[ -z "$val" ]]; then
			env_secret_error "Secret is empty: $secret_path"
			return 1
		fi

		# Export the expanded variable
		export "$var=$val"
		env_secret_debug "✓ Expanded $var (${#val} chars)"

		# Clear the variable from this scope for security
		unset val
	fi

	return 0
}

env_secrets_expand() {
	local var entry
	local expanded=0
	local failed=0

	env_secret_debug "Scanning environment variables for secrets..."

	# Use safer iteration method
	while IFS= read -r -d '' entry; do
		var=${entry%%=*}
		# Skip if variable name is empty or contains special chars
		[[ -z "$var" || "$var" =~ [^a-zA-Z0-9_] ]] && continue

		if env_secret_expand "$var"; then
			((expanded += 1))
		else
			((failed += 1))
		fi
	done < <(printenv -0)

	if [[ -n "${ENV_SECRETS_DEBUG}" ]]; then
		env_secret_debug "Summary: $expanded secrets expanded, $failed failed"
		env_secret_debug "\n--- Environment Variables ---"
		env | sort >&2
	fi

	# Fail if any secrets failed to expand (optional - comment out if you want soft failures)
	if ((failed > 0)); then
		env_secret_error "Failed to expand $failed secret(s)"
		exit 1
	fi
}

# Main execution
env_secrets_expand

# Execute the CMD
exec "$@"
