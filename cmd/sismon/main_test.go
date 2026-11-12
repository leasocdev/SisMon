package main

import (
	"bytes"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"version"}, nil, &out, &out); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if out.String() != "SisMon 1.6.0-dev\n" {
		t.Fatalf("got %q", out.String())
	}
}
