package registry

import "fmt"

type VersionNotFoundError struct {
	Package string
	Version string
	Latest  string
}

func (e VersionNotFoundError) Error() string {
	if e.Latest == "" {
		return fmt.Sprintf("version %q was not found for package %q", e.Version, e.Package)
	}
	return fmt.Sprintf("version %q was not found for package %q; latest is %q", e.Version, e.Package, e.Latest)
}

type PackageNotFoundError struct {
	Package string
}

func (e PackageNotFoundError) Error() string {
	return fmt.Sprintf("package %q was not found", e.Package)
}
