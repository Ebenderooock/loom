package downloads

// GetTitle returns a human-readable title for notification formatting.
func (e *DownloadQueuedEvent) GetTitle() string { return e.DownloadID }

// NotificationData returns structured data for notification templates.
func (e *DownloadQueuedEvent) NotificationData() map[string]any {
	return map[string]any{
		"download_id": e.DownloadID,
		"client_id":   e.ClientID,
		"title":       e.DownloadID,
	}
}

// GetTitle returns a human-readable title for notification formatting.
func (e *DownloadCompletedEvent) GetTitle() string {
	if e.Title != "" {
		return e.Title
	}
	return e.DownloadID
}

// NotificationData returns structured data for notification templates.
func (e *DownloadCompletedEvent) NotificationData() map[string]any {
	return map[string]any{
		"download_id": e.DownloadID,
		"client_id":   e.ClientID,
		"title":       e.GetTitle(),
		"category":    e.Category,
	}
}

// GetTitle returns a human-readable title for notification formatting.
func (e *DownloadStalledEvent) GetTitle() string {
	if e.Title != "" {
		return e.Title
	}
	return e.DownloadID
}

// NotificationData returns structured data for notification templates.
func (e *DownloadStalledEvent) NotificationData() map[string]any {
	return map[string]any{
		"download_id": e.DownloadID,
		"title":       e.GetTitle(),
		"reason":      e.Reason,
		"action":      e.Action,
	}
}

// GetTitle returns a human-readable title for notification formatting.
func (e *DownloadFailureEvent) GetTitle() string { return e.OriginResultID }

// NotificationData returns structured data for notification templates.
func (e *DownloadFailureEvent) NotificationData() map[string]any {
	return map[string]any{
		"client_id": e.ClientID,
		"title":     e.OriginResultID,
		"error":     e.Error,
	}
}

// GetTitle returns a human-readable title for notification formatting.
func (e *DownloadRetryEvent) GetTitle() string { return e.Title }

// NotificationData returns structured data for notification templates.
func (e *DownloadRetryEvent) NotificationData() map[string]any {
	return map[string]any{
		"title":    e.Title,
		"category": e.Category,
		"reason":   e.Reason,
		"attempt":  e.Attempt,
	}
}
