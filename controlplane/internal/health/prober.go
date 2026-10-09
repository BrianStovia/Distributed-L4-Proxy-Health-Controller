package health

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/arcgg/controlplane/internal/metrics"
	apiv1 "github.com/arcgg/controlplane/pkg/api/v1"
)

// Target represents a monitored backend server.
type Target struct {
	ID      string
	Address string
	Weight  uint32
	State   apiv1.BackendState
}

// Prober manages active health probes against backend targets.
type Prober struct {
	mu           sync.RWMutex
	targets      map[string]*Target
	probeTimeout time.Duration
	interval     time.Duration
	onChange     chan struct{}
}

// NewProber initializes a Prober instance.
func NewProber(initialTargets []*Target, interval, timeout time.Duration) *Prober {
	targetMap := make(map[string]*Target)
	for _, t := range initialTargets {
		targetMap[t.ID] = t
	}

	return &Prober{
		targets:      targetMap,
		probeTimeout: timeout,
		interval:     interval,
		onChange:     make(chan struct{}, 16),
	}
}

// SubscribeStateChanges returns a channel that signals whenever any backend changes state.
func (p *Prober) SubscribeStateChanges() <-chan struct{} {
	return p.onChange
}

// GetSnapshot returns the current cluster configuration snapshot.
func (p *Prober) GetSnapshot(version string) *apiv1.ClusterConfigSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()

	backends := make([]*apiv1.BackendTarget, 0, len(p.targets))
	for _, t := range p.targets {
		backends = append(backends, &apiv1.BackendTarget{
			Id:      t.ID,
			Address: t.Address,
			Weight:  t.Weight,
			State:   t.State,
		})
	}

	return &apiv1.ClusterConfigSnapshot{
		Version:              version,
		TimestampUnixEpochMs: time.Now().UnixMilli(),
		Algorithm:            apiv1.BalancingAlgorithm_BALANCING_ALGORITHM_ROUND_ROBIN,
		Backends:             backends,
	}
}

// Start runs active probing in a continuous background loop.
func (p *Prober) Start(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Perform initial probe right away
	p.probeAll()

	for {
		select {
		case <-ctx.Done():
			log.Println("[Prober] Health check worker stopping...")
			return
		case <-ticker.C:
			p.probeAll()
		}
	}
}

func (p *Prober) probeAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	stateChanged := false

	for id, target := range p.targets {
		isReachable := p.checkTCP(target.Address)
		newState := apiv1.BackendState_BACKEND_STATE_HEALTHY
		if !isReachable {
			newState = apiv1.BackendState_BACKEND_STATE_UNHEALTHY
		}

		metrics.RecordBackendHealth(target.ID, target.Address, isReachable)

		if target.State != newState {
			log.Printf("[Prober] Backend state transition: id=%s addr=%s %s -> %s\n",
				id, target.Address, target.State.String(), newState.String())
			target.State = newState
			stateChanged = true
		}
	}

	if stateChanged {
		select {
		case p.onChange <- struct{}{}:
		default:
			// Channel buffer already has pending notification
		}
	}
}

func (p *Prober) checkTCP(address string) bool {
	conn, err := net.DialTimeout("tcp", address, p.probeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// SetTargetState manually adjusts a backend state (useful for maintenance / draining).
func (p *Prober) SetTargetState(id string, state apiv1.BackendState) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	target, exists := p.targets[id]
	if !exists {
		return fmt.Errorf("target %s not found", id)
	}

	if target.State != state {
		target.State = state
		select {
		case p.onChange <- struct{}{}:
		default:
		}
	}
	return nil
}
