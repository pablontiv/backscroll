package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pablontiv/backscroll/internal/models"
)

// CurrentOriginVersion identifies the parser-backed origin semantics persisted
// independently from the general extraction epoch.
const CurrentOriginVersion = 1

func normalizedOrigin(origin models.MessageOrigin) models.MessageOrigin {
	return models.PersistedMessageOrigin(origin)
}

// rejectConflictingOrigin protects a perennial identity from receiving two
// contradictory proven actors. Unknown is deliberately not treated as proof.
func rejectConflictingOrigin(ctx context.Context, tx *sql.Tx, uuid string, incoming models.MessageOrigin) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if uuid == "" || incoming == models.OriginUnknown {
		return nil
	}

	var stored models.MessageOrigin
	err := tx.QueryRowContext(ctx, `SELECT origin FROM search_items WHERE uuid = ?`, uuid).Scan(&stored)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read origin for uuid %s: %w", uuid, err)
	}
	if stored != models.OriginUnknown && stored != incoming {
		return fmt.Errorf("conflicting proven origins for uuid %s: %s != %s", uuid, stored, incoming)
	}
	return nil
}

// enrichOrigin advances unknown provenance when a parser later proves an
// actor. A partial parse may report unknown, but it can never erase proof that
// was already retained for the identity. No payload columns are modified.
func enrichOrigin(ctx context.Context, tx *sql.Tx, uuid string, incoming models.MessageOrigin) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if uuid == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE search_items
		SET origin = CASE WHEN origin = 'unknown' THEN ? ELSE origin END,
		    origin_version = ?
		WHERE uuid = ?
	`, incoming, CurrentOriginVersion, uuid)
	return err
}
