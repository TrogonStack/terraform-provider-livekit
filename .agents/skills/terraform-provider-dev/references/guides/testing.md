# Testing

## Overview

This provider uses acceptance tests against an in-memory fake of the generated `livekit.CloudAgent` Twirp service, never real LiveKit Cloud API calls. The testing pattern:

1. `fake_cloud_agent_test.go` defines `fakeCloudAgent`, a mutex-guarded, in-memory implementation of the full `CloudAgent` interface. The RPCs the provider actually calls (`CreateAgentV2`, `ListAgents`, `UpdateAgent`, `DeleteAgent`, `ListAgentSecrets`, `UpdateAgentSecrets`) are backed by maps; everything else returns `twirp.NewError(twirp.Unimplemented, ...)`
2. `setupTestServer` wraps the fake in `livekit.NewCloudAgentServer(fake)` and serves it from an `httptest.Server`
3. `setupTestClient` points a real `lksdk.AgentClient` at that server and injects it as the package-level `testAPIClient`, bypassing provider configuration (and JWT signing against a real project) entirely
4. `resource.Test` runs Terraform configurations and assertions against that fake backend

## Test Infrastructure

Defined in `provider_test.go`:

```go
// Provider factory for test cases
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
    "livekit": providerserver.NewProtocol6WithError(New("test")()),
}

// Minimal valid provider config (values unused since testAPIClient bypasses auth)
const testProviderConfig = `
provider "livekit" {
  url        = "https://test.livekit.cloud"
  api_key    = "test-key"
  api_secret = "test-secret"
}
`

// Create test HTTP server and register cleanup
func setupTestServer(t *testing.T, handler http.Handler) *httptest.Server {
    t.Helper()
    server := httptest.NewServer(handler)
    t.Cleanup(server.Close)
    return server
}

// Point a real AgentClient at the fake server and inject it as testAPIClient
func setupTestClient(t *testing.T, server *httptest.Server) {
    t.Helper()
    t.Setenv("LK_AGENTS_URL", server.URL)
    agentClient, err := lksdk.NewAgentClient(server.URL, "test-key", "test-secret", lksdk.WithHTTPClient(server.Client()))
    if err != nil {
        t.Fatalf("failed to create agent client: %v", err)
    }
    testAPIClient = &apiClient{agent: agentClient}
    t.Cleanup(func() { testAPIClient = nil })
}
```

Every acceptance test follows the same three-line setup:

```go
fake := newFakeCloudAgent()
server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
setupTestClient(t, server)
```

## The Fake CloudAgent

`fakeCloudAgent` (`fake_cloud_agent_test.go`) keeps agents, their region assignments, and their secrets in maps:

```go
type fakeCloudAgent struct {
    mu             sync.Mutex
    nextID         int
    undeployed     bool
    kindUnreported bool
    agents         map[string]*livekit.AgentInfo
    regions        map[string][]string
    secrets        map[string]map[string]*livekit.AgentSecret
}
```

Two boolean fields exist purely to simulate API quirks that the resources must tolerate:

- `undeployed`: when true, `deployments()` always returns an empty slice, simulating an agent that has been created but has no deployment yet (`AgentInfo.AgentDeployments` empty even though regions were configured)
- `kindUnreported`: when true, `ListAgentSecrets` reports every secret's `Kind` as `AGENT_SECRET_KIND_UNKNOWN`, simulating an API that does not echo back the secret kind it was given

Tests reach into the fake's maps directly (under `fake.mu`) to assert on server-side state that never appears in Terraform state, such as a secret's raw value:

```go
fake.mu.Lock()
defer fake.mu.Unlock()
secret, ok := fake.secrets[agentId]["MY_SECRET"]
if !ok {
    t.Fatalf("expected secret MY_SECRET to exist for agent %s", agentId)
}
if string(secret.Value) != "top-secret" {
    t.Fatalf("expected secret value %q, got %q", "top-secret", string(secret.Value))
}
```

## Basic Test Structure

```go
func TestAccAgent_Basic(t *testing.T) {
    fake := newFakeCloudAgent()
    server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
    setupTestClient(t, server)

    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east", "us-west"]
}
`,
                Check: resource.ComposeAggregateTestCheckFunc(
                    resource.TestCheckResourceAttrSet("livekit_agent.test", "id"),
                    resource.TestCheckResourceAttrSet("livekit_agent.test", "agent_id"),
                    resource.TestCheckResourceAttr("livekit_agent.test", "regions.#", "2"),
                    resource.TestCheckTypeSetElemAttr("livekit_agent.test", "regions.*", "us-east"),
                ),
            },
        },
    })
}
```

## Multi-Step Tests (Create then Update)

```go
func TestAccAgent_UpdateRegions(t *testing.T) {
    fake := newFakeCloudAgent()
    server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
    setupTestClient(t, server)

    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east"]
}
`,
                Check: resource.TestCheckResourceAttr("livekit_agent.test", "regions.#", "1"),
            },
            {
                Config: testProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east", "eu-west"]
}
`,
                Check: resource.ComposeAggregateTestCheckFunc(
                    resource.TestCheckResourceAttr("livekit_agent.test", "regions.#", "2"),
                    resource.TestCheckTypeSetElemAttr("livekit_agent.test", "regions.*", "eu-west"),
                ),
            },
        },
    })
}
```

## Import Tests

```go
{
    ResourceName:      "livekit_agent.test",
    ImportState:       true,
    ImportStateVerify: true,
    ImportStateIdFunc: func(s *terraform.State) (string, error) {
        return s.RootModule().Resources["livekit_agent.test"].Primary.ID, nil
    },
},
```

For a write-only attribute that import cannot populate, ignore it in verification (`livekit_agent_secret`):

```go
{
    ResourceName:            "livekit_agent_secret.test",
    ImportState:             true,
    ImportStateVerify:       true,
    ImportStateVerifyIgnore: []string{"value_wo_version"},
    ImportStateIdFunc: func(s *terraform.State) (string, error) {
        return s.RootModule().Resources["livekit_agent_secret.test"].Primary.ID, nil
    },
},
```

For the compound `<agent_id>/<name>` import ID format, pass it directly to `ImportStateId`, or derive it as above from the resource's own `id`.

## Delete-Not-Found Tests

Verify graceful handling when a resource is deleted externally. Since the fake has no HTTP layer to intercept, tests mutate the fake's maps directly between steps and use `PlanOnly` + `ExpectNonEmptyPlan` to assert that Read detected the drift:

```go
func TestAccAgent_DeleteNotFound(t *testing.T) {
    fake := newFakeCloudAgent()
    server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
    setupTestClient(t, server)

    config := testProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east"]
}
`
    var agentId string

    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: config,
                Check: func(s *terraform.State) error {
                    agentId = s.RootModule().Resources["livekit_agent.test"].Primary.ID
                    return nil
                },
            },
            {
                PreConfig: func() {
                    fake.mu.Lock()
                    delete(fake.agents, agentId)
                    delete(fake.regions, agentId)
                    delete(fake.secrets, agentId)
                    fake.mu.Unlock()
                },
                Config:             config,
                PlanOnly:           true,
                ExpectNonEmptyPlan: true,
            },
        },
    })
}
```

## Testing API Quirks Directly

Two provider behaviors exist specifically to tolerate CloudAgent API responses that don't echo back what was configured. Both are tested by flipping a fake flag before the test steps run, not by mocking HTTP responses:

```go
func TestAccAgent_RegionsKeptBeforeFirstDeployment(t *testing.T) {
    fake := newFakeCloudAgent()
    fake.undeployed = true
    server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
    setupTestClient(t, server)
    // ... assert regions.# is still 1, even though AgentDeployments came back empty
}

func TestAccAgentSecret_FileKindNotReportedByAPI(t *testing.T) {
    fake := newFakeCloudAgent()
    fake.kindUnreported = true
    server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
    setupTestClient(t, server)
    // ... assert kind is still "file", even though ListAgentSecrets reported AGENT_SECRET_KIND_UNKNOWN
}
```

## Testing Write-Only Secret Rotation

`value_wo_version` is what tells the provider to send a new `value_wo`. A rotation test bumps the version and asserts the fake's stored value changed:

```go
func TestAccAgentSecret_Rotate(t *testing.T) {
    fake := newFakeCloudAgent()
    server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
    setupTestClient(t, server)

    checkValue := func(want string) resource.TestCheckFunc {
        return func(s *terraform.State) error {
            agentId := s.RootModule().Resources["livekit_agent.test"].Primary.ID
            fake.mu.Lock()
            defer fake.mu.Unlock()
            secret := fake.secrets[agentId]["MY_SECRET"]
            if string(secret.Value) != want {
                t.Fatalf("expected secret value %q, got %q", want, string(secret.Value))
            }
            return nil
        }
    }

    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testProviderConfig + `
resource "livekit_agent" "test" {}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "MY_SECRET"
  value_wo         = "version-one"
  value_wo_version = 1
}
`,
                Check: checkValue("version-one"),
            },
            {
                Config: testProviderConfig + `
resource "livekit_agent" "test" {}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "MY_SECRET"
  value_wo         = "version-two"
  value_wo_version = 2
}
`,
                Check: checkValue("version-two"),
            },
        },
    })
}
```

## Testing the Retry Policy Directly

`retry.go`'s `retryPolicy` is a plain function, so it gets ordinary unit tests instead of acceptance tests, with no server or fake involved (`retry_test.go`):

```go
func TestRetryPolicy_429_Retries(t *testing.T) {
    resp := &http.Response{StatusCode: 429}
    retry, err := retryPolicy(context.Background(), resp, nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !retry {
        t.Error("expected retry on 429")
    }
}

func TestRetryPolicy_501_DoesNotRetry(t *testing.T) {
    resp := &http.Response{StatusCode: 501}
    retry, _ := retryPolicy(context.Background(), resp, nil)
    if retry {
        t.Error("expected no retry on 501")
    }
}
```

## Check Functions

| Function                                                       | Use Case                     |
| ------------------------------------------------------------------ | --------------------------------- |
| `resource.TestCheckResourceAttr(name, key, value)`             | Exact attribute match        |
| `resource.TestCheckResourceAttrSet(name, key)`                 | Attribute is set (any value) |
| `resource.TestCheckNoResourceAttr(name, key)`                  | Attribute is NOT set         |
| `resource.TestCheckTypeSetElemAttr(name, key, value)`          | Element present in a Set attribute (`regions.*`) |
| `resource.TestCheckResourceAttrPair(name1, key1, name2, key2)` | Two attributes match         |
| `resource.ComposeAggregateTestCheckFunc(...)`                  | Combine multiple checks      |

## Running Tests

```bash
# All acceptance tests
go test ./internal/provider/ -v -run TestAcc

# One resource
go test ./internal/provider/ -v -run TestAccAgent

# One test
go test ./internal/provider/ -v -run TestAccAgentSecret_Rotate

# With race detection (the fake is mutex-guarded, so this should stay clean)
go test ./internal/provider/ -race -v -run TestAcc

# Full suite, matching `mise run test`
go test -count=1 -cover ./...
```

## Test Naming Convention

```
TestAcc<Resource>_<Scenario>
```

Examples from this provider:

- `TestAccAgent_Basic`
- `TestAccAgent_UpdateRegions`
- `TestAccAgent_Import`
- `TestAccAgent_DeleteNotFound`
- `TestAccAgent_RegionsKeptBeforeFirstDeployment`
- `TestAccAgentSecret_Basic`
- `TestAccAgentSecret_Rotate`
- `TestAccAgentSecret_Import`
- `TestAccAgentSecret_DeleteNotFound`
- `TestAccAgentSecret_FileKindNotReportedByAPI`

## Related Framework References

| File                      | Contents                             |
| --------------------------- | ----------------------------------------- |
| `framework/acctests.mdx`  | Acceptance test setup with framework |
| `framework/debugging.mdx` | Debugging test failures              |
