package readers

import (
	"context"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
)

type mockReader struct {
	name  string
	epoch uint64
}

func (m *mockReader) Name() string           { return m.name }
func (m *mockReader) IngestionEpoch() uint64 { return m.epoch }
func (m *mockReader) Discover(context.Context, input_config.InputDefinition) ([]string, error) {
	return []string{"ref1", "ref2"}, nil
}
func (m *mockReader) Hash(_ context.Context, ref string) (string, error) { return "hash-" + ref, nil }
func (m *mockReader) Parse(_ context.Context, ref string, _ input_config.InputDefinition) (models.ParsedFile, error) {
	return models.ParsedFile{Path: ref}, nil
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockReader{name: "jsonl"})
	r.Register(&mockReader{name: "opencode"})

	sr, ok := r.Get("jsonl")
	if !ok || sr.Name() != "jsonl" {
		t.Errorf("Get jsonl: ok=%v name=%q", ok, sr.Name())
	}

	sr, ok = r.Get("unknown")
	if ok {
		t.Errorf("Get unknown: expected false, got %q", sr.Name())
	}
}

func TestRegistry_Default(t *testing.T) {
	r := NewRegistry()
	if r.Default() != nil {
		t.Error("Default() on empty registry should return nil")
	}

	r.Register(&mockReader{name: "claude"})
	if r.Default().Name() != "claude" {
		t.Error("Default() should return claude reader")
	}
}

func TestRegistry_ForDef(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockReader{name: "claude"})

	def := input_config.InputDefinition{Decode: input_config.DecodeConfig{Format: "claude"}}
	sr, err := r.ForDef(def)
	if err != nil || sr.Name() != "claude" {
		t.Errorf("ForDef claude: err=%v name=%q", err, sr.Name())
	}

	// Empty format falls back to claude
	def.Decode.Format = ""
	sr, err = r.ForDef(def)
	if err != nil || sr.Name() != "claude" {
		t.Errorf("ForDef empty: err=%v name=%q", err, sr.Name())
	}

	// Unknown format returns error
	def.Decode.Format = "unknown"
	_, err = r.ForDef(def)
	if err == nil {
		t.Error("ForDef unknown: expected error")
	}
}

func TestRegistry_DuplicatePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	r := NewRegistry()
	r.Register(&mockReader{name: "jsonl"})
	r.Register(&mockReader{name: "jsonl"}) // should panic
}

func TestMockReaderInterface(t *testing.T) {
	// Verify mockReader implements SessionReader
	var _ SessionReader = &mockReader{}

	m := &mockReader{name: "test"}
	refs, err := m.Discover(context.Background(), input_config.InputDefinition{})
	if err != nil || len(refs) != 2 {
		t.Errorf("Discover: err=%v refs=%v", err, refs)
	}

	hash, err := m.Hash(context.Background(), "ref1")
	if err != nil || hash != "hash-ref1" {
		t.Errorf("Hash: err=%v hash=%q", err, hash)
	}

	pf, err := m.Parse(context.Background(), "ref1", input_config.InputDefinition{})
	if err != nil || pf.Path != "ref1" {
		t.Errorf("Parse: err=%v pf=%v", err, pf)
	}
}

func TestIngestionFingerprintCoversSupportedSemantics(t *testing.T) {
	base := input_config.InputDefinition{
		Source: "session",
		Decode: input_config.DecodeConfig{Format: "pi"},
	}
	current := IngestionFingerprint(&mockReader{name: "pi", epoch: 1}, base)
	if current == "" || current[:3] != "v1:" {
		t.Fatalf("fingerprint = %q, want v1 prefix", current)
	}
	cases := []struct {
		name   string
		reader SessionReader
		def    input_config.InputDefinition
	}{
		{name: "decoder", reader: &mockReader{name: "omp", epoch: 1}, def: base},
		{name: "epoch", reader: &mockReader{name: "pi", epoch: 2}, def: base},
		{name: "source", reader: &mockReader{name: "pi", epoch: 1}, def: input_config.InputDefinition{Source: "decision", Decode: base.Decode}},
		{name: "reasoning", reader: &mockReader{name: "pi", epoch: 1}, def: input_config.InputDefinition{Source: "session", Decode: input_config.DecodeConfig{Format: "pi", IndexReasoning: true}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IngestionFingerprint(tc.reader, tc.def); got == current {
				t.Fatalf("%s transition retained fingerprint %q", tc.name, got)
			}
		})
	}
}
