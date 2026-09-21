package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The password must never reach the disk in readable form.
func TestDPAPIRoundTrip(t *testing.T) {
	const secret = "rahasia-socks-123"
	blob := protectPassword(secret)
	if blob == "" {
		t.Fatal("protectPassword returned nothing")
	}
	if strings.Contains(blob, secret) {
		t.Fatal("plaintext leaked into the DPAPI blob")
	}
	got, err := unprotectPassword(blob)
	if err != nil {
		t.Fatalf("unprotectPassword: %v", err)
	}
	if got != secret {
		t.Fatalf("round trip mismatch: %q", got)
	}
	if _, err := unprotectPassword("not-a-blob"); err == nil {
		t.Fatal("garbage blob should not decrypt silently")
	}
}

// settings.json must carry pass_enc, never the password itself.
func TestSettingsFileNeverStoresPlainPassword(t *testing.T) {
	dir := t.TempDir()
	app := &App{runDir: dir}
	app.settings = Settings{Host: "10.12.132.225", Port: 1080, User: "u", Pass: "sangat-rahasia", Tray: true}
	app.saveSettings()

	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if strings.Contains(string(raw), "sangat-rahasia") {
		t.Fatal("settings.json contains the plaintext password")
	}

	var written Settings
	if err := json.Unmarshal(raw, &written); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	if written.Pass != "" {
		t.Fatal("plain password field was written")
	}
	if written.PassEnc == "" {
		t.Fatal("encrypted password field is missing")
	}
	plain, err := unprotectPassword(written.PassEnc)
	if err != nil || plain != "sangat-rahasia" {
		t.Fatalf("stored blob does not decrypt back: %q %v", plain, err)
	}
}
