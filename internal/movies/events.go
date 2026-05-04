package movies

// Event topics for the movies service. These are published to the event bus
// for downstream consumers (e.g., search indexing, dashboard updates).
const (
	TopicMovieAdded           = "movies.added"
	TopicMovieUpdated         = "movies.updated"
	TopicMovieDeleted         = "movies.deleted"
	TopicMonitoringChanged    = "movies.monitoring_changed"
)
