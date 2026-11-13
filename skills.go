// Package deptrust provides the integration assets shipped with the CLI.
package deptrust

import _ "embed"

//go:embed .agents/skills/deptrust-package-check/SKILL.md
var packageCheckSkill string

// PackageCheckSkill returns the same skill source distributed by the npm package.
func PackageCheckSkill() string { return packageCheckSkill }
