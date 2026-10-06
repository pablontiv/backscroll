package models

// MessageOrigin is parser-backed provenance for one indexed message.
type MessageOrigin string

const (
	OriginHuman      MessageOrigin = "human"
	OriginAssistant  MessageOrigin = "assistant"
	OriginSystem     MessageOrigin = "system"
	OriginAutomation MessageOrigin = "automation"
	OriginUnknown    MessageOrigin = "unknown"
)

// ValidMessageOrigin reports whether origin belongs to the persisted domain.
func ValidMessageOrigin(origin MessageOrigin) bool {
	switch origin {
	case OriginHuman, OriginAssistant, OriginSystem, OriginAutomation, OriginUnknown:
		return true
	default:
		return false
	}
}

// PersistedMessageOrigin converts an absent or invalid internal value to the
// safe persisted fallback. Readers should set a proven value directly.
func PersistedMessageOrigin(origin MessageOrigin) MessageOrigin {
	if ValidMessageOrigin(origin) {
		return origin
	}
	return OriginUnknown
}
