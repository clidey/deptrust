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
| Maven | Selected effective XML is recognized and returns unknown | One artifact omits other effective sources; full routing not implemented |
| Gradle | Official mechanisms researched; no routing implementation | Lockfiles omit routing; evaluating build configuration or inventing an export is outside this static-artifact implementation |
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
