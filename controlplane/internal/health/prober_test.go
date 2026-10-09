package health

import (
	"testing"
	"time"

	apiv1 "github.com/arcgg/controlplane/pkg/api/v1"
)

func TestProberSnapshotAndStateUpdate(t *testing.T) {
	targets := []*Target{
		{
			ID:      "test-1",
			Address: "127.0.0.1:9099",
			Weight:  100,
			State:   apiv1.BackendState_BACKEND_STATE_HEALTHY,
		},
	}

	prober := NewProber(targets, 1*time.Second, 100*time.Millisecond)

	snapshot := prober.GetSnapshot("rev-test-1")
	if snapshot.GetVersion() != "rev-test-1" {
		t.Fatalf("expected version rev-test-1, got %s", snapshot.GetVersion())
	}

	if len(snapshot.GetBackends()) != 1 {
		t.Fatalf("expected 1 backend target, got %d", len(snapshot.GetBackends()))
	}

	if snapshot.GetBackends()[0].GetState() != apiv1.BackendState_BACKEND_STATE_HEALTHY {
		t.Fatalf("expected backend healthy, got %v", snapshot.GetBackends()[0].GetState())
	}

	// Test manual state modification (draining)
	err := prober.SetTargetState("test-1", apiv1.BackendState_BACKEND_STATE_DRAINING)
	if err != nil {
		t.Fatalf("failed to set target state: %v", err)
	}

	updated := prober.GetSnapshot("rev-test-2")
	if updated.GetBackends()[0].GetState() != apiv1.BackendState_BACKEND_STATE_DRAINING {
		t.Fatalf("expected backend draining, got %v", updated.GetBackends()[0].GetState())
	}
}
