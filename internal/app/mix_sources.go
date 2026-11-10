package app

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

// Parse Elixir syntax into AST only. Never evaluate mix.lock or load mix.exs.
const mixSourceScript = `
[file, name] = System.argv()
ast = file |> File.read!() |> Code.string_to_quoted!()
entries = case ast do
  {:%{}, _, entries} when is_list(entries) -> entries
  _ -> []
end
matches = Enum.filter(entries, fn
  {key, {:{}, _, [:hex, package | _]}} ->
    (is_binary(key) and key == name) or (is_atom(package) and Atom.to_string(package) == name)
  {key, _} -> is_binary(key) and key == name
  _ -> false
end)
public = matches != [] and Enum.all?(matches, fn
  {_key, {:{}, _, [:hex, package | rest]}} when is_atom(package) ->
    repo = Enum.at(rest, 4)
    Atom.to_string(package) == name and (repo == "hexpm" or is_nil(repo))
  _ -> false
end)
IO.puts(if public, do: "public", else: "unknown")
`

func mixArtifactRestriction(ctx context.Context, q models.Query) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "elixir", "-e", mixSourceScript, "--", q.SourceFile, q.Package)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot parse Mix lockfile; Elixir is required and the lockfile must contain valid syntax")
	}
	switch strings.TrimSpace(string(out)) {
	case "public":
		return "", nil
	case "unknown":
		return unsupportedArtifactSource, nil
	default:
		return "", fmt.Errorf("invalid Mix source decision")
	}
}
