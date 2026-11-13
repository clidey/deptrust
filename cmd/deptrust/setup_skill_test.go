package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clidey/deptrust"
)

func TestNativeRefreshBacksUpExistingSkillAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	parent := filepath.Join(home, ".agents", "skills")
	target := filepath.Join(parent, "deptrust-package-check")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	old := []byte("old skill with local instructions\n")
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), old, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "custom.txt"), []byte("keep this too"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := refreshIntegrations("/example/deptrust", &out); err != nil {
		t.Fatal(err)
	}
	read := func(path string) []byte {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	path := filepath.Join(target, "SKILL.md")
	if string(read(path)) != deptrust.PackageCheckSkill() {
		t.Fatal("installed skill does not match embedded source")
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	backups := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "deptrust-package-check.bak-") {
			backups = append(backups, entry.Name())
		}
	}
	if len(backups) != 1 {
		t.Fatalf("backups: %v", backups)
	}
	backup := filepath.Join(parent, backups[0])
	if !bytes.Equal(read(filepath.Join(backup, "SKILL.md")), old) || string(read(filepath.Join(backup, "custom.txt"))) != "keep this too" {
		t.Fatal("lost customization")
	}
	stamp := time.Unix(1000000000, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := refreshIntegrations("/example/deptrust", &out); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(stamp) {
		t.Fatal("unchanged skill was rewritten")
	}
	after, err := os.ReadDir(parent)
	if err != nil || len(after) != len(entries) {
		t.Fatal("repeat refresh created another backup")
	}
}

func TestNativeSkillInstallRequiresExplicitOptIn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var out bytes.Buffer
	if err := runSetup([]string{"--refresh"}, strings.NewReader(""), &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents")); !os.IsNotExist(err) {
		t.Fatal("refresh enabled absent skill")
	}
	if err := runSetup([]string{"--skill"}, strings.NewReader(""), &out, &out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "deptrust-package-check", "SKILL.md"))
	if err != nil || string(b) != deptrust.PackageCheckSkill() {
		t.Fatal("explicit skill install failed")
	}
	for _, name := range []string{".codex", ".claude", ".claude.json"} {
		if _, err := os.Stat(filepath.Join(home, name)); !os.IsNotExist(err) {
			t.Fatalf("enabled unrelated integration %s", name)
		}
	}
}
