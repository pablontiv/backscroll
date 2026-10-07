package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type syncTransactionFinalizer interface {
	Commit() error
	Rollback() error
}

type syncTransactionState uint8

const (
	syncTransactionActive syncTransactionState = iota
	syncTransactionRolledBack
	syncTransactionCommitChosen
	syncTransactionCommitted
	syncTransactionCommitFailed
)

// syncTransactionGate serializes context cancellation with the decision to
// commit. SQL statements still receive the caller's context, but the
// transaction itself is owned by a non-cancelable context so database/sql
// cannot race an automatic rollback against Commit.
type syncTransactionGate struct {
	ctx context.Context
	tx  syncTransactionFinalizer

	mu              sync.Mutex
	state           syncTransactionState
	rollbackErr     error
	canceled        bool
	callbackStopped bool
	callbackStarted chan struct{}
	callbackDone    chan struct{}
	stopCallback    func() bool
}

func newSyncTransactionGate(ctx context.Context, tx syncTransactionFinalizer) *syncTransactionGate {
	gate := &syncTransactionGate{
		ctx:             ctx,
		tx:              tx,
		state:           syncTransactionActive,
		callbackStarted: make(chan struct{}),
		callbackDone:    make(chan struct{}),
	}
	gate.stopCallback = context.AfterFunc(ctx, gate.cancel)
	return gate
}

func (g *syncTransactionGate) cancel() {
	close(g.callbackStarted)
	defer close(g.callbackDone)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.canceled = true
	if g.state == syncTransactionActive {
		g.state = syncTransactionRolledBack
		g.rollbackErr = g.tx.Rollback()
	}
}

func (g *syncTransactionGate) stopCancellationLocked() bool {
	if g.callbackStopped {
		return true
	}
	stopped := g.stopCallback()
	if stopped {
		g.callbackStopped = true
	}
	return stopped
}

// commit reaches its point-of-no-return only after the cancellation callback
// has been disabled and the context is still live while holding mu. A cancel
// that started first makes stopCallback return false and wins; a cancel that
// arrives after state becomes syncTransactionCommitChosen is deliberately late.
func (g *syncTransactionGate) commit() error {
	g.mu.Lock()
	if g.state != syncTransactionActive {
		stopped := g.stopCancellationLocked()
		g.mu.Unlock()
		if !stopped {
			<-g.callbackDone
		}
		return g.cancellationError()
	}

	stopped := g.stopCancellationLocked()
	ctxErr := g.ctx.Err()
	if !stopped || ctxErr != nil {
		g.canceled = true
		g.state = syncTransactionRolledBack
		g.rollbackErr = g.tx.Rollback()
		g.mu.Unlock()
		if !stopped {
			<-g.callbackDone
		}
		return g.cancellationError()
	}

	// Linearization point: cancellation can no longer start the rollback
	// callback, and the live context has been observed under the same lock.
	g.state = syncTransactionCommitChosen
	g.mu.Unlock()

	if err := g.tx.Commit(); err != nil {
		g.mu.Lock()
		g.state = syncTransactionCommitFailed
		g.mu.Unlock()
		return fmt.Errorf("commit transaction: %w", err)
	}

	g.mu.Lock()
	g.state = syncTransactionCommitted
	g.mu.Unlock()
	return nil
}

// rollback ends an uncommitted transaction and synchronizes with any callback
// already launched by context cancellation. It is safe to call from a defer
// after commit; the chosen/finished commit states are left untouched.
func (g *syncTransactionGate) rollback() error {
	g.mu.Lock()
	if g.state == syncTransactionCommitChosen || g.state == syncTransactionCommitted || g.state == syncTransactionCommitFailed {
		g.mu.Unlock()
		return nil
	}

	stopped := g.stopCancellationLocked()
	if g.ctx.Err() != nil {
		g.canceled = true
	}
	if g.state == syncTransactionActive {
		g.state = syncTransactionRolledBack
		g.rollbackErr = g.tx.Rollback()
	}
	g.mu.Unlock()

	if !stopped {
		<-g.callbackDone
	}
	g.mu.Lock()
	err := normalizeSyncRollbackError(g.rollbackErr, g.canceled)
	g.mu.Unlock()
	return err
}

func completedSyncRollbackError(err error) bool {
	if errors.Is(err, sql.ErrTxDone) {
		return true
	}
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) &&
		sqliteErr.Code() == sqlite3.SQLITE_ERROR &&
		sqliteErr.Error() == "SQL logic error: cannot rollback - no transaction is active (1)"
}

func normalizeSyncRollbackError(err error, canceled bool) error {
	if err == nil || (canceled && completedSyncRollbackError(err)) {
		return nil
	}
	return err
}

func (g *syncTransactionGate) cancellationError() error {
	g.mu.Lock()
	ctxErr := g.ctx.Err()
	rollbackErr := normalizeSyncRollbackError(g.rollbackErr, g.canceled)
	g.mu.Unlock()
	if ctxErr == nil {
		ctxErr = context.Canceled
	}
	if rollbackErr != nil {
		return errors.Join(ctxErr, fmt.Errorf("rollback transaction: %w", rollbackErr))
	}
	return ctxErr
}

// cancellationBeforeCommit reports cancellation only while cancellation still
// owns the outcome. A context canceled after commit was chosen is intentionally
// not retroactively reported as a failed operation.
func (g *syncTransactionGate) cancellationBeforeCommit() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.canceled || g.state == syncTransactionCommitChosen || g.state == syncTransactionCommitted || g.state == syncTransactionCommitFailed {
		return nil
	}
	return g.ctx.Err()
}
