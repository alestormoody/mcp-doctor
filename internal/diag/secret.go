// Package diag holds the diagnostic primitives: Secret, Finding, the code registry and the exit-code convention.
package diag

import (
	"fmt"
	"log/slog"
)

// Redacted is what a Secret prints in place of its value.
const Redacted = "[REDACTED]"

// Secret holds a sensitive value that never appears in formatted, marshalled or logged output.
// The value sits behind a pointer so that even raw reflection of an unexported field shows an address.
type Secret struct{ v *string }

// NewSecret wraps s.
func NewSecret(s string) Secret { return Secret{v: &s} }

// Reveal returns the wrapped value. It is the only way to read it.
func (s Secret) Reveal() string {
	if s.v == nil {
		return ""
	}
	return *s.v
}

// String implements fmt.Stringer.
func (s Secret) String() string { return Redacted }

// GoString implements fmt.GoStringer.
func (s Secret) GoString() string { return Redacted }

// Format implements fmt.Formatter so every verb, including %d and %x, prints Redacted.
func (s Secret) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(Redacted)) }

// MarshalJSON implements json.Marshaler.
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + Redacted + `"`), nil }

// MarshalText implements encoding.TextMarshaler.
func (s Secret) MarshalText() ([]byte, error) { return []byte(Redacted), nil }

// LogValue implements slog.LogValuer.
func (s Secret) LogValue() slog.Value { return slog.StringValue(Redacted) }
