package storage

import (
	"context"
	"database/sql"
	"fmt"
	"iter"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/tagging"
)

// A UUID has one canonical coordinate, even when native history is copied into
// another transcript. Retained coordinates win; new owners use lexical paths.
// The message is a value copy so merging proof never changes a caller's slice.
type syncUUIDOwner struct {
	path, source string
	message      IndexedMessage
	retained     bool
	crossPath    bool
	applied      bool
	incomingTool bool
	next         *syncUUIDOwner // absent-owner path batch; reuses the canonical event
}

type syncUUIDBatch struct {
	byUUID map[string]*syncUUIDOwner
	owners []*syncUUIDOwner
}

func reconcileSyncUUIDs(ctx context.Context, tx *sql.Tx, files []IndexedFile) (*syncUUIDBatch, error) {
	batch := &syncUUIDBatch{byUUID: make(map[string]*syncUUIDOwner)}
	type occurrence struct{ path, uuid string }
	seen := make(map[occurrence]struct{})
	type sessionKey struct{ path, session string }
	nativeProjects := make(map[sessionKey]string)
	for _, file := range files {
		for _, message := range file.Messages {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if message.SessionID != "" && message.ProjectEvidence == "native_cwd" {
				key := sessionKey{file.SourcePath, message.SessionID}
				if project, ok := nativeProjects[key]; ok {
					if project != message.Project {
						return nil, fmt.Errorf("session %q contradicts incoming native project ownership", message.SessionID)
					}
				} else {
					if err := checkNativeProject(ctx, tx, file.SourcePath, message); err != nil {
						return nil, err
					}
					nativeProjects[key] = message.Project
				}
			}
			if message.UUID == "" {
				continue
			}
			if message.Ordinal < 0 {
				return nil, fmt.Errorf("negative incoming ordinal %d for UUID %q", message.Ordinal, message.UUID)
			}
			key := occurrence{file.SourcePath, message.UUID}
			if _, duplicate := seen[key]; duplicate {
				return nil, fmt.Errorf("duplicate UUID %q in source_path %q", message.UUID, file.SourcePath)
			}
			seen[key] = struct{}{}
			owner := batch.byUUID[message.UUID]
			if owner == nil {
				owner = &syncUUIDOwner{path: file.SourcePath, source: file.Source, message: effectiveSyncMessage(file, message)}
				batch.byUUID[message.UUID] = owner
				batch.owners = append(batch.owners, owner)
			} else {
				owner.crossPath = true
				if file.SourcePath < owner.path {
					owner.path, owner.source, owner.message = file.SourcePath, file.Source, effectiveSyncMessage(file, message)
				}
			}
			owner.incomingTool = owner.incomingTool || message.ToolName != ""
		}
	}

	for _, owner := range batch.owners {
		if err := loadSyncUUIDOwner(ctx, tx, owner); err != nil {
			return nil, err
		}
	}
	for _, file := range files {
		for _, message := range file.Messages {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if message.UUID == "" {
				continue
			}
			owner := batch.byUUID[message.UUID]
			incoming := effectiveSyncMessage(file, message)
			if owner.crossPath {
				if owner.source != file.Source || !sameSyncPayload(owner.message, incoming) {
					return nil, fmt.Errorf("UUID %q contradicts canonical payload at source_path %q", message.UUID, owner.path)
				}
				if err := mergeSyncProof(&owner.message, incoming); err != nil {
					return nil, fmt.Errorf("UUID %q contradicts canonical proof at source_path %q: %w", message.UUID, owner.path, err)
				}
			} else if owner.retained {
				// Same-source re-extraction keeps its established parser-drift
				// contract. Cross-path admission above requires matching payload.
				if owner.message.SessionID != "" && incoming.SessionID != "" && owner.message.SessionID != incoming.SessionID {
					return nil, fmt.Errorf("UUID %q contradicts retained session ownership", message.UUID)
				}
				if incoming.SessionID != "" && incoming.ProjectEvidence == "native_cwd" && owner.message.ProjectEvidence == "native_cwd" && incoming.Project != owner.message.Project {
					return nil, fmt.Errorf("UUID %q contradicts retained native project ownership", message.UUID)
				}
				owner.message = incoming
			}
		}
	}
	for _, owner := range batch.owners {
		message := owner.message
		if message.SessionID == "" || message.ProjectEvidence != "native_cwd" {
			continue
		}
		key := sessionKey{owner.path, message.SessionID}
		if project, ok := nativeProjects[key]; ok {
			if project != message.Project {
				return nil, fmt.Errorf("session %q contradicts canonical native project ownership", message.SessionID)
			}
		} else {
			if err := checkNativeProject(ctx, tx, owner.path, message); err != nil {
				return nil, err
			}
			nativeProjects[key] = message.Project
		}
	}
	return batch, nil
}

func effectiveSyncMessage(file IndexedFile, message IndexedMessage) IndexedMessage {
	if message.SessionID == "" {
		message.Project, message.ProjectEvidence = file.Project, file.ProjectEvidence
	}
	return message
}

func checkNativeProject(ctx context.Context, tx *sql.Tx, path string, message IndexedMessage) error {
	if message.SessionID == "" || message.ProjectEvidence != "native_cwd" {
		return nil
	}
	var conflict bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM search_items WHERE source_path=? AND session_id=? AND project_evidence='native_cwd' AND project IS NOT ?)`, path, message.SessionID, message.Project).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return fmt.Errorf("session %q contradicts retained native project ownership", message.SessionID)
	}
	return nil
}

func loadSyncUUIDOwner(ctx context.Context, tx *sql.Tx, owner *syncUUIDOwner) error {
	var stored IndexedMessage
	stored.UUID = owner.message.UUID
	var path, source string
	err := tx.QueryRowContext(ctx, `SELECT source_path,source,ordinal,COALESCE(session_id,''),COALESCE(project,''),COALESCE(project_evidence,'unknown') FROM search_items WHERE uuid=?`, stored.UUID).Scan(
		&path, &source, &stored.Ordinal, &stored.SessionID, &stored.Project, &stored.ProjectEvidence)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("inspect UUID %q: %w", stored.UUID, err)
	}
	if err == nil {
		if stored.Ordinal < 0 {
			return fmt.Errorf("negative persisted ordinal %d for UUID %q", stored.Ordinal, stored.UUID)
		}
		owner.crossPath = owner.crossPath || path != owner.path
		if owner.crossPath {
			if err := tx.QueryRowContext(ctx, `SELECT role,origin,text,COALESCE(timestamp,''),content_type,COALESCE(extraction_version,0),COALESCE(search_echo,0),COALESCE(was_interrupted,0) FROM search_items WHERE uuid=?`, stored.UUID).Scan(
				&stored.Role, &stored.Origin, &stored.Text, &stored.Timestamp, &stored.ContentType, &stored.ExtractionVersion, &stored.SearchEcho, &stored.WasInterrupted); err != nil {
				return err
			}
		}
		owner.path, owner.source, owner.message, owner.retained = path, source, stored, true
	}
	var eventPath, tool, head string
	var ordinal int
	var native sql.NullString
	var isError sql.NullBool
	var exit sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT source_path,ordinal,tool_name,COALESCE(command_head,''),native_call_id,is_error,exit_code FROM tool_events WHERE message_uuid=?`, stored.UUID).Scan(&eventPath, &ordinal, &tool, &head, &native, &isError, &exit)
	if err == sql.ErrNoRows && owner.retained && owner.crossPath && owner.incomingTool {
		err = tx.QueryRowContext(ctx, `SELECT source_path,ordinal,tool_name,COALESCE(command_head,''),native_call_id,is_error,exit_code FROM tool_events WHERE source_path=? AND ordinal=? AND NULLIF(message_uuid,'') IS NULL`, owner.path, owner.message.Ordinal).Scan(&eventPath, &ordinal, &tool, &head, &native, &isError, &exit)
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		if eventPath != owner.path || (owner.retained && ordinal != owner.message.Ordinal) {
			return fmt.Errorf("UUID %q has conflicting tool_event owner or ordinal", stored.UUID)
		}
		if owner.crossPath {
			owner.message.ToolName, owner.message.CommandHead, owner.message.ToolUseID = tool, head, native.String
			if isError.Valid {
				owner.message.IsError = &isError.Bool
			}
			if exit.Valid {
				value := int(exit.Int64)
				owner.message.ExitCode = &value
			}
		}
	}
	return nil
}

func sameSyncPayload(a, b IndexedMessage) bool {
	if a.Role != b.Role || a.Text != b.Text || a.ContentType != b.ContentType {
		return false
	}
	if a.Timestamp == "" || b.Timestamp == "" || a.Timestamp == b.Timestamp {
		return true
	}
	at, ae := time.Parse(time.RFC3339Nano, a.Timestamp)
	bt, be := time.Parse(time.RFC3339Nano, b.Timestamp)
	return ae == nil && be == nil && at.Equal(bt)
}

func mergeSyncProof(a *IndexedMessage, b IndexedMessage) error {
	mergeIdentity := func(name string, dst *string, incoming string) error {
		if *dst != "" && incoming != "" && *dst != incoming {
			return fmt.Errorf("conflicting %s", name)
		}
		if *dst == "" {
			*dst = incoming
		}
		return nil
	}
	for _, field := range []struct {
		name  string
		dst   *string
		value string
	}{{"session identity", &a.SessionID, b.SessionID}, {"tool name", &a.ToolName, b.ToolName}, {"command head", &a.CommandHead, b.CommandHead}, {"native call identity", &a.ToolUseID, b.ToolUseID}} {
		if err := mergeIdentity(field.name, field.dst, field.value); err != nil {
			return err
		}
	}
	aOrigin, bOrigin := normalizedOrigin(a.Origin), normalizedOrigin(b.Origin)
	if aOrigin != models.OriginUnknown && bOrigin != models.OriginUnknown && aOrigin != bOrigin {
		return fmt.Errorf("conflicting proven origins")
	}
	if aOrigin == models.OriginUnknown {
		a.Origin = bOrigin
	}
	provedProject := func(e string) bool { return e == "registry" || e == "native_cwd" }
	if provedProject(a.ProjectEvidence) && provedProject(b.ProjectEvidence) && a.Project != b.Project {
		return fmt.Errorf("conflicting proven projects")
	}
	if !provedProject(a.ProjectEvidence) && (provedProject(b.ProjectEvidence) || projectEvidence(a.ProjectEvidence) == "unknown") {
		a.Project, a.ProjectEvidence = b.Project, b.ProjectEvidence
	}
	if a.IsError != nil && b.IsError != nil && *a.IsError != *b.IsError {
		return fmt.Errorf("conflicting tool error proof")
	}
	if a.ExitCode != nil && b.ExitCode != nil && *a.ExitCode != *b.ExitCode {
		return fmt.Errorf("conflicting tool exit proof")
	}
	if a.IsError == nil {
		a.IsError = b.IsError
	}
	if a.ExitCode == nil {
		a.ExitCode = b.ExitCode
	}
	a.SearchEcho = a.SearchEcho || b.SearchEcho
	a.WasInterrupted = a.WasInterrupted || b.WasInterrupted
	if a.Timestamp == "" {
		a.Timestamp = b.Timestamp
	}
	return nil
}

func (b *syncUUIDBatch) messages(file IndexedFile) ([]IndexedMessage, bool) {
	// Reuse the incoming slice when reconciliation has nothing to change.
	changed := false
	kept := 0
	for _, message := range file.Messages {
		owner := b.byUUID[message.UUID]
		if owner != nil && owner.crossPath {
			changed = true
		}
		if owner == nil || owner.path == file.SourcePath {
			kept++
		}
	}
	if !changed {
		for _, message := range file.Messages {
			if owner := b.byUUID[message.UUID]; owner != nil {
				owner.applied = true
			}
		}
		return file.Messages, false
	}
	messages := make([]IndexedMessage, 0, kept)
	for _, message := range file.Messages {
		owner := b.byUUID[message.UUID]
		if owner != nil {
			if owner.path != file.SourcePath {
				continue
			}
			message = owner.message
			owner.applied = true
		}
		messages = append(messages, message)
	}
	return messages, true
}

func canonicalSyncTags(messages []IndexedMessage) []string {
	a := tagging.Accumulator{}
	for _, message := range messages {
		if message.SessionID != "" {
			continue
		}
		a.Add(message.Text)
	}
	return a.Tags()
}

// indexedSyncMessages exposes messages by reference for shared derived work.
func indexedSyncMessages(messages []IndexedMessage) iter.Seq2[int, *IndexedMessage] {
	return func(yield func(int, *IndexedMessage) bool) {
		for i := range messages {
			if !yield(i, &messages[i]) {
				return
			}
		}
	}
}

func copiedSyncMessages(head *syncUUIDOwner) iter.Seq2[int, *IndexedMessage] {
	return func(yield func(int, *IndexedMessage) bool) {
		i := 0
		for owner := head; owner != nil; owner = owner.next {
			if !yield(i, &owner.message) {
				return
			}
			i++
		}
	}
}
