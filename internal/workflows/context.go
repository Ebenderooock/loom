package workflows

import "context"

type workflowCtxKey struct{}

// WithWorkflowID returns a child context carrying the given workflow ID.
func WithWorkflowID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, workflowCtxKey{}, id)
}

// WorkflowIDFromContext extracts the workflow ID from ctx, if present.
func WorkflowIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(workflowCtxKey{}).(string)
	return id, ok && id != ""
}
