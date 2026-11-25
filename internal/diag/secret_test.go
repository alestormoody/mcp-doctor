package diag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const canary = "canary-value-7f3a"

type exported struct {
	Name  string
	Token Secret
}

type unexported struct {
	name  string
	token Secret
}

type pointerField struct {
	Token *Secret
}

type sliceField struct {
	Tokens []Secret
}

func checkRedacted(t *testing.T, name, out string, wantMarker bool) {
	t.Helper()
	if strings.Contains(out, canary) {
		t.Errorf("%s leaked the secret: %q", name, out)
	}
	if wantMarker && !strings.Contains(out, Redacted) {
		t.Errorf("%s missing %q: %q", name, Redacted, out)
	}
}

func TestSecretFmt(t *testing.T) {
	s := NewSecret(canary)
	sp := &s
	cases := []struct {
		name       string
		out        string
		wantMarker bool
	}{
		{"%v", fmt.Sprintf("%v", s), true},
		{"%+v", fmt.Sprintf("%+v", s), true},
		{"%#v", fmt.Sprintf("%#v", s), true},
		{"%s", fmt.Sprintf("%s", s), true},
		{"%q", fmt.Sprintf("%q", s), true},
		{"%x", fmt.Sprintf("%x", s), true},
		{"%X", fmt.Sprintf("%X", s), true},
		{"%d", fmt.Sprintf("%d", s), true},
		{"pointer %v", fmt.Sprintf("%v", sp), true},
		{"Sprint", fmt.Sprint(s), true},
		{"Sprintln", fmt.Sprintln(s), true},
		{"Errorf", fmt.Errorf("x: %v", s).Error(), true},
		{"exported %+v", fmt.Sprintf("%+v", exported{"a", s}), true},
		{"exported %#v", fmt.Sprintf("%#v", exported{"a", s}), true},
		{"unexported %+v", fmt.Sprintf("%+v", unexported{"a", s}), false},
		{"unexported %#v", fmt.Sprintf("%#v", unexported{"a", s}), false},
		{"pointer field %+v", fmt.Sprintf("%+v", pointerField{sp}), false},
		{"pointer field %#v", fmt.Sprintf("%#v", pointerField{sp}), false},
		{"slice field %+v", fmt.Sprintf("%+v", sliceField{[]Secret{s}}), true},
		{"slice field %#v", fmt.Sprintf("%#v", sliceField{[]Secret{s}}), true},
	}
	for _, c := range cases {
		checkRedacted(t, c.name, c.out, c.wantMarker)
	}
}

func TestSecretJSON(t *testing.T) {
	s := NewSecret(canary)
	for name, v := range map[string]any{
		"struct":  exported{"a", s},
		"map":     map[string]Secret{"k": s},
		"pointer": pointerField{&s},
		"slice":   sliceField{[]Secret{s}},
	} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		checkRedacted(t, "json "+name, string(b), true)
	}
	txt, err := s.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	checkRedacted(t, "MarshalText", string(txt), true)
}

func TestSecretSlog(t *testing.T) {
	s := NewSecret(canary)
	handlers := map[string]func(*bytes.Buffer) slog.Handler{
		"text": func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
		"json": func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
	}
	for name, h := range handlers {
		var buf bytes.Buffer
		l := slog.New(h(&buf))
		l.Info("m", slog.Any("s", s))
		l.Info("m", slog.Any("cfg", exported{"a", s}))
		l.Info("m", slog.Group("g", slog.Any("s", s)))
		checkRedacted(t, "slog "+name, buf.String(), true)
	}
}

func TestSecretReveal(t *testing.T) {
	if got := NewSecret(canary).Reveal(); got != canary {
		t.Errorf("Reveal() = %q, want %q", got, canary)
	}
	var zero Secret
	if got := zero.Reveal(); got != "" {
		t.Errorf("zero Reveal() = %q, want empty", got)
	}
	if got := fmt.Sprint(zero); got != Redacted {
		t.Errorf("zero prints %q, want %q", got, Redacted)
	}
}
