package notifications

import (
	"bytes"
	"fmt"
	"text/template"
)

// TemplateData holds variables available in notification templates.
type TemplateData struct {
	Title     string
	Year      string
	Quality   string
	Indexer   string
	Size      string
	EventType string
	MediaType string
}

// Default templates per event type.
var defaultTemplates = map[EventType]string{
	EventOnGrab:              "Grabbed: {{.Title}} ({{.Year}}) — {{.Quality}} from {{.Indexer}}",
	EventOnDownload:          "Download complete: {{.Title}} ({{.Year}}) — {{.Quality}}",
	EventOnUpgrade:           "Upgraded: {{.Title}} ({{.Year}}) to {{.Quality}}",
	EventOnRename:            "Renamed: {{.Title}} ({{.Year}})",
	EventOnDelete:            "Deleted: {{.Title}} ({{.Year}})",
	EventOnHealthIssue:       "Health issue: {{.Title}}",
	EventOnApplicationUpdate: "Application updated: {{.Title}}",
	EventOnTest:              "Test notification: {{.Title}}",
}

// DefaultTemplate returns the default template string for an event type.
func DefaultTemplate(event EventType) string {
	if t, ok := defaultTemplates[event]; ok {
		return t
	}
	return "{{.EventType}}: {{.Title}}"
}

// RenderTemplate renders a notification message using a Go text/template.
// If templateStr is empty, the default template for the event type is used.
func RenderTemplate(templateStr string, event EventType, data map[string]any) (string, error) {
	if templateStr == "" {
		templateStr = DefaultTemplate(event)
	}

	td := TemplateData{
		EventType: string(event),
	}

	if v, ok := data["title"].(string); ok {
		td.Title = v
	}
	if v, ok := data["year"].(string); ok {
		td.Year = v
	}
	if v, ok := data["quality"].(string); ok {
		td.Quality = v
	}
	if v, ok := data["indexer"].(string); ok {
		td.Indexer = v
	}
	if v, ok := data["size"].(string); ok {
		td.Size = v
	}
	if v, ok := data["media_type"].(string); ok {
		td.MediaType = v
	}

	tmpl, err := template.New("notification").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, td); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// AvailableVariables returns the list of template variables for display in the UI.
func AvailableVariables() []string {
	return []string{
		"{{.Title}}",
		"{{.Year}}",
		"{{.Quality}}",
		"{{.Indexer}}",
		"{{.Size}}",
		"{{.EventType}}",
		"{{.MediaType}}",
	}
}
