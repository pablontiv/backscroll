package embedding

import (
	"context"
	"testing"
)

func TestMockProvider_ImplementsInterface(t *testing.T) {
	var _ EmbeddingProvider = &MockEmbeddingProvider{}
}

func TestMockProvider_Dimensions(t *testing.T) {
	m := NewMockProvider(384)
	if m.Dimensions() != 384 {
		t.Errorf("Dimensions() = %d, want 384", m.Dimensions())
	}
}

func TestMockProvider_Embed_Length(t *testing.T) {
	m := NewMockProvider(384)
	vec, err := m.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 384 {
		t.Errorf("Embed len = %d, want 384", len(vec))
	}
}

func TestMockProvider_Embed_Deterministic(t *testing.T) {
	m := NewMockProvider(384)
	v1, _ := m.Embed(context.Background(), "hello")
	v2, _ := m.Embed(context.Background(), "hello")
	if len(v1) != len(v2) {
		t.Fatal("lengths differ")
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Errorf("v1[%d]=%v != v2[%d]=%v (not deterministic)", i, v1[i], i, v2[i])
		}
	}
}

func TestMockProvider_Embed_Distinct(t *testing.T) {
	m := NewMockProvider(384)
	vh, _ := m.Embed(context.Background(), "hello")
	vw, _ := m.Embed(context.Background(), "world")

	same := true
	for i := range vh {
		if vh[i] != vw[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("Embed('hello') == Embed('world'), expected distinct vectors")
	}
}

func TestMockProvider_Embed_ZeroDims(t *testing.T) {
	m := NewMockProvider(0)
	vec, err := m.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 0 {
		t.Errorf("Embed len = %d, want 0", len(vec))
	}
}

func TestMockProvider_Close(t *testing.T) {
	m := NewMockProvider(384)
	if err := m.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestMockProvider_Embed_Range(t *testing.T) {
	m := NewMockProvider(8)
	vec, _ := m.Embed(context.Background(), "test")
	for i, v := range vec {
		if v < -1.0 || v > 1.0 {
			t.Errorf("vec[%d] = %f, want in [-1, 1]", i, v)
		}
	}
}
