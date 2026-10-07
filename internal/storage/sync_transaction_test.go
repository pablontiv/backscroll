package storage

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
)

type fakeSyncTransaction struct {
	mu          sync.Mutex
	commits     int
	rollbacks   int
	commitErr   error
	rollbackErr error
}

func (f *fakeSyncTransaction) Commit() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commits++
	return f.commitErr
}

func (f *fakeSyncTransaction) Rollback() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rollbacks++
	return f.rollbackErr
}

func (f *fakeSyncTransaction) counts() (commits, rollbacks int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commits, f.rollbacks
}

func TestSyncTransactionGateCancellationWinsCommitGate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tx := &fakeSyncTransaction{}
	gate := newSyncTransactionGate(ctx, tx)

	// Hold the gate lock until the cancellation callback has started. This
	// deterministically places cancellation at the commit boundary without a
	// timing hook in production code.
	gate.mu.Lock()
	cancel()
	<-gate.callbackStarted
	gate.mu.Unlock()

	err := gate.commit()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("commit error = %v, want context.Canceled", err)
	}
	if commits, rollbacks := tx.counts(); commits != 0 || rollbacks != 1 {
		t.Fatalf("finalizer calls = commits %d, rollbacks %d; want 0, 1", commits, rollbacks)
	}
	select {
	case <-gate.callbackDone:
		// commit waits for a started callback, so no callback goroutine remains.
	default:
		t.Fatal("cancellation callback was not complete when commit returned")
	}
	if err := gate.rollback(); err != nil {
		t.Fatalf("deferred rollback after cancellation: %v", err)
	}
	if commits, rollbacks := tx.counts(); commits != 0 || rollbacks != 1 {
		t.Fatalf("deferred finalizer calls = commits %d, rollbacks %d; want 0, 1", commits, rollbacks)
	}
}

func TestSyncTransactionGateCommitWinsLateCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tx := &fakeSyncTransaction{}
	gate := newSyncTransactionGate(ctx, tx)

	if err := gate.commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	cancel()

	if commits, rollbacks := tx.counts(); commits != 1 || rollbacks != 0 {
		t.Fatalf("finalizer calls = commits %d, rollbacks %d; want 1, 0", commits, rollbacks)
	}
	select {
	case <-gate.callbackStarted:
		t.Fatal("late cancellation started a disabled rollback callback")
	default:
	}
	gate.mu.Lock()
	stopped := gate.callbackStopped
	state := gate.state
	gate.mu.Unlock()
	if !stopped || state != syncTransactionCommitted {
		t.Fatalf("gate state = %v, callbackStopped = %v; want committed and stopped", state, stopped)
	}
	if err := gate.rollback(); err != nil {
		t.Fatalf("deferred rollback after commit: %v", err)
	}
}

func TestSyncTransactionGateCancellationKeepsStableErrors(t *testing.T) {
	rollbackFailure := errors.New("rollback failed")
	ctx, cancel := context.WithCancel(context.Background())
	tx := &fakeSyncTransaction{rollbackErr: rollbackFailure}
	gate := newSyncTransactionGate(ctx, tx)
	cancel()
	<-gate.callbackStarted

	err := gate.commit()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("commit error = %v, want context.Canceled identity", err)
	}
	if !errors.Is(err, rollbackFailure) {
		t.Fatalf("commit error = %v, want rollback failure identity", err)
	}
}

func TestSyncTransactionGateRollbackNormalizesTxDone(t *testing.T) {
	tx := &fakeSyncTransaction{rollbackErr: sql.ErrTxDone}
	gate := newSyncTransactionGate(context.Background(), tx)

	if err := gate.rollback(); err != nil {
		t.Fatalf("rollback error = %v, want nil", err)
	}
	if commits, rollbacks := tx.counts(); commits != 0 || rollbacks != 1 {
		t.Fatalf("finalizer calls = commits %d, rollbacks %d; want 0, 1", commits, rollbacks)
	}
}

func TestSyncTransactionGateRollbackPreservesDistinctError(t *testing.T) {
	rollbackFailure := errors.New("distinct rollback failure")
	tx := &fakeSyncTransaction{rollbackErr: rollbackFailure}
	gate := newSyncTransactionGate(context.Background(), tx)

	err := gate.rollback()
	if !errors.Is(err, rollbackFailure) {
		t.Fatalf("rollback error = %v, want distinct rollback failure", err)
	}
}

func TestSyncTransactionGateCommitErrorIdentity(t *testing.T) {
	commitFailure := errors.New("commit failed")
	tx := &fakeSyncTransaction{commitErr: commitFailure}
	gate := newSyncTransactionGate(context.Background(), tx)

	err := gate.commit()
	if !errors.Is(err, commitFailure) {
		t.Fatalf("commit error = %v, want commit failure identity", err)
	}
	if commits, rollbacks := tx.counts(); commits != 1 || rollbacks != 0 {
		t.Fatalf("finalizer calls = commits %d, rollbacks %d; want 1, 0", commits, rollbacks)
	}
	if err := gate.rollback(); err != nil {
		t.Fatalf("deferred rollback after failed commit: %v", err)
	}
}
