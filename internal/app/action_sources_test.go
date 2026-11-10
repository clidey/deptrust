package app

import (
	"context"
	"github.com/clidey/deptrust/internal/models"
	"os"
	"path/filepath"
	"testing"
)

func TestActionRepositoryVisibility(t *testing.T) {
	for _, tt := range []struct {
		data    string
		blocked bool
	}{
		{`{"full_name":"company/secret","private":false,"visibility":"public","html_url":"https://github.com/company/secret"}`, false},
		{`{"full_name":"company/secret","private":true,"visibility":"private","html_url":"https://github.com/company/secret"}`, true},
		{`{"full_name":"company/other","private":false,"visibility":"public","html_url":"https://github.com/company/other"}`, true},
		{`{"full_name":"company/secret"}`, true},
	} {
		file := filepath.Join(t.TempDir(), "repository.json")
		if err := os.WriteFile(file, []byte(tt.data), 0600); err != nil {
			t.Fatal(err)
		}
		q := models.Query{Ecosystem: models.EcosystemGitHubActions, Package: "company/secret", SourceFile: file}
		reason, err := sourceRestriction(context.Background(), q)
		if err != nil || (reason != "") != tt.blocked {
			t.Fatalf("%q %v", reason, err)
		}
		if tt.blocked {
			assertUnavailableSource(t, q)
		}
	}
	for _, name := range []string{"./.github/actions/private", "../action", "docker://private.invalid/action"} {
		assertUnavailableSource(t, models.Query{Ecosystem: models.EcosystemGitHubActions, Package: name})
	}
}
