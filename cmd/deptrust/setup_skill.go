package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/clidey/deptrust"
)

// Refresh is opt-in by presence: upgrading never enables an absent skill.
// An explicit setup --skill also supports first-time installation without npm.
func configurePackageCheckSkill(home string, install bool, stdout io.Writer) error {
	parent := filepath.Join(home, ".agents", "skills")
	target := filepath.Join(parent, "deptrust-package-check")
	contents := []byte(deptrust.PackageCheckSkill())
	current, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if os.IsNotExist(err) && !install {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read installed skill: %w", err)
	}
	if err == nil && bytes.Equal(current, contents) {
		return nil
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	staged, err := os.MkdirTemp(parent, ".deptrust-skill-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged)
	if err := os.WriteFile(filepath.Join(staged, "SKILL.md"), contents, 0600); err != nil {
		return err
	}
	backup := ""
	if _, err := os.Lstat(target); err == nil {
		backup, err = os.MkdirTemp(parent, "deptrust-package-check.bak-")
		if err != nil {
			return err
		}
		// Reserve a unique name; remove only the empty directory we just made.
		if err := os.Remove(backup); err != nil {
			return err
		}
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("back up installed skill: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(staged, target); err != nil {
		if backup != "" {
			if restoreErr := os.Rename(backup, target); restoreErr != nil {
				return fmt.Errorf("install skill: %v; restore failed: %v; original preserved at %s", err, restoreErr, backup)
			}
		}
		return fmt.Errorf("install skill: %w", err)
	}
	if backup != "" {
		fmt.Fprintf(stdout, "Previous skill preserved at %s.\n", backup)
	}
	fmt.Fprintf(stdout, "Updated deptrust skill at %s.\n", target)
	return nil
}
