package compat

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type sqlTokenKind byte

const (
	sqlBare sqlTokenKind = iota
	sqlLiteral
	sqlQuotedIdentifier
	sqlPunctuation
)

type sqlToken struct {
	kind sqlTokenKind
	text string
}

func canonicalSQL(input string) string {
	tokens := scanSQLTokens(input)
	var encoded strings.Builder
	for _, token := range tokens {
		encoded.WriteByte(byte('0' + token.kind))
		encoded.WriteByte(':')
		encoded.WriteString(strconv.Itoa(len(token.text)))
		encoded.WriteByte(':')
		encoded.WriteString(token.text)
	}
	return encoded.String()
}

func scanSQLTokens(input string) []sqlToken {
	tokens := make([]sqlToken, 0, len(input)/2)
	for i := 0; i < len(input); {
		r, size := utf8.DecodeRuneInString(input[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		if i+1 < len(input) && input[i] == '-' && input[i+1] == '-' {
			i += 2
			for i < len(input) && input[i] != '\n' && input[i] != '\r' {
				i++
			}
			continue
		}
		if i+1 < len(input) && input[i] == '/' && input[i+1] == '*' {
			_, next := scanBlockComment(input, i)
			i = next
			continue
		}
		if input[i] == '\'' {
			text, next := scanSingleQuoted(input, i)
			tokens = append(tokens, sqlToken{kind: sqlLiteral, text: text})
			i = next
			continue
		}
		if input[i] == '"' || input[i] == '`' || input[i] == '[' {
			close := byte('"')
			doubled := true
			if input[i] == '`' {
				close = '`'
				doubled = false
			} else if input[i] == '[' {
				close = ']'
				doubled = false
			}
			text, next := scanDelimitedIdentifier(input, i, input[i], close, doubled)
			tokens = append(tokens, sqlToken{kind: sqlQuotedIdentifier, text: text})
			i = next
			continue
		}
		if op, ok := longestSQLOperator(input, i); ok {
			tokens = append(tokens, sqlToken{kind: sqlPunctuation, text: op})
			i += len(op)
			continue
		}
		start := i
		for i < len(input) {
			r, size = utf8.DecodeRuneInString(input[i:])
			if unicode.IsSpace(r) || isSQLPunctuation(input[i]) {
				break
			}
			if input[i] == '\'' || input[i] == '"' || input[i] == '`' || input[i] == '[' {
				break
			}
			if i+1 < len(input) && input[i] == '-' && input[i+1] == '-' {
				break
			}
			if i+1 < len(input) && input[i] == '/' && input[i+1] == '*' {
				break
			}
			i += size
		}
		tokens = append(tokens, sqlToken{kind: sqlBare, text: strings.ToLower(input[start:i])})
	}
	return tokens
}

func scanSingleQuoted(input string, start int) (string, int) {
	var b strings.Builder
	if start < len(input) {
		b.WriteByte(input[start])
	}
	i := start + 1
	for i < len(input) {
		b.WriteByte(input[i])
		if input[i] == '\'' {
			if i+1 < len(input) && input[i+1] == '\'' {
				b.WriteByte(input[i+1])
				i += 2
				continue
			}
			i++
			return b.String(), i
		}
		i++
	}
	return b.String(), i
}

func scanDelimitedIdentifier(input string, start int, _ byte, close byte, doubledClose bool) (string, int) {
	var b strings.Builder
	if start < len(input) {
		b.WriteByte(input[start])
	}
	i := start + 1
	for i < len(input) {
		b.WriteByte(input[i])
		if input[i] == close {
			if doubledClose && i+1 < len(input) && input[i+1] == close {
				b.WriteByte(input[i+1])
				i += 2
				continue
			}
			i++
			return b.String(), i
		}
		i++
	}
	return b.String(), i
}

func longestSQLOperator(input string, offset int) (string, bool) {
	for _, op := range []string{"->>", "||", "->", "<<", ">>", "<=", ">=", "==", "!=", "<>"} {
		if strings.HasPrefix(input[offset:], op) {
			return op, true
		}
	}
	if offset < len(input) && isSQLPunctuation(input[offset]) {
		return input[offset : offset+1], true
	}
	return "", false
}

func isSQLPunctuation(ch byte) bool {
	switch ch {
	case '(', ')', ',', ';', '.', '+', '-', '*', '/', '%', '<', '>', '=', '&', '|', '~':
		return true
	default:
		return false
	}
}

func scanBlockComment(input string, start int) (string, int) {
	i := start + 2
	for i+1 < len(input) {
		if input[i] == '*' && input[i+1] == '/' {
			return input[start : i+2], i + 2
		}
		i++
	}
	return input[start:], len(input)
}
