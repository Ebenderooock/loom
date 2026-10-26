package proxies

import (
	"encoding/json"
	"net/url"
	"strings"
	"unicode"
)

func redactProxy(proxy Proxy) Proxy {
	proxy.Config = redactConfigSecrets(proxy.Config)
	return proxy
}

func redactConfigSecrets(config json.RawMessage) json.RawMessage {
	var value any
	if json.Unmarshal(config, &value) != nil {
		return json.RawMessage(`{}`)
	}
	encoded, err := json.Marshal(redactSecretValue(value))
	if err != nil {
		return config
	}
	return encoded
}

func redactSecretValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, child := range value {
			if isSecretConfigKey(key) {
				continue
			}
			if isURLConfigKey(key) {
				if text, ok := child.(string); ok {
					out[key] = redactSecretURL(text)
					continue
				}
			}
			out[key] = redactSecretValue(child)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, child := range value {
			out[i] = redactSecretValue(child)
		}
		return out
	case string:
		return redactSecretURL(value)
	default:
		return value
	}
}

func preserveSecretContainer(current, incoming map[string]any) {
	for key, oldValue := range current {
		newValue, exists := incoming[key]
		oldChild, oldOK := oldValue.(map[string]any)
		newChild, newOK := newValue.(map[string]any)
		if oldOK && newOK {
			preserveSecretContainer(oldChild, newChild)
		} else if !exists || isEmptyOrMaskedSecret(newValue) {
			incoming[key] = oldValue
		}
	}
}

func preserveConfigSecrets(current, incoming json.RawMessage) json.RawMessage {
	if len(incoming) == 0 || strings.TrimSpace(string(incoming)) == "null" {
		return current
	}
	var oldValue, newValue any
	if json.Unmarshal(current, &oldValue) != nil || json.Unmarshal(incoming, &newValue) != nil {
		return incoming
	}
	oldMap, oldOK := oldValue.(map[string]any)
	newMap, newOK := newValue.(map[string]any)
	if !oldOK || !newOK {
		return incoming
	}
	preserveSecretFields(oldMap, newMap)
	encoded, err := json.Marshal(newMap)
	if err != nil {
		return incoming
	}
	return encoded
}

func preserveSecretFields(current, incoming map[string]any) {
	for key, oldValue := range current {
		newValue, exists := incoming[key]
		if isSecretConfigKey(key) {
			oldChild, oldOK := oldValue.(map[string]any)
			newChild, newOK := newValue.(map[string]any)
			if oldOK && newOK {
				preserveSecretContainer(oldChild, newChild)
				continue
			}
			if !exists || isEmptyOrMaskedSecret(newValue) {
				incoming[key] = oldValue
			}
			continue
		}
		if isURLConfigKey(key) {
			if oldURL, oldOK := oldValue.(string); oldOK {
				if newURL, newOK := newValue.(string); newOK {
					incoming[key] = preserveSecretURL(oldURL, newURL)
					continue
				}
			}
		}
		oldChild, oldOK := oldValue.(map[string]any)
		newChild, newOK := newValue.(map[string]any)
		if oldOK && newOK {
			preserveSecretFields(oldChild, newChild)
		} else if oldOK && !exists {
			child := make(map[string]any)
			preserveSecretFields(oldChild, child)
			if len(child) > 0 {
				incoming[key] = child
			}
		}
	}
}

func isURLConfigKey(key string) bool {
	var normalized strings.Builder
	for _, r := range strings.ToLower(key) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			normalized.WriteRune(r)
		}
	}
	value := normalized.String()
	return value == "url" || value == "baseurl" || value == "endpoint"
}

func redactSecretURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	changed := u.User != nil
	u.User = nil
	query, queryErr := url.ParseQuery(u.RawQuery)
	if queryErr != nil {
		u.RawQuery = ""
		u.ForceQuery = false
		return u.String()
	}
	for key := range query {
		if isSecretConfigKey(key) {
			query.Del(key)
			changed = true
		}
	}
	if !changed {
		return raw
	}
	u.RawQuery = query.Encode()
	u.ForceQuery = false
	return u.String()
}

func preserveSecretURL(stored, submitted string) string {
	oldURL, oldErr := url.Parse(stored)
	newURL, newErr := url.Parse(submitted)
	if oldErr != nil || newErr != nil {
		return submitted
	}
	if newURL.User == nil || isEmptyOrMaskedSecret(newURL.User.Username()) {
		newURL.User = oldURL.User
	} else if oldURL.User != nil && newURL.User.Username() == oldURL.User.Username() {
		if password, _ := newURL.User.Password(); isEmptyOrMaskedSecret(password) {
			if storedPassword, ok := oldURL.User.Password(); ok {
				newURL.User = url.UserPassword(newURL.User.Username(), storedPassword)
			}
		}
	}
	oldQuery, newQuery := oldURL.Query(), newURL.Query()
	for key, values := range oldQuery {
		if isSecretConfigKey(key) && (len(newQuery[key]) == 0 || isEmptyOrMaskedSecret(newQuery.Get(key))) {
			newQuery[key] = values
		}
	}
	newURL.RawQuery = newQuery.Encode()
	return newURL.String()
}
func isEmptyOrMaskedSecret(value any) bool {
	if value == nil {
		return true
	}
	text, ok := value.(string)
	if !ok {
		return false
	}
	if len([]rune(text)) >= 3 && strings.Trim(text, "*•") == "" {
		return true
	}
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "", "********", "••••••••", "[redacted]", "<redacted>":
		return true
	default:
		return false
	}
}

func isSecretConfigKey(key string) bool {
	var normalized strings.Builder
	for _, r := range strings.ToLower(key) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			normalized.WriteRune(r)
		}
	}
	key = normalized.String()
	for _, marker := range []string{
		"password", "passwd", "apikey", "passkey", "cookie", "token",
		"secret", "credential", "privatekey", "passphrase", "authorization", "bearer",
	} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return key == "username" || key == "user" || key == "key" || key == "pass" || key == "auth"
}
