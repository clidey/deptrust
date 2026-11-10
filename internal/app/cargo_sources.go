package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

func cargoRestriction(ctx context.Context, query models.Query) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "cargo", "metadata", "--format-version", "1", "--no-deps", "--offline")
	cmd.Dir = query.ProjectDir
	cmd.Env = append(os.Environ(), "RUSTUP_AUTO_INSTALL=0")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot read offline Cargo metadata for project")
	}
	reason, err := cargoMetadataRestriction(out, query.Package)
	if err != nil || reason != "" {
		return reason, err
	}
	// Manifest-only metadata does not resolve patches. Confirm the actual source
	// from the locked graph before allowing a public assessment.
	resolved := exec.CommandContext(ctx, "cargo", "metadata", "--format-version", "1", "--offline", "--locked")
	resolved.Dir = query.ProjectDir
	resolved.Env = append(os.Environ(), "RUSTUP_AUTO_INSTALL=0")
	out, err = resolved.Output()
	if err != nil {
		return "", fmt.Errorf("cannot verify locked Cargo sources offline; a current lockfile and cached dependency metadata are required")
	}
	return cargoResolvedRestriction(out, query.Package)
}

func cargoMetadataRestriction(data []byte, name string) (string, error) {
	var metadata struct {
		Packages []struct {
			Name         string `json:"name"`
			Dependencies []struct {
				Name     string  `json:"name"`
				Rename   string  `json:"rename"`
				Source   *string `json:"source"`
				Registry *string `json:"registry"`
				Path     string  `json:"path"`
			} `json:"dependencies"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", fmt.Errorf("invalid Cargo metadata")
	}
	const reason = "Cargo project does not exclusively declare this package from crates.io; public registry and advisory requests were skipped. Assessment is unavailable."
	found := false
	for _, pkg := range metadata.Packages {
		if pkg.Name == name {
			return reason, nil
		}
		for _, dep := range pkg.Dependencies {
			if dep.Name != name && dep.Rename != name {
				continue
			}
			found = true
			// Aliases are not public registry identities. Require the actual crate name.
			if dep.Name != name || dep.Path != "" || dep.Source == nil {
				return reason, nil
			}
			if *dep.Source != "registry+https://github.com/rust-lang/crates.io-index" && *dep.Source != "sparse+https://index.crates.io/" {
				return reason, nil
			}
			if dep.Registry != nil && strings.TrimRight(*dep.Registry, "/") != "https://github.com/rust-lang/crates.io-index" && strings.TrimRight(*dep.Registry, "/") != "sparse+https://index.crates.io" {
				return reason, nil
			}
		}
	}
	if !found {
		return reason, nil
	}
	return "", nil
}

func cargoResolvedRestriction(data []byte, name string) (string, error) {
	var metadata struct {
		Packages []struct {
			Name   string  `json:"name"`
			Source *string `json:"source"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", fmt.Errorf("invalid resolved Cargo metadata")
	}
	const reason = "Resolved Cargo package is not exclusively from crates.io; public registry and advisory requests were skipped. Assessment is unavailable."
	found := false
	for _, pkg := range metadata.Packages {
		if pkg.Name != name {
			continue
		}
		found = true
		if pkg.Source == nil || (*pkg.Source != "registry+https://github.com/rust-lang/crates.io-index" && *pkg.Source != "sparse+https://index.crates.io/") {
			return reason, nil
		}
	}
	if !found {
		return reason, nil
	}
	return "", nil
}
