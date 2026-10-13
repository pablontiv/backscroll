package main

import (
	"context"
	"fmt"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
)

// inputSnapshot is invocation-local and never exposes its retained slices.
// Readers and output receive copies, not permission to mutate the snapshot.
type inputSnapshot struct {
	definitions []input_config.InputDefinition
	mode        input_config.InputMode
}
type inputSnapshotKey struct{}

func cloneInputDefinitions(defs []input_config.InputDefinition) []input_config.InputDefinition {
	cloned := append([]input_config.InputDefinition(nil), defs...)
	for i := range cloned {
		cloned[i].Discover.Roots = append([]string(nil), defs[i].Discover.Roots...)
		cloned[i].Discover.Include = append([]string(nil), defs[i].Discover.Include...)
		cloned[i].Discover.Exclude = append([]string(nil), defs[i].Discover.Exclude...)
	}
	return cloned
}

func (s *startupSyncService) resolveInputSnapshot(cfg *config.Config) (*inputSnapshot, error) {
	defs, mode, err := s.activeInputs(cfg.SessionDirs)
	if err != nil {
		return nil, err
	}
	registry := s.newRegistry()
	for _, def := range defs {
		if _, err := registry.ForDef(def); err != nil {
			return nil, fmt.Errorf("%s decode.format: %w; use a registered reader name", def.Location(), err)
		}
	}
	return &inputSnapshot{definitions: cloneInputDefinitions(defs), mode: mode}, nil
}

func inputSnapshotFrom(ctx context.Context) *inputSnapshot {
	snapshot, _ := ctx.Value(inputSnapshotKey{}).(*inputSnapshot)
	return snapshot
}
