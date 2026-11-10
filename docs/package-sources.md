# Package source support

Source context is optional and explicit. Without context, checks use the public
registry, except for recognized Go privacy and Python/Dart environment settings,
repository-qualified Hex names, and local/container GitHub Actions identities.
A caller-selected artifact is a snapshot of that file only. It does not prove
freshness, complete effective configuration, authenticity, or installed content.
Do not use an incomplete snapshot to establish an effective private/public boundary.

| Manager | Implemented context | Remaining boundary |
|---|---|---|
| Go | Effective GOPRIVATE, saved GOENV, environment precedence | GONOPROXY/GONOSUMDB are not privacy declarations; no automatic classification from them |
| npm | Effective npm config with project_dir; selected .npmrc | Scope alone cannot identify private npmjs.org packages; authenticated visibility assessment is separate |
| Yarn | Selected Classic .yarnrc or Berry .yarnrc.yml registry/scopes | No complete inherited config/plugin/alias resolution |
| pnpm | Selected workspace/global YAML registry mappings | No inherited merge, named-registry routing, or URL-keyed declaration objects |
| Poetry | Native TOML sources, groups, optional dependencies, PEP 621 | No resolution/freshness or transitive source inheritance |
| uv | Selected project index constraints or uv.lock sources | No complete environment/global/project/CLI merge; custom recognized index environment stops public checks |
| Pipenv | Selected Pipfile/Pipfile.lock indexes | Missing lock sources are unknown; no complete runtime/global override reconstruction |
| pip | Selected pip.conf/pip.ini global/install indexes; recognized environment | No complete global/user/site/CLI hierarchy reconstruction |
| NuGet | Standalone config or effective .NET CLI-discovered hierarchy, mapping specificity, collection operations, source environment URLs | Requires a .NET SDK with config paths for hierarchy; directory must match restore context; unsupported mapping directives fail |
| Cargo | Offline metadata, locked resolved-source confirmation | Public checks require lock/cache; rustup auto-install is disabled; no arbitrary transitive-source query; alternate identities remain unknown |
| Bundler | Selected lockfile GEM sources, safe separation from foreign sources | No Gemfile evaluation/freshness/local overrides; Git/path/plugin gems remain unknown |
| Composer | Selected root repository declarations, only/exclude filters | No global/environment/plugin merge; cannot infer repository identity from lock download URLs |
| Dart/pub | Selected pubspec/lock sources, dependency overrides; PUB_HOSTED_URL | No workspace/pubspec_overrides.yaml merge or freshness |
| Maven | Offline effective-project lifecycle callback, parent/profile/settings processing and public mirror API | Trusted project/JDK/Maven required; direct declared dependencies only; system/reactor identities and mixed endpoints remain unknown; standalone XML still insufficient |
| Gradle | Offline trusted-project inspection using public repository/resolution APIs and an inspection task | Cached metadata required; no undeclared/transitive queries, local substitutions or composite builds; mixed sources remain unknown even if filters might narrow them |
| Hex | Selected API metadata or parser-only mix.lock AST; qualified repository names | Mix parsing requires Elixir; no effective runtime repository overrides or authenticated private fetching |
| Cabal | Selected plan.json package/repository locations | No plan generation, config inheritance, or freshness verification |
| CocoaPods | Selected Podfile.lock spec repos and external sources | No Podfile evaluation, installed repo overrides, or freshness verification |
| GitHub Actions | Local/container identities; selected repository visibility API response | No live authenticated visibility discovery or enterprise-host support |

## Proof and compatibility

- Every native reader has public/unsupported source fixtures where applicable.
- Private/uncertain check, suggest, and compare paths use a nil registry and a
  provider that panics if queried. CLI/MCP acceptance tests additionally count and
  reject application HTTP requests across all native artifact readers.
- Follow-up commands preserve the selector. Credential sentinels must not appear
  in results. Source selectors are mutually exclusive.
- Hooks accept explicit source_context; cwd enables effective npm/Cargo/NuGet checks.
  Other project managers require explicit context, and recognized source-override
  command flags stop the hook before public requests. Hook command parsing is not
  a complete shell interpreter.
- Bundler tests exercise the installed Ruby parser without evaluating Gemfile or
  altering lockfiles. Cabal/Hex fixtures follow official writers/API metadata;
  these are not live private-registry or physical installation proofs.
- No private-registry credentials, private advisory provider, installed-MCP
  replacement, remote publication, or production deployment is part of this work.

## Official sources

- [Go private modules](https://go.dev/ref/mod#private-modules)
- [npm config](https://docs.npmjs.com/cli/v11/using-npm/config/)
- [Yarn registry scopes](https://yarnpkg.com/configuration/yarnrc/)
- [pnpm settings](https://pnpm.io/settings)
- [Poetry sources](https://python-poetry.org/docs/repositories/)
- [uv indexes](https://docs.astral.sh/uv/configuration/indexes/)
- [Pipenv indexes](https://pipenv.pypa.io/en/stable/indexes.html)
- [pip configuration](https://pip.pypa.io/en/stable/topics/configuration/)
- [NuGet source mapping](https://learn.microsoft.com/en-us/nuget/consume-packages/package-source-mapping)
- [Cargo metadata](https://doc.rust-lang.org/cargo/commands/cargo-metadata.html)
- [Bundler lockfiles](https://guides.rubygems.org/gemfile-lock/)
- [Composer filters](https://getcomposer.org/doc/articles/repository-priorities.md)
- [Dart dependency sources](https://dart.dev/tools/pub/dependencies)
- [Maven repository order](https://maven.apache.org/guides/mini/guide-multiple-repositories)
- [Gradle exclusive content](https://docs.gradle.org/current/userguide/filtering_repository_content.html)
- [Hex private organizations](https://hex.pm/docs/private)
- [Cabal plan writer](https://github.com/haskell/cabal/blob/master/cabal-install/src/Distribution/Client/ProjectPlanOutput.hs)
- [CocoaPods lockfile](https://github.com/CocoaPods/Core/blob/master/lib/cocoapods-core/lockfile.rb)
- [GitHub repository metadata](https://docs.github.com/en/rest/repos/repos)

## Runtime integration verification

NuGet inheritance is checked against SDK NuGet.Configuration merge decisions,
not just matching Go fixtures. Mix uses Elixir AST parsing, tested in an official
network-disabled container without evaluating executable lock content.
Maven/Gradle runtime tests are opt-in (`DEPTRUST_TEST_JVM=1`) and use isolated
projects with local repository request counters. Maven inspects before build
goals; Gradle executes an inspection task and may run configuration-time code.
These integrations are for trusted projects and are not filesystem/network
sandboxes for arbitrary project code. Authenticated private endpoint fetching
and advisory-provider integration remain separate from source classification.

Run `python3 scripts/test-source-runtime.py` to require all NuGet, Maven,
Gradle, Mix, and built-process tests. It fails if a required test is missing or
skipped. Install Go, .NET SDK 10+, a compatible JDK/Maven/Gradle, npm, Cargo,
Ruby/Bundler, and Elixir first. Alternatively, pull the official Elixir image
pinned in `.github/workflows/ci.yml` and set `DEPTRUST_TEST_ELIXIR_IMAGE` to it;
Docker is then required. The Ubuntu 24.04 CI runtime job uses the runner's
installed toolchains and pulls that image before running tests. Runner software
versions are printed in the job; a GitHub Actions run verifies that environment.

The process suite builds the real CLI with the race detector, checks private
source decisions through CLI and stdio MCP, and executes generated follow-up
commands through `sh`, including paths with spaces and apostrophes. It checks
for credential disclosure, artifact mutation, and unexpected HTTP requests using
local proxy/repository tripwires. It also exercises quoted hook override flags.
These checks complement in-process HTTP guards; they do not sandbox project code
or authenticate to private registries. Gradle's public runtime fixture warms an
isolated dependency metadata cache online before exercising offline inspection.
