package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ResolveSecretPlaceholders fetches the provided secret names and replaces their
// __NAME__ placeholders within content. If repo is nil, secrets is empty, or
// content lacks placeholders, the original content is returned unchanged.
func ResolveSecretPlaceholders(
	ctx context.Context,
	repo SecretRepository,
	secretNames []string,
	content string,
) (string, error) {
	if len(secretNames) == 0 || strings.TrimSpace(content) == "" {
		return content, nil
	}
	if repo == nil {
		return "", errors.New("secret repository not configured")
	}

	seen := make(map[string]struct{}, len(secretNames))
	resolved := content

	for _, name := range secretNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}

		placeholder := "__" + name + "__"
		if !strings.Contains(resolved, placeholder) {
			continue
		}

		secret, err := repo.GetByName(ctx, name)
		if err != nil {
			return "", fmt.Errorf("resolve secret %q: %w", name, err)
		}
		resolved = strings.ReplaceAll(resolved, placeholder, secret.Value)
	}

	return resolved, nil
}
