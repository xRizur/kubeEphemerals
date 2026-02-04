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

import (
	"context"
	"net/http"
)

const devUser = "dev@local"

// AuthMiddleware returns an http middleware that sets the request user in context.
// In production: user is taken from X-Forwarded-User (set by OAuth2 Proxy).
// When UnsafeDevMode is true: if no header is present, user is set to "dev@local".
// When UnsafeDevMode is false and no X-Forwarded-User: responds with 401 Unauthorized.
func AuthMiddleware(UnsafeDevMode bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := r.Header.Get("X-Forwarded-User")
			if user == "" {
				if UnsafeDevMode {
					user = devUser
				} else {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
			}
			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
