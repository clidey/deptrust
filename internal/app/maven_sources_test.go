package app

import (
	"github.com/clidey/deptrust/internal/models"
	"os"
	"path/filepath"
	"testing"
)

func TestMavenSourceArtifacts(t *testing.T) {
	for _, xml := range []string{
		`<settings><mirrors><mirror><mirrorOf>*</mirrorOf><url>https://private.invalid/maven</url></mirror></mirrors></settings>`,
		`<project><repositories><repository><url>https://repo.maven.apache.org/maven2</url></repository></repositories></project>`,
	} {
		file := filepath.Join(t.TempDir(), "effective.xml")
		if err := os.WriteFile(file, []byte(xml), 0600); err != nil {
			t.Fatal(err)
		}
		assertUnavailableSource(t, models.Query{Ecosystem: models.EcosystemMaven, Package: "company:secret", SourceFile: file})
	}
}
