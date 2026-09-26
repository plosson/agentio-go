package vault

import (
	"strings"
	"testing"
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
