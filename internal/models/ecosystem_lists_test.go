package models

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// ecosystemIDs reads the Ecosystem constants from models.go, so a new
// ecosystem is picked up without editing this test.
func ecosystemIDs(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "models.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if ident, ok := spec.Type.(*ast.Ident); !ok || ident.Name != "Ecosystem" {
			return true
		}
		for _, value := range spec.Values {
			if literal, ok := value.(*ast.BasicLit); ok {
				id, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				ids = append(ids, id)
			}
		}
		return true
	})
	if len(ids) == 0 {
		t.Fatal("no Ecosystem constants found in models.go")
	}
	return ids
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile("../../" + path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEcosystemListsNameEveryEcosystem(t *testing.T) {
	ids := ecosystemIDs(t)

	usage := regexp.MustCompile(`Supported ecosystems: (.*)`).FindStringSubmatch(readRepoFile(t, "cmd/deptrust/main.go"))
	description := regexp.MustCompile(`ecosystemDescription = "(.*)"`).FindStringSubmatch(readRepoFile(t, "internal/mcp/server.go"))
	for name, match := range map[string][]string{"printUsage in cmd/deptrust/main.go": usage, "ecosystemDescription in internal/mcp/server.go": description} {
		if match == nil {
			t.Fatalf("%s: ecosystem list not found", name)
		}
		listed := strings.FieldsFunc(match[1], func(r rune) bool { return r == ',' || r == ' ' || r == '.' })
		for _, id := range ids {
			if !contains(listed, id) {
				t.Errorf("%s does not list %q", name, id)
			}
		}
	}

	table := regexp.MustCompile(`(?s)\| Ecosystem \| Registry metadata .*?\n\n`).FindString(readRepoFile(t, "README.md"))
	// Every table line starts with "|"; the header and separator are not ecosystems.
	rows := strings.Count(table, "\n|") - 1
	if rows != len(ids) {
		t.Errorf("README provider coverage table has %d rows; models.go defines %d ecosystems", rows, len(ids))
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
