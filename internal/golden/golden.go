// Package golden holds what the Bun CLI printed or wrote, so parity tests
// compare the Go port with it without running Bun.
//
// The files live under testdata/bun next to the tests that read them. A test
// asks for one by name and passes the function that produces it from Bun.
// That function runs only when a maintainer regenerates the files:
//
//	make golden BUN_REPO=/path/to/agentio
//
// which sets AGENTIO_BUN_REPO to a checkout of github.com/plosson/agentio
// with its dependencies installed. Without it, tests never run Bun.
package golden

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// bunRepo is read once, at start-up: testbox.Isolate clears the AGENTIO_*
// variables before most tests ask for it.
var bunRepo = os.Getenv("AGENTIO_BUN_REPO")

// Regenerating reports whether the goldens are being rewritten from Bun.
func Regenerating() bool { return bunRepo != "" }

// Repo is the Bun checkout the goldens are regenerated from, or "".
func Repo() string { return bunRepo }

// Path is where the golden called name is kept, in the calling package.
func Path(name string) string { return filepath.Join("testdata", "bun", filepath.FromSlash(name)) }

// Bytes returns the golden called name. When regenerating, it first runs
// produce and writes what it returns.
func Bytes(t *testing.T, name string, produce func(t *testing.T) []byte) []byte {
	t.Helper()
	path := Path(name)
	if Regenerating() {
		data := produce(t)
		if t.Failed() {
			t.FailNow()
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
		return data
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (regenerate with: make golden BUN_REPO=/path/to/agentio)", err)
	}
	return data
}

// JSON decodes the golden called name into out. When regenerating, it first
// writes what produce returns, as indented JSON.
func JSON(t *testing.T, name string, out any, produce func(t *testing.T) any) {
	t.Helper()
	data := Bytes(t, name, func(t *testing.T) []byte {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(produce(t)); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	})
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("%s: %v", Path(name), err)
	}
}

// Bun is a command running bun in the Bun checkout, with nothing of this
// process's environment but PATH: HOME is a throwaway directory, network
// access goes to a closed proxy (loopback excepted), and open/xdg-open do
// nothing. env is added last.
func Bun(t *testing.T, env []string, args ...string) *exec.Cmd {
	t.Helper()
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Fatal("regenerating the goldens needs bun: ", err)
	}
	shims := t.TempDir()
	for _, name := range []string{"open", "xdg-open"} {
		if err := os.WriteFile(filepath.Join(shims, name), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(bun, args...)
	cmd.Dir = bunRepo
	cmd.Env = append([]string{
		"HOME=" + t.TempDir(), "TMPDIR=" + os.TempDir(), "NODE_ENV=test",
		"PATH=" + shims + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HTTPS_PROXY=http://127.0.0.1:9", "HTTP_PROXY=http://127.0.0.1:9", "NO_PROXY=127.0.0.1,localhost",
	}, env...)
	return cmd
}
