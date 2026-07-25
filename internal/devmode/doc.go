// Package devmode provides embedded HTTP stub servers for all Loom external
// dependencies. When active, every outbound API call is served by a local
// httptest.Server returning canned fixture responses so the app runs fully
// without network access or API keys.
package devmode
