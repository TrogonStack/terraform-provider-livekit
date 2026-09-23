# Testing

## Overview

This provider uses acceptance tests with a mock HTTP server — no real Google Workspace API calls. The testing pattern:

1. Create an `httptest.Server` with a handler matching expected API requests
2. Inject it via `setupTestClient`
3. Run `resource.Test` with Terraform configurations and assertions

## Test Infrastructure

Defined in `provider_test.go`:

```go
// Provider factory for test cases
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
    "googleworkspace": providerserver.NewProtocol6WithError(New("test")()),
}

// Minimal valid provider config (values unused due to mock client)
const testProviderConfig = `
provider "googleworkspace" {
  access_token            = "test-token"
  service_account         = "test@test.iam.gserviceaccount.com"
  impersonated_user_email = "admin@test.com"
  customer_id             = "C00000000"
}
`

// Create test HTTP server and register cleanup
func setupTestServer(t *testing.T, handler http.Handler) *httptest.Server {
    t.Helper()
    server := httptest.NewServer(handler)
    t.Cleanup(server.Close)
    return server
}

// Inject mock client into provider (bypasses authentication)
func setupTestClient(t *testing.T, server *httptest.Server) {
    t.Helper()
    testAPIClient = &apiClient{
        client:     server.Client(),
        customerID: "C00000000",
        basePath:   server.URL,
    }
    t.Cleanup(func() { testAPIClient = nil })
}

// Write JSON response
func jsonResponse(w http.ResponseWriter, status int, body any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(body)
}
```

## Basic Test Structure

```go
func TestAccFoo_Basic(t *testing.T) {
    server := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch {
        case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/foos"):
            jsonResponse(w, 200, map[string]any{
                "id":   "foo-123",
                "name": "test",
            })

        case r.Method == "GET" && strings.Contains(r.URL.Path, "/foos/foo-123"):
            jsonResponse(w, 200, map[string]any{
                "id":   "foo-123",
                "name": "test",
            })

        case r.Method == "DELETE" && strings.Contains(r.URL.Path, "/foos/foo-123"):
            w.WriteHeader(204)

        default:
            t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
            w.WriteHeader(500)
        }
    }))
    setupTestClient(t, server)

    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testProviderConfig + `
resource "googleworkspace_foo" "test" {
  name = "test"
}
`,
                Check: resource.ComposeAggregateTestCheckFunc(
                    resource.TestCheckResourceAttr("googleworkspace_foo.test", "id", "foo-123"),
                    resource.TestCheckResourceAttr("googleworkspace_foo.test", "name", "test"),
                ),
            },
        },
    })
}
```

## Multi-Step Tests (Create → Update)

```go
func TestAccFoo_Update(t *testing.T) {
    var updateCount atomic.Int32

    server := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch {
        case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/foos"):
            jsonResponse(w, 200, map[string]any{"id": "foo-456", "name": "original"})

        case r.Method == "PATCH" && strings.Contains(r.URL.Path, "/foos/foo-456"):
            updateCount.Add(1)
            jsonResponse(w, 200, map[string]any{"id": "foo-456", "name": "updated"})

        case r.Method == "GET" && strings.Contains(r.URL.Path, "/foos/foo-456"):
            name := "original"
            if updateCount.Load() > 0 {
                name = "updated"
            }
            jsonResponse(w, 200, map[string]any{"id": "foo-456", "name": name})

        case r.Method == "DELETE":
            w.WriteHeader(204)

        default:
            t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
            w.WriteHeader(500)
        }
    }))
    setupTestClient(t, server)

    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testProviderConfig + `
resource "googleworkspace_foo" "test" {
  name = "original"
}
`,
                Check: resource.TestCheckResourceAttr("googleworkspace_foo.test", "name", "original"),
            },
            {
                Config: testProviderConfig + `
resource "googleworkspace_foo" "test" {
  name = "updated"
}
`,
                Check: resource.TestCheckResourceAttr("googleworkspace_foo.test", "name", "updated"),
            },
        },
    })
}
```

## Import Tests

```go
{
    // First step creates the resource
    Config: testProviderConfig + `
resource "googleworkspace_foo" "test" {
  name = "imported"
}
`,
},
{
    ResourceName:      "googleworkspace_foo.test",
    ImportState:       true,
    ImportStateId:     "foo-123",           // What user passes to `terraform import`
    ImportStateVerify: true,                // Verify imported state matches
    ImportStateVerifyIgnore: []string{      // Skip attributes that differ
        "some_write_only_field",
    },
},
```

For compound import IDs:

```go
ImportStateId: "true,drive-123",  // matches importSplitId format
```

## Delete-Not-Found Tests

Verify graceful handling when a resource is deleted externally:

```go
func TestAccFoo_DeleteNotFound(t *testing.T) {
    server := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch {
        case r.Method == "POST":
            jsonResponse(w, 200, map[string]any{"id": "foo-gone", "name": "test"})

        case r.Method == "GET":
            // Simulate external deletion
            jsonResponse(w, 404, map[string]any{
                "error": map[string]any{"code": 404, "message": "Not found"},
            })

        case r.Method == "DELETE":
            w.WriteHeader(204)

        default:
            t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
            w.WriteHeader(500)
        }
    }))
    setupTestClient(t, server)

    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testProviderConfig + `
resource "googleworkspace_foo" "test" {
  name = "test"
}
`,
                ExpectNonEmptyPlan: true, // Read returns 404 → removed from state → plan shows create
            },
        },
    })
}
```

## Verifying Request Bodies

Use `sync/atomic` and captured request bodies:

```go
var lastBody []byte

server := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if r.Method == "PATCH" {
        body, _ := io.ReadAll(r.Body)
        lastBody = body
        // ... respond
    }
}))

// After resource.Test completes:
var reqBody map[string]any
json.Unmarshal(lastBody, &reqBody)
if val, exists := reqBody["enabled"]; !exists {
    t.Error("ForceSendFields bug: 'enabled' not sent")
}
```

## Check Functions

| Function                                                       | Use Case                     |
| -------------------------------------------------------------- | ---------------------------- |
| `resource.TestCheckResourceAttr(name, key, value)`             | Exact attribute match        |
| `resource.TestCheckResourceAttrSet(name, key)`                 | Attribute is set (any value) |
| `resource.TestCheckNoResourceAttr(name, key)`                  | Attribute is NOT set         |
| `resource.TestCheckResourceAttrPair(name1, key1, name2, key2)` | Two attributes match         |
| `resource.ComposeAggregateTestCheckFunc(...)`                  | Combine multiple checks      |

## Running Tests

```bash
# All acceptance tests
go test ./internal/provider/ -v -run TestAcc

# Specific resource
go test ./internal/provider/ -v -run TestAccDrive

# Specific test
go test ./internal/provider/ -v -run TestAccDrive_Import

# With race detection
go test ./internal/provider/ -race -v -run TestAcc

# Short timeout (tests use mock server, should be fast)
go test ./internal/provider/ -v -timeout 60s -run TestAcc
```

## Test Naming Convention

```
TestAcc<Resource>_<Scenario>
```

Examples:

- `TestAccDrive_CreateWithRestrictions`
- `TestAccDrive_UpdateRestrictions`
- `TestAccDrive_Import`
- `TestAccDrive_DeleteNotFound`
- `TestAccUser_Basic`

## Related Framework References

| File                      | Contents                             |
| ------------------------- | ------------------------------------ |
| `framework/acctests.mdx`  | Acceptance test setup with framework |
| `framework/debugging.mdx` | Debugging test failures              |
