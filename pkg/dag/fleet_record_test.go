package dag

import (
	"bytes"
	"context"
	"testing"
)

func TestUnplaceableTaskRecordsInfrastructureFailure(t *testing.T) {
	pool := NewPool(localTransport{}, &Worker{
		ID:     "container-only",
		Venues: []string{VenueSandbox},
		CPU:    4,
	})
	task := &Task{Name: "chunk", Venue: VenueUserland}
	res := pool.Exec(context.Background(), Constraints{Venue: VenueUserland}, task,
		TaskIO{Stdout: new(bytes.Buffer), Stderr: new(bytes.Buffer)})

	record := RecordAttempt(task, nil, 1, res)
	if record.Status != RunInfraFailed {
		t.Fatalf("status = %q, want %q", record.Status, RunInfraFailed)
	}
	if record.Failure == nil || record.Failure.Code != FailNoWorker {
		t.Fatalf("failure = %+v, want code %q", record.Failure, FailNoWorker)
	}
	if record.Status.HasVerdict() {
		t.Fatal("an unplaceable task must not produce a conformance verdict")
	}
}
