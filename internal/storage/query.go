package storage

import (
	"fmt"
	"strings"
)

// RebindQuery converts the repository-wide ? bind markers to PostgreSQL's $n
// syntax when needed. Question marks inside single-quoted SQL strings are
// preserved, including strings with doubled quote escapes.
func RebindQuery(query string, postgres bool) string {
	if !postgres {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	arg := 1
	inSingleQuote := false
	for i := 0; i < len(query); i++ {
		switch query[i] {
		case '\'':
			b.WriteByte(query[i])
			if inSingleQuote && i+1 < len(query) && query[i+1] == '\'' {
				i++
				b.WriteByte(query[i])
				continue
			}
			inSingleQuote = !inSingleQuote
		case '?':
			if inSingleQuote {
				b.WriteByte(query[i])
			} else {
				fmt.Fprintf(&b, "$%d", arg)
				arg++
			}
		default:
			b.WriteByte(query[i])
		}
	}
	return b.String()
}
