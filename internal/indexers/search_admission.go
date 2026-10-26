package indexers

import "context"

type searchAdmissionKey struct{}

// WithSearchAdmission gates each actual provider query in non-streaming Search,
// after query eligibility and circuit-breaker filtering. admit must atomically
// reserve any quota and be safe for concurrent use. A denied query is skipped,
// never expanded to all IDs. Without a callback (e.g. manual searches), fanout
// is unchanged.
func WithSearchAdmission(ctx context.Context, admit func(indexerID string) bool) context.Context {
	return context.WithValue(ctx, searchAdmissionKey{}, admit)
}

func admitSearch(ctx context.Context, id string) bool {
	admit, _ := ctx.Value(searchAdmissionKey{}).(func(string) bool)
	return admit == nil || admit(id)
}
