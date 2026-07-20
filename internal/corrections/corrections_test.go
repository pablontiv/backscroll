package corrections

import (
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
)

func testMsg(role, content string) models.Message {
	return models.Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}
}

func TestDetectorRegistry(t *testing.T) {
	// Test that registered detectors can be run
	result := RunDetectors([]models.Message{{Role: "user", Content: "test"}})
	if result == nil {
		result = make(map[int][]Detection)
	}
	// For a normal non-correction message, no detectors should fire
	if len(result) != 0 {
		t.Errorf("expected no detections for normal message, got %d", len(result))
	}
}

func TestCorrectionLexiconDetector(t *testing.T) {
	tests := []struct {
		name        string
		messages    []models.Message
		idx         int
		expectFire  bool
		expectWords []string
	}{
		{
			name: "Spanish: no, te pedí otra cosa",
			messages: []models.Message{
				testMsg("user", "Do X"),
				testMsg("assistant", "Doing X"),
				testMsg("user", "no, te pedí otra cosa"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "Spanish: eso no es",
			messages: []models.Message{
				testMsg("user", "Do X"),
				testMsg("assistant", "Doing X"),
				testMsg("user", "eso no es correcto"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "Spanish: otra vez",
			messages: []models.Message{
				testMsg("user", "Do X"),
				testMsg("assistant", "Doing X"),
				testMsg("user", "es todo. otra vez no"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "Spanish: te dije",
			messages: []models.Message{
				testMsg("user", "Do X"),
				testMsg("assistant", "Doing X"),
				testMsg("user", "pero te dije que no"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "Spanish: es mentira",
			messages: []models.Message{
				testMsg("user", "Do X"),
				testMsg("assistant", "Doing X"),
				testMsg("user", "eso es mentira"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "Spanish: no era eso",
			messages: []models.Message{
				testMsg("user", "Do X"),
				testMsg("assistant", "Doing X"),
				testMsg("user", "no era eso"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "Spanish: de nuevo",
			messages: []models.Message{
				testMsg("user", "Do X"),
				testMsg("assistant", "Doing X"),
				testMsg("user", "de nuevo mal"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "English: not what I asked",
			messages: []models.Message{
				testMsg("user", "Do X"),
				testMsg("assistant", "Doing X"),
				testMsg("user", "that's not what I asked for"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "English: I said",
			messages: []models.Message{
				testMsg("user", "Do X"),
				testMsg("assistant", "Doing X"),
				testMsg("user", "I said use Go, not Python"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "English: you ignored",
			messages: []models.Message{
				testMsg("user", "Do X"),
				testMsg("assistant", "Doing X"),
				testMsg("user", "you ignored the requirement"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "English: again (wrong file)",
			messages: []models.Message{
				testMsg("user", "Do X in file A"),
				testMsg("assistant", "Doing X"),
				testMsg("user", "again, wrong file"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "English: wrong",
			messages: []models.Message{
				testMsg("user", "Do X"),
				testMsg("assistant", "Doing X"),
				testMsg("user", "this is wrong"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "assistant message (should not fire)",
			messages: []models.Message{
				testMsg("user", "Do X"),
				testMsg("assistant", "no, te pedí otra cosa"),
			},
			idx:        1,
			expectFire: false,
		},
		{
			name: "normal user message (should not fire)",
			messages: []models.Message{
				testMsg("user", "ok, proceed"),
			},
			idx:        0,
			expectFire: false,
		},
		{
			name: "Spanish false positive: no, eso no es un bug, es esperado (known limitation)",
			messages: []models.Message{
				testMsg("user", "Why is this happening?"),
				testMsg("assistant", "Unknown cause"),
				testMsg("user", "no, eso no es un bug, es esperado"),
			},
			idx:        2,
			expectFire: true, // fires (known false positive — acceptable in v1)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			det := cCorrectionLexiconDetector(tt.messages, tt.idx)
			if tt.expectFire && det == nil {
				t.Errorf("expected detector to fire, got nil")
			}
			if !tt.expectFire && det != nil {
				t.Errorf("expected detector not to fire, got %+v", det)
			}
			if det != nil && det.Confidence != 0.8 {
				t.Errorf("expected confidence 0.8, got %f", det.Confidence)
			}
		})
	}
}

func TestInterruptDetector(t *testing.T) {
	tests := []struct {
		name       string
		messages   []models.Message
		idx        int
		expectFire bool
	}{
		{
			name: "user message with WasInterrupted=true",
			messages: []models.Message{
				testMsg("user", "Do X"),
				{Role: "user", Content: "Let me try a different approach", Timestamp: time.Now(), WasInterrupted: true},
			},
			idx:        1,
			expectFire: true,
		},
		{
			name: "user message with WasInterrupted=false",
			messages: []models.Message{
				testMsg("user", "Do X"),
				{Role: "user", Content: "Do Y", Timestamp: time.Now(), WasInterrupted: false},
			},
			idx:        1,
			expectFire: false,
		},
		{
			name: "assistant message with WasInterrupted=true (should not fire)",
			messages: []models.Message{
				testMsg("user", "Do X"),
				{Role: "assistant", Content: "Done", Timestamp: time.Now(), WasInterrupted: true},
			},
			idx:        1,
			expectFire: false,
		},
		{
			name: "user message WasInterrupted=true with stub text < 20 chars (should not fire)",
			messages: []models.Message{
				testMsg("user", "Do X"),
				{Role: "user", Content: "[ by user]", Timestamp: time.Now(), WasInterrupted: true},
			},
			idx:        1,
			expectFire: false,
		},
		{
			name: "user message WasInterrupted=true with substantive text >= 20 chars (should fire)",
			messages: []models.Message{
				testMsg("user", "Do X"),
				{Role: "user", Content: "Let me try a different approach here now", Timestamp: time.Now(), WasInterrupted: true},
			},
			idx:        1,
			expectFire: true,
		},
		{
			name: "user message WasInterrupted=true with exactly 20 chars (should fire)",
			messages: []models.Message{
				testMsg("user", "Do X"),
				{Role: "user", Content: "12345678901234567890", Timestamp: time.Now(), WasInterrupted: true},
			},
			idx:        1,
			expectFire: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			det := cInterruptDetector(tt.messages, tt.idx)
			if tt.expectFire && det == nil {
				t.Errorf("expected detector to fire, got nil")
			}
			if !tt.expectFire && det != nil {
				t.Errorf("expected detector not to fire, got %+v", det)
			}
			if det != nil && det.Confidence != 0.5 {
				t.Errorf("expected confidence 0.5, got %f", det.Confidence)
			}
		})
	}
}

func TestDenialDetector(t *testing.T) {
	tests := []struct {
		name       string
		messages   []models.Message
		idx        int
		expectFire bool
	}{
		{
			name: "user message after assistant denial",
			messages: []models.Message{
				testMsg("user", "Run this command"),
				testMsg("assistant", "This is denied for security reasons"),
				testMsg("user", "ok, try something else"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "user message after Spanish denial",
			messages: []models.Message{
				testMsg("user", "Execute X"),
				testMsg("assistant", "Operación rechazada por política"),
				testMsg("user", "hmm, understood"),
			},
			idx:        2,
			expectFire: true,
		},
		{
			name: "user message with no preceding denial",
			messages: []models.Message{
				testMsg("user", "Do this"),
				testMsg("assistant", "Done!"),
				testMsg("user", "Great, next task"),
			},
			idx:        2,
			expectFire: false,
		},
		{
			name: "first user message (no previous message)",
			messages: []models.Message{
				testMsg("user", "Start here"),
			},
			idx:        0,
			expectFire: false,
		},
		{
			name: "assistant message after denial (should not fire)",
			messages: []models.Message{
				testMsg("user", "Do this"),
				testMsg("assistant", "denied"),
				testMsg("assistant", "Here's more info"),
			},
			idx:        2,
			expectFire: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			det := cDenialDetector(tt.messages, tt.idx)
			if tt.expectFire && det == nil {
				t.Errorf("expected detector to fire, got nil")
			}
			if !tt.expectFire && det != nil {
				t.Errorf("expected detector not to fire, got %+v", det)
			}
			if det != nil && det.Confidence != 0.4 {
				t.Errorf("expected confidence 0.4, got %f", det.Confidence)
			}
		})
	}
}

func TestRephraseDetector(t *testing.T) {
	tests := []struct {
		name       string
		messages   []models.Message
		idx        int
		expectFire bool
	}{
		{
			name: "high overlap: rephrase (should fire)",
			messages: []models.Message{
				testMsg("user", "read the file please"),
				testMsg("assistant", "I'll read it"),
				testMsg("user", "read the file"),
			},
			idx:        2,
			expectFire: true, // Jaccard("read file please", "read file") ≈ 0.67 >= 0.6
		},
		{
			name: "direct rephrase with small wording change",
			messages: []models.Message{
				testMsg("user", "compile the go program"),
				testMsg("assistant", "compiling..."),
				testMsg("user", "compile go program"),
			},
			idx:        2,
			expectFire: true, // high overlap
		},
		{
			name: "low overlap: different intent (should not fire)",
			messages: []models.Message{
				testMsg("user", "read file"),
				testMsg("assistant", "ok"),
				testMsg("user", "write file"),
			},
			idx:        2,
			expectFire: false, // overlap too low
		},
		{
			name: "first user message (no previous)",
			messages: []models.Message{
				testMsg("user", "first request"),
			},
			idx:        0,
			expectFire: false,
		},
		{
			name: "user after non-user (should not fire)",
			messages: []models.Message{
				testMsg("assistant", "some response"),
				testMsg("user", "some response"),
			},
			idx:        1,
			expectFire: false, // previous is not user
		},
		{
			name: "assistant message (should not fire)",
			messages: []models.Message{
				testMsg("user", "do x"),
				testMsg("assistant", "do x"),
			},
			idx:        1,
			expectFire: false, // current is not user
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			det := cRephraseDetector(tt.messages, tt.idx)
			if tt.expectFire && det == nil {
				t.Errorf("expected detector to fire, got nil")
			}
			if !tt.expectFire && det != nil {
				t.Errorf("expected detector not to fire, got %+v", det)
			}
			if det != nil && det.Confidence != 0.6 {
				t.Errorf("expected confidence 0.6, got %f", det.Confidence)
			}
		})
	}
}
