package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

// Parse only the selected lock artifact. Never evaluate the executable Gemfile,
// load source plugins, resolve dependencies, or access a gem server.
const bundlerSourceScript = `
require "bundler"
require "json"
require "uri"
text = File.read(ARGV[0])
if text.lines.any? { |line| %w[GIT PATH PLUGIN].include?(line.strip) && line == line.lstrip }
  # Avoid constructing path, Git, and plugin sources. These may depend on
  # project context or load plugin code; this artifact cannot be assessed.
  puts JSON.generate({restricted: true})
  exit
end
parsed = Bundler::LockfileParser.new(text)
specs = parsed.specs.select { |spec| spec.name == ARGV[1] }
restricted = specs.empty? || specs.any? do |spec|
  source = spec.source
  next true unless source.instance_of?(Bundler::Source::Rubygems)
  remotes = source.remotes
  remotes.empty? || remotes.any? do |remote|
    uri = URI.parse(remote.to_s)
    uri.scheme != "https" || uri.host != "rubygems.org" || uri.port != 443 ||
      uri.userinfo || uri.query || uri.fragment || !["", "/"].include?(uri.path)
  end
end
puts JSON.generate({restricted: restricted})
`

func bundlerRestriction(ctx context.Context, query models.Query) (string, error) {
	if !filepath.IsAbs(query.BundlerLock) {
		return "", fmt.Errorf("bundler_lock must be absolute")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ruby", "--disable=rubyopt", "-e", bundlerSourceScript, query.BundlerLock, query.Package)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot read selected Bundler lockfile; Ruby and Bundler are required")
	}
	var decision struct {
		Restricted *bool `json:"restricted"`
	}
	if err := json.Unmarshal(out, &decision); err != nil || decision.Restricted == nil {
		return "", fmt.Errorf("invalid Bundler source decision")
	}
	if *decision.Restricted {
		return "Selected Bundler lockfile does not exclusively establish this gem from RubyGems.org; public registry and advisory requests were skipped. Assessment is unavailable.", nil
	}
	return "", nil
}
