# terraform-provider-livekit

Always use `/claude-md-improver` when updating this file.

Terraform provider for managing LiveKit Cloud agents and their secrets through the CloudAgent API.

- **Module**: `github.com/TrogonStack/terraform-provider-livekit`
- **Package**: `internal/provider/` (single flat package, all resources here)

## Commands

```bash
mise run test          # go test -count=1 -cover ./...
mise run lint          # golangci-lint run --fix ./...
mise run build         # full CI pipeline (download, tidy, lint, test, docs, diff)
mise run docs          # regenerate docs/ from schema descriptions
```

Single test:

```bash
go test ./internal/provider/ -v -run TestAccAgent
```

Runtime credentials the provider itself needs (not required to run the test suite, which never calls a real API):

- `LIVEKIT_URL`: the LiveKit Cloud project URL
- `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET`: the API key pair used to sign the admin JWT for every CloudAgent call

## Skills

Always load `terraform-provider-dev` when working on a resource or data source.

## Architecture

- File naming: `resource_<name>.go`, `resource_<name>_test.go`, `data_source_<name>.go`
- Provider client: `*apiClient` wraps `*lksdk.AgentClient` (`github.com/livekit/server-sdk-go/v2`), which signs an agent-admin JWT and dials the CloudAgent Twirp service
- Auth: API key/secret pair, signed into a JWT carrying an agent admin grant
- New resources must be registered in `provider.go` `Resources()` / `DataSources()`

## Conventions

### Resource naming

Use the same terminology as the CloudAgent API. `livekit_agent` maps to `AgentInfo`, `livekit_agent_secret` maps to `AgentSecret`. Never abbreviate to jargon that doesn't appear in the API surface.

### Not-found handling

Twirp reports a missing agent or secret as an error with code `twirp.NotFound`, never as an empty success response. Detect it with the shared `isNotFound(err)` helper in `errors.go`, which unwraps with `errors.As` into the `twirp.Error` interface rather than a type assertion, since a wrapped error would otherwise fail a direct type check.

- **Read**: call `resp.State.RemoveResource(ctx)` and return (resource was deleted externally)
- **Delete**: return without error (idempotent)

### Testing

Tests use an in-memory fake of the generated `livekit.CloudAgent` Twirp service, never real API calls:

- `fake_cloud_agent_test.go`: `fakeCloudAgent` implements the full `CloudAgent` interface; the RPCs the provider actually calls are backed by mutex-guarded maps, the rest return `twirp.Unimplemented`
- `setupTestServer()`: wraps the fake in `livekit.NewCloudAgentServer(fake)` and serves it from an `httptest.Server`
- `setupTestClient()`: points a real `lksdk.AgentClient` at that server (via `LK_AGENTS_URL`) and injects it as `testAPIClient`, bypassing provider configuration entirely

### Context propagation

Every `apiClient.agent` method takes `ctx` as its first argument and threads it straight through to the underlying Twirp client, so passing the resource CRUD method's own `ctx` is enough for Terraform cancellation (user Ctrl+C, timeouts) to reach in-flight requests. There is no separate `.Context(ctx)` call to remember.

### Write-only secret values

`livekit_agent_secret.value_wo` is a write-only argument: read from `req.Config`, never `req.Plan`, and never written back to state. It is paired with `value_wo_version` (a plain `Int64`) so that bumping the version is what tells the provider to send a new value; the value itself is never diffed.

### Retry

Automatic retry on 429 and 5xx except 501. No configuration attribute: the transport in `retry.go` is fixed, since there is no per-request quota signal on this API worth special-casing.

## Benchmarking

When designing resources or solving implementation questions, reference [`livekit/livekit-cli`](https://github.com/livekit/livekit-cli) for prior art on how the CloudAgent API is called, particularly `cmd/lk/agent.go`, which covers agent creation, secret listing and updates, and the fields the CLI chooses to display versus keep hidden (it never prints a secret's `value`, only its name and timestamps).

## CI

- PR: lint + test + build (GitHub Actions)
- Release: release-please + goreleaser on push to main
