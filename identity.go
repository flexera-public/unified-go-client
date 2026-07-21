package flexera

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// UserIDFromToken decodes a Flexera JWT access token (without verifying its
// signature) and returns the integer user ID encoded in either the `sub`
// claim (`u-{id}`) or the legacy numeric `user` claim. It returns an error for
// service-account tokens (subject `sa-...`), which have no user identity.
//
// This supports auto-detecting the caller's user ID for user-scoped IAM
// endpoints (e.g. GET /iam/v1/users/{id}/orgs) when an explicit ID is omitted.
func UserIDFromToken(token string) (int, error) {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, errors.New("provided access token is not a JWT (expected 3 dot-separated segments)")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, fmt.Errorf("failed to decode JWT payload: %w", err)
	}
	var claims struct {
		Sub  string      `json:"sub"`
		User json.Number `json:"user"`
	}
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	dec.UseNumber()
	if err := dec.Decode(&claims); err != nil {
		return 0, fmt.Errorf("failed to parse JWT claims: %w", err)
	}
	if strings.HasPrefix(claims.Sub, "sa-") {
		return 0, errors.New("the provided credentials belong to a service account; this operation requires a user-issued token (use a refresh token or a user access token)")
	}
	if strings.HasPrefix(claims.Sub, "u-") {
		uid, convErr := strconv.Atoi(strings.TrimPrefix(claims.Sub, "u-"))
		if convErr != nil {
			return 0, fmt.Errorf("invalid user ID in JWT sub claim %q: %w", claims.Sub, convErr)
		}
		return uid, nil
	}
	if claims.User.String() != "" {
		uid, convErr := strconv.Atoi(claims.User.String())
		if convErr != nil {
			return 0, fmt.Errorf("invalid legacy user claim %q: %w", claims.User.String(), convErr)
		}
		return uid, nil
	}
	return 0, errors.New("no user identity (`sub: u-*` or `user`) found in JWT")
}
