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
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware(t *testing.T) {
	// Dummy handler that reads user from context and writes it to header for assertion
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		w.Header().Set("X-Test-User", user)
		w.WriteHeader(http.StatusOK)
	})

	t.Run("Scenario A: Prod - X-Forwarded-User set to alice", func(t *testing.T) {
		mw := AuthMiddleware(false)
		handler := mw(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-User", "alice")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
		if got := rec.Header().Get("X-Test-User"); got != "alice" {
			t.Errorf("context user: expected alice, got %q", got)
		}
	})

	t.Run("Scenario B: Dev/Bypass - No header, UnsafeDevMode=true", func(t *testing.T) {
		mw := AuthMiddleware(true)
		handler := mw(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
		if got := rec.Header().Get("X-Test-User"); got != "dev@local" {
			t.Errorf("context user: expected dev@local, got %q", got)
		}
	})

	t.Run("Scenario C: Unauthorized - No header, UnsafeDevMode=false", func(t *testing.T) {
		mw := AuthMiddleware(false)
		handler := mw(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})
}
