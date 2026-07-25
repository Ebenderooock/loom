//go:build integration

package integration

import (
	"net/http"
	"testing"
)

func TestAuthFlows(t *testing.T) {
	s := New(t)

	okLogin := s.MustPost(t, "/api/v1/auth/login", map[string]string{
		"username": "admin",
		"password": "testpassword",
	})
	requireStatus(t, okLogin, http.StatusOK)
	_ = okLogin.Body.Close()

	badLogin := s.MustPost(t, "/api/v1/auth/login", map[string]string{
		"username": "admin",
		"password": "wrong-password",
	})
	requireStatus(t, badLogin, http.StatusUnauthorized)
	_ = badLogin.Body.Close()

	unauthMe, err := (&http.Client{}).Get(s.URL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("call /auth/me unauthenticated: %v", err)
	}
	requireStatus(t, unauthMe, http.StatusUnauthorized)
	_ = unauthMe.Body.Close()

	authedMe := s.MustGet(t, "/api/v1/auth/me")
	requireStatus(t, authedMe, http.StatusOK)
	var me struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	decodeJSON(t, authedMe, &me)
	if me.Username != "admin" || me.Role != "admin" {
		t.Fatalf("unexpected /auth/me payload: %+v", me)
	}
}
