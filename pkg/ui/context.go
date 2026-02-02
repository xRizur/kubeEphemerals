/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ui

import "context"

type contextKey string

const userContextKey contextKey = "user"

// UserFromContext returns the authenticated user identity from the request context.
// Set by AuthMiddleware from X-Forwarded-User or dev bypass.
func UserFromContext(ctx context.Context) string {
	if v := ctx.Value(userContextKey); v != nil {
		return v.(string)
	}
	return ""
}

// ContextWithUser returns a context with the given user set. Used by tests and by AuthMiddleware.
func ContextWithUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}
