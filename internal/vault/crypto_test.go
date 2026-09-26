package vault

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/plosson/agentio-go/internal/golden"
)

func TestDecryptRejectsGarbage(t *testing.T) {
	if _, err := Decrypt("not-base64!!!", "whatever-pass"); err == nil {
		t.Fatal("non-base64 payload decrypted")
	}
	short := EncryptMust(t, "x", "long-enough-passphrase")
	raw := []byte(short)
	if _, err := Decrypt(string(raw[:8]), "long-enough-passphrase"); err == nil {
		t.Fatal("truncated payload decrypted")
	}
	if _, err := Decrypt(short, "wrong-passphrase-value"); err == nil {
		t.Fatal("wrong passphrase decrypted")
	}
}

// bunWireCase is a plaintext as Bun's encryptVault wrote it, and what Bun's
// decryptVault then read back: the plaintext, or the message it threw.
type bunWireCase struct {
	Plain string `json:"plain"`
	Enc   string `json:"enc"`
	Read  string `json:"read"`
	Error string `json:"error,omitempty"`
}

// bunWire is testdata/bun/wire.json: the cases under one passphrase.
type bunWire struct {
	Pass  string        `json:"pass"`
	Cases []bunWireCase `json:"cases"`
}

var wirePlaintexts = []string{
	`{"version":1,"note":"go-then-bun"}`,
	"{\"s\":\"é😀\u2028\",\"n\":1}",
	"",
	strings.Repeat("x", 4096),
}

// Go reads what Bun encrypts as Bun does (an empty plaintext is too short for
// both), rejects it under another passphrase or with a byte changed, and
// writes the same envelope: base64 of salt, IV, ciphertext and tag, as long
// as Bun's for the same plaintext.
func TestBunCiphertextIsReadable(t *testing.T) {
	var ref bunWire
	golden.JSON(t, "wire.json", &ref, func(t *testing.T) any {
		out := bunWire{Pass: "correct horse battery"}
		for _, plain := range wirePlaintexts {
			cmd := golden.Bun(t, []string{"PLAIN=" + plain, "PASS=" + out.Pass}, "-e",
				`import { decryptVault, encryptVault } from "./src/vault/crypto.ts";
const enc = await encryptVault(process.env.PLAIN, process.env.PASS);
let read = '', error;
try { read = await decryptVault(enc, process.env.PASS); } catch (e) { error = e.message; }
process.stdout.write(JSON.stringify({ plain: process.env.PLAIN, enc, read, error }));`)
			raw, err := cmd.Output()
			if err != nil {
				t.Fatalf("bun: %v", err)
			}
			var c bunWireCase
			if err := json.Unmarshal(raw, &c); err != nil {
				t.Fatal(err)
			}
			out.Cases = append(out.Cases, c)
		}
		return out
	})
	if len(ref.Cases) != len(wirePlaintexts) {
		t.Fatalf("wire.json has %d cases, want %d: regenerate the goldens", len(ref.Cases), len(wirePlaintexts))
	}
	for i, c := range ref.Cases {
		if c.Plain != wirePlaintexts[i] {
			t.Fatalf("wire.json case %d is for another plaintext: regenerate the goldens", i)
		}
		got, err := Decrypt(c.Enc, ref.Pass)
		gotErr := ""
		if err != nil {
			gotErr = err.Error()
		}
		if got != c.Read || gotErr != c.Error {
			t.Errorf("case %d: read %q, %q; Bun read %q, %q", i, got, gotErr, c.Read, c.Error)
		}
		if _, err := Decrypt(c.Enc, ref.Pass+"!"); err == nil {
			t.Errorf("case %d: decrypted under the wrong passphrase", i)
		}
		raw, err := base64.StdEncoding.DecodeString(c.Enc)
		if err != nil {
			t.Fatal(err)
		}
		raw[len(raw)-1] ^= 1
		if _, err := Decrypt(base64.StdEncoding.EncodeToString(raw), ref.Pass); err == nil {
			t.Errorf("case %d: decrypted with the tag changed", i)
		}
		mine, err := Encrypt(c.Plain, ref.Pass)
		if err != nil {
			t.Fatal(err)
		}
		if len(mine) != len(c.Enc) || StructurallyValid(mine) != StructurallyValid(c.Enc) || StructurallyValid(mine) != (c.Error == "") {
			t.Errorf("case %d: Go's envelope %d bytes (valid %v), Bun's %d (valid %v)", i, len(mine), StructurallyValid(mine), len(c.Enc), StructurallyValid(c.Enc))
		}
	}
}

func TestPassphraseFileIsNotConsultedWhenMemoryOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENTIO_TEST", "1")
	t.Setenv("AGENTIO_PASSPHRASE", "")
	Reset()
	if err := StorePassphrase("stored-passphrase"); err != nil {
		t.Fatal(err)
	}
	SetMemoryOnly(true)
	ResetPassphrase()
	SetMemoryOnly(true)
	if pw, src := lookupPassphrase(); pw != "" || src != fromNone {
		t.Fatalf("memory-only provider returned %q from %s", pw, src)
	}
}

func TestTestGuardRefusesRealHome(t *testing.T) {
	t.Setenv("AGENTIO_TEST", "1")
	err := AssertTestWritable("/Users/plosson/.config/agentio/vault.path", "vault pointer")
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("guard = %v", err)
	}
}

func EncryptMust(t *testing.T, plain, pass string) string {
	t.Helper()
	enc, err := Encrypt(plain, pass)
	if err != nil {
		t.Fatal(err)
	}
	return enc
}
