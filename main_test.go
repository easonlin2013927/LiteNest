package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanRelative(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"", "", true},
		{"photos/2026", "photos/2026", true},
		{"/photos//summer", "photos/summer", true},
		{"../secret", "", false},
		{"photos/../../secret", "", false},
		{`bad\..\secret`, "", false},
	}
	for _, test := range tests {
		got, err := cleanRelative(test.input)
		if test.ok && (err != nil || got != test.want) {
			t.Errorf("cleanRelative(%q) = %q, %v; want %q", test.input, got, err, test.want)
		}
		if !test.ok && err == nil {
			t.Errorf("cleanRelative(%q) unexpectedly succeeded", test.input)
		}
	}
}

func TestCopyUploadQuota(t *testing.T) {
	var output bytes.Buffer
	written, err := copyUpload(&output, bytes.NewBufferString("12345"), 4)
	if !errors.Is(err, errQuota) || written != 5 {
		t.Fatalf("expected quota error after 5 bytes, got %d, %v", written, err)
	}
	output.Reset()
	written, err = copyUpload(&output, bytes.NewBufferString("12345"), -1)
	if err != nil || written != 5 || output.String() != "12345" {
		t.Fatalf("unlimited copy failed: %d, %v", written, err)
	}
}

func TestStoreCreatesDefaultAdmin(t *testing.T) {
	dir := t.TempDir()
	store, err := loadStore(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	admin := store.data.Users["admin"]
	if admin == nil || !admin.Admin || !verifyPassword("admin", admin) {
		t.Fatal("default administrator was not created correctly")
	}
	info, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("config permissions are too broad: %v", info.Mode().Perm())
	}
}
