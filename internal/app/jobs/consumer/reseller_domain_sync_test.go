package consumer

import (
	"context"
	"testing"

	"github.com/dujiao-next/internal/queue"
	"github.com/hibiken/asynq"
)

func TestLegacyResellerDomainSyncTaskIsDrainedWithoutServices(t *testing.T) {
	// Already queued tasks must succeed without reaching domain services or being retried.
	mux := asynq.NewServeMux()
	New(nil).Register(mux)
	if err := mux.ProcessTask(context.Background(), asynq.NewTask(queue.TaskResellerDomainSync, nil)); err != nil {
		t.Fatalf("legacy domain task was not drained: %v", err)
	}
}
