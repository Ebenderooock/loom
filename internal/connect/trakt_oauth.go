package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// TraktOAuthRouter returns routes for the Trakt OAuth2 flow.
func TraktOAuthRouter(svc Service) chi.Router {
	r := chi.NewRouter()
	r.Post("/authorize", handleTraktAuthorize(svc))
	r.Post("/callback", handleTraktCallback(svc))
	r.Post("/refresh/{id}", handleTraktRefresh(svc))
	return r
}

type traktAuthorizeRequest struct {
	ClientID    string `json:"client_id"`
	RedirectURI string `json:"redirect_uri"`
}

func handleTraktAuthorize(_ Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req traktAuthorizeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if req.ClientID == "" {
			writeError(w, http.StatusBadRequest, "client_id is required")
			return
		}
		if req.RedirectURI == "" {
			writeError(w, http.StatusBadRequest, "redirect_uri is required")
			return
		}

		params := url.Values{
			"response_type": {"code"},
			"client_id":     {req.ClientID},
			"redirect_uri":  {req.RedirectURI},
		}
		authorizeURL := "https://trakt.tv/oauth/authorize?" + params.Encode()

		writeJSON(w, http.StatusOK, map[string]any{
			"authorize_url": authorizeURL,
		})
	}
}

type traktCallbackRequest struct {
	Code         string `json:"code"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	ConnectionID string `json:"connection_id"`
}

type traktTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	CreatedAt    int64  `json:"created_at"`
	TokenType    string `json:"token_type"`
}

func handleTraktCallback(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req traktCallbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		for _, check := range []struct{ field, name string }{
			{req.Code, "code"},
			{req.ClientID, "client_id"},
			{req.ClientSecret, "client_secret"},
			{req.RedirectURI, "redirect_uri"},
			{req.ConnectionID, "connection_id"},
		} {
			if check.field == "" {
				writeError(w, http.StatusBadRequest, check.name+" is required")
				return
			}
		}

		tokenResp, err := exchangeTraktToken(r.Context(), map[string]string{
			"code":          req.Code,
			"client_id":     req.ClientID,
			"client_secret": req.ClientSecret,
			"redirect_uri":  req.RedirectURI,
			"grant_type":    "authorization_code",
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}

		conn, err := svc.GetConnection(r.Context(), req.ConnectionID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		expiry := time.Unix(tokenResp.CreatedAt+tokenResp.ExpiresIn, 0).UTC()
		conn.Settings.ClientID = req.ClientID
		conn.Settings.ClientSecret = req.ClientSecret
		conn.Settings.AccessToken = tokenResp.AccessToken
		conn.Settings.RefreshToken = tokenResp.RefreshToken
		conn.Settings.TokenExpiry = expiry.Format(time.RFC3339)

		if err := svc.UpdateConnection(r.Context(), conn); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, conn)
	}
}

func handleTraktRefresh(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		conn, err := svc.GetConnection(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if conn.Provider != ProviderTrakt {
			writeError(w, http.StatusBadRequest, "connection is not a trakt provider")
			return
		}
		if conn.Settings.RefreshToken == "" {
			writeError(w, http.StatusBadRequest, "no refresh token available")
			return
		}

		tokenResp, err := exchangeTraktToken(r.Context(), map[string]string{
			"refresh_token": conn.Settings.RefreshToken,
			"client_id":     conn.Settings.ClientID,
			"client_secret": conn.Settings.ClientSecret,
			"redirect_uri":  "urn:ietf:wg:oauth:2.0:oob",
			"grant_type":    "refresh_token",
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}

		expiry := time.Unix(tokenResp.CreatedAt+tokenResp.ExpiresIn, 0).UTC()
		conn.Settings.AccessToken = tokenResp.AccessToken
		conn.Settings.RefreshToken = tokenResp.RefreshToken
		conn.Settings.TokenExpiry = expiry.Format(time.RFC3339)

		if err := svc.UpdateConnection(r.Context(), conn); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, conn)
	}
}

// exchangeTraktToken posts to the Trakt OAuth token endpoint.
func exchangeTraktToken(ctx context.Context, params map[string]string) (*traktTokenResponse, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("trakt token: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.trakt.tv/oauth/token", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("trakt token: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trakt token: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("trakt token: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trakt token: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp traktTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("trakt token: parse response: %w", err)
	}
	return &tokenResp, nil
}
