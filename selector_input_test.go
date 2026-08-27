package main

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestQuitAwareInput(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{"q quits", "q", string(interruptByte)},
		{"escape quits", string(escapeByte), string(interruptByte)},
		{"arrow survives", "\x1b[A", "\x1b[A"},
	} {
		input := newQuitAwareInput(strings.NewReader(test.input), time.Millisecond)
		got, err := io.ReadAll(input)
		if err != nil {
			t.Errorf("%s: %v", test.name, err)
		} else if string(got) != test.want {
			t.Errorf("%s = %q, want %q", test.name, got, test.want)
		}
	}
}
