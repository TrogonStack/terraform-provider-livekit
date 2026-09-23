# Contributing

## Prerequisites

- [mise](https://mise.jdx.dev), which pins the Go, `golangci-lint`, GoReleaser, and `tfplugindocs` versions used by CI
- Terraform 1.11 or later, only if you intend to exercise the write-only `value_wo` argument locally (`mise install` also installs a pinned Terraform for the test suite)

Install the toolchain with `mise install`. Every command below runs through `mise` so local runs match CI.

## Development

```bash
mise run build   # full CI pipeline: download, lint, test, tidy, docs, diff
mise run test    # go test -count=1 -cover ./...
mise run lint    # golangci-lint run --fix ./...
mise run docs    # regenerate docs/ from schema descriptions
```

`mise run build` is what CI runs on every pull request, including the `git diff --exit-code` check, so run it before pushing.

## Testing

Tests run against an in-memory fake of the generated `livekit.CloudAgent` Twirp service and never reach LiveKit Cloud. `setupTestServer` wraps the fake in `livekit.NewCloudAgentServer` and serves it from an `httptest.Server`; `setupTestClient` points a real `lksdk.AgentClient` at that server and injects it as `testAPIClient`, bypassing provider configuration entirely.

```bash
mise exec -- go test ./internal/provider/ -v -run TestAccAgent
```

## Code layout

All resources live in the flat `internal/provider/` package, named `resource_<name>.go` with tests alongside as `<file>_test.go`. New resources must be registered in the `Resources()` or `DataSources()` method in `provider.go`, or the provider will not expose them.

Two things are easy to get wrong here:

- On a missing agent or secret, use the shared `isNotFound(err)` helper, which unwraps with `errors.As` into `twirp.Error` rather than a type assertion. A wrapped Twirp error fails a direct type assertion silently.
- `value_wo` must be read from `req.Config`, never `req.Plan`; the plan never carries a write-only value.

On a not-found error, `Read` should call `resp.State.RemoveResource(ctx)` and return; `Delete` should return without an error.

## Commits

Commits follow [Conventional Commits](https://www.conventionalcommits.org) and require a [DCO](https://developercertificate.org) sign-off:

```bash
git commit -s -m "fix(agent): correct region set diffing"
```

The commit type determines the next version, so it is worth getting right.

## Releases

[release-please](https://github.com/googleapis/release-please) reads the conventional commits merged into `main` and maintains an open release pull request with the computed version bump and changelog entries. Merging that pull request tags the release and publishes the provider archives, plus a GPG-signed checksum file, via [GoReleaser](https://goreleaser.com). No release happens without that pull request being merged.

Each release carries the assets the provider registry protocol expects: one zip per platform, a `SHA256SUMS` file, a detached GPG signature over it, and `terraform-provider-livekit_<version>_manifest.json` built from `terraform-registry-manifest.json` at the repository root. That manifest declares plugin protocol 6, which `providerserver.Serve` uses because `main.go` leaves `ProtocolVersion` unset. Registries assume protocol 5.0 when the manifest is missing, so a release without it installs and then fails to load.
