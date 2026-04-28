package jobs

import (
	"context"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// MarkInterruptedAtStartup transitions any rows still in queued or running
// state at process boot to "interrupted". Called from cmd/nova-api/main.go
// before the worker starts consuming tasks.
func MarkInterruptedAtStartup(ctx context.Context, q *storedb.Queries) error {
	return q.MarkRunningInterrupted(ctx)
}
