package imports

// GetTitle returns a human-readable title for notification formatting.
func (e *ImportCompletedEvent) GetTitle() string { return e.Title }

// NotificationData returns structured data for notification templates.
func (e *ImportCompletedEvent) NotificationData() map[string]any {
	return map[string]any{
		"title":      e.Title,
		"media_type": string(e.MediaType),
		"media_id":   e.MediaID,
		"dest_path":  e.DestPath,
	}
}

// GetTitle returns a human-readable title for notification formatting.
func (e *ImportFailedEvent) GetTitle() string { return e.Title }

// NotificationData returns structured data for notification templates.
func (e *ImportFailedEvent) NotificationData() map[string]any {
	return map[string]any{
		"title": e.Title,
		"error": e.Error,
	}
}
