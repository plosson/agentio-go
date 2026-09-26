package vault_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/plosson/agentio-go/internal/golden"
	"github.com/plosson/agentio-go/internal/testbox"
	"github.com/plosson/agentio-go/internal/vault"
)

const testPass = "test-pass-123"

// A vault written by a newer Bun, or holding state Go does not model: unknown
// keys at every level, a service Go has no plugin for, and profile arrays that
// mix bare strings and objects.
const foreignVault = `{
  "version": 1,
  "topExtra": {"a": 1, "b": [true, null, "s"]},
  "config": {
    "profiles": {
      "acme": ["bare", {"name": "obj", "readOnly": true, "note": "keep"}, {"name": "plain"}, {"name": "ro-false", "readOnly": false}],
      "whatsapp": ["wa"]
    },
    "apiKeys": [{
      "id": "k1", "name": "key", "secretHash": "h", "allowedProfiles": "*",
      "readOnly": false, "createdAt": "2026-01-01T00:00:00.000Z",
      "future": {"x": [1, 2]}, "canAddProfiles": true
    }],
    "server": {"oauth": {"clients": {"c1": {"secret": "s", "expiresAt": 1735689600000}}}}
  },
  "credentials": {
    "whatsapp": {"wa": {"session": "x", "nested": {"n": 1.5}}},
    "acme": {"obj": {"accessToken": "t", "extra": [1, "two"]}}
  }
}`

// seedForeign creates a vault in the isolated HOME and overwrites it with raw.
func seedForeign(t *testing.T, raw string) string {
	t.Helper()
	testbox.Isolate(t)
	t.Setenv("AGENTIO_PASSPHRASE", testPass)
	path := vault.DefaultVaultPath()
	if err := vault.Create(path, testPass, vault.EmptyContents()); err != nil {
		t.Fatal(err)
	}
	enc, err := vault.Encrypt(raw, testPass)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(enc), 0o600); err != nil {
		t.Fatal(err)
	}
	vault.Reset()
	return path
}

// onDisk decrypts the vault file into a generic JSON value.
func onDisk(t *testing.T, path string) map[string]any {
	t.Helper()
	enc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := vault.Decrypt(string(enc), testPass)
	if err != nil {
		t.Fatal(err)
	}
	return parseJSON(t, plain)
}

func parseJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("%v\n%s", err, raw)
	}
	return v
}

func assertSameJSON(t *testing.T, got, want map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		g, _ := json.MarshalIndent(got, "", "  ")
		w, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("vault changed\n got: %s\nwant: %s", g, w)
	}
}

func TestLoadSaveKeepsWhatGoDoesNotModel(t *testing.T) {
	path := seedForeign(t, foreignVault)
	c, err := vault.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.Save(c); err != nil {
		t.Fatal(err)
	}
	assertSameJSON(t, onDisk(t, path), parseJSON(t, foreignVault))
}

func TestUpdateChangesOnlyWhatItTouches(t *testing.T) {
	path := seedForeign(t, foreignVault)
	if err := vault.Update(func(c *vault.Contents) error {
		c.Credentials.Put("ping", "p", testbox.Object(map[string]any{"k": "v"}))
		c.Config.Profiles.Set("ping", append(c.Config.Profiles.Get("ping"), vault.ProfileValue{Name: "p"}))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := parseJSON(t, foreignVault)
	want["config"].(map[string]any)["profiles"].(map[string]any)["ping"] = []any{map[string]any{"name": "p"}}
	want["credentials"].(map[string]any)["ping"] = map[string]any{"p": map[string]any{"k": "v"}}
	assertSameJSON(t, onDisk(t, path), want)
}

// bunForeign is testdata/bun/foreign.json: the foreign vault as Bun's
// updateVault wrote it (encrypted with testPass), and the plaintext Bun
// writes when it then adds the profile the test below adds from Go.
type bunForeign struct {
	Vault   string `json:"vault"`
	AfterGo string `json:"afterGo"`
}

// Go reads the vault Bun wrote, changes one profile, and writes exactly the
// bytes Bun writes for the same change: nothing either side does not model
// is lost, and nothing moves.
func TestBunVaultChangedByGoIsWhatBunWrites(t *testing.T) {
	path := seedForeign(t, `{"version":1,"config":{"profiles":{}},"credentials":{}}`)
	var ref bunForeign
	golden.JSON(t, "foreign.json", &ref, func(t *testing.T) any {
		bun := func(script string) {
			t.Helper()
			cmd := golden.Bun(t, []string{"HOME=" + os.Getenv("HOME"), "AGENTIO_TEST=1", "AGENTIO_PASSPHRASE=" + testPass, "FOREIGN=" + foreignVault}, "-e", script)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("bun: %v\n%s", err, out)
			}
		}
		bun(`
import { updateVault } from "./src/vault/vault.ts";
await updateVault((v) => { Object.assign(v, JSON.parse(process.env.FOREIGN)); });
`)
		written, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		bun(`
import { updateVault } from "./src/vault/vault.ts";
await updateVault((v) => {
  v.config.profiles.acme.push({ name: "go" });
  v.credentials.acme.go = { from: "go" };
});
`)
		enc, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		after, err := vault.Decrypt(string(enc), testPass)
		if err != nil {
			t.Fatal(err)
		}
		return bunForeign{Vault: string(written), AfterGo: after}
	})
	if err := os.WriteFile(path, []byte(ref.Vault), 0o600); err != nil {
		t.Fatal(err)
	}
	vault.Reset()
	if err := vault.Update(func(c *vault.Contents) error {
		c.Credentials.Put("acme", "go", testbox.Object(map[string]any{"from": "go"}))
		c.Config.Profiles.Set("acme", append(c.Config.Profiles.Get("acme"), vault.ProfileValue{Name: "go"}))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	enc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := vault.Decrypt(string(enc), testPass)
	if err != nil {
		t.Fatal(err)
	}
	if got != ref.AfterGo {
		t.Fatalf("the vault differs from Bun's\n got: %s\nwant: %s", got, ref.AfterGo)
	}
	want := parseJSON(t, foreignVault)
	profiles := want["config"].(map[string]any)["profiles"].(map[string]any)
	profiles["acme"] = append(profiles["acme"].([]any), map[string]any{"name": "go"})
	want["credentials"].(map[string]any)["acme"].(map[string]any)["go"] = map[string]any{"from": "go"}
	assertSameJSON(t, onDisk(t, path), want)
	if !strings.HasPrefix(path, os.TempDir()) {
		t.Fatalf("vault escaped the temp dir: %s", path)
	}
}

// The daemon locks and unlocks from one handler while others, /health among
// them, ask whether it is unlocked. The resident passphrase must survive that
// under -race.
func TestResidentPassphraseIsSafeAcrossGoroutines(t *testing.T) {
	testbox.Isolate(t)
	if err := vault.Create(vault.DefaultVaultPath(), testPass, vault.EmptyContents()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTIO_PASSPHRASE", "")
	vault.SetMemoryOnly(true)
	t.Cleanup(vault.ResetPassphrase)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 5; i++ {
			if err := vault.Unlock(testPass); err != nil {
				t.Error(err)
			}
			vault.Lock()
		}
	}()
	for {
		select {
		case <-done:
			return
		default:
			_ = vault.Unlocked()
		}
	}
}
