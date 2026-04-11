package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestAppHelp(t *testing.T) {
	var out bytes.Buffer
	app := NewApp("dev", "unknown")
	app.SetStdout(&out)
	if err := app.Run(nil); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "Commands:") {
		t.Fatalf("unexpected help output: %q", got)
	}
}

func TestAppVersion(t *testing.T) {
	var out bytes.Buffer
	app := NewApp("1.2.3", "now")
	app.SetStdout(&out)
	if err := app.Run([]string{"version"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "1.2.3") {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRequireTarget(t *testing.T) {
	target, err := requireTarget([]string{"https://example.com", "-format", "json"})
	if err != nil {
		t.Fatalf("requireTarget returned error: %v", err)
	}
	if target != "https://example.com" {
		t.Fatalf("unexpected target: %q", target)
	}
}

func TestRequireTargetMissing(t *testing.T) {
	if _, err := requireTarget([]string{"-format", "json", "-timeout=1s"}); err == nil {
		t.Fatal("expected missing target error")
	}
}

func TestRequireTargetSkipsKnownFlagValues(t *testing.T) {
	target, err := requireTarget([]string{"-format", "json", "-insecure", "https://example.com"})
	if err != nil {
		t.Fatalf("requireTarget returned error: %v", err)
	}
	if target != "https://example.com" {
		t.Fatalf("unexpected target: %q", target)
	}
}

func TestFlagTakesValue(t *testing.T) {
	if !flagTakesValue("-format") || !flagTakesValue("--trace-dir") {
		t.Fatal("expected known flags to require values")
	}
	if flagTakesValue("-insecure") {
		t.Fatal("expected boolean flag not to require value")
	}
}

func TestAppUnknownCommand(t *testing.T) {
	app := NewApp("dev", "unknown")
	if err := app.Run([]string{"wat"}); err == nil {
		t.Fatal("expected unknown command error")
	}
}
