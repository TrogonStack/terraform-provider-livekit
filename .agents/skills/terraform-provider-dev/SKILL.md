---
name: terraform-provider-dev
description: >
  Use this skill when developing terraform-provider-googleworkspace: adding
  resources or data sources, designing schemas, implementing CRUD operations,
  plan modification, state upgrades, import, validation, acceptance testing,
  debugging, or any Terraform Plugin Framework work in Go. Also use when the
  user asks about terraform provider patterns, attribute types, or how to
  structure tests. This is the primary development skill for this repository.
---

# Terraform Provider Development (Plugin Framework)

## Mental Model

- Provider = Go server implementing Terraform RPCs (GetProviderSchema, PlanResourceChange, ApplyResourceChange, ReadResource, etc.)
- Resource = struct implementing `resource.Resource` interface: Metadata, Schema, Configure, Create, Read, Update, Delete
- DataSource = struct implementing `datasource.DataSource` interface: Metadata, Schema, Configure, Read
- Schema defines the "shape" of config/plan/state: attributes (leaf values) and blocks (nested structures)
- Plan → Apply: Terraform calls PlanResourceChange (propose changes), then ApplyResourceChange (execute)
- State = Terraform's record of the real world; Plan = expected post-apply state
- Computed attributes: set by the provider from API responses (IDs, timestamps, server-generated values)
- Plugin Framework uses strong Go types: `types.String`, `types.Bool`, `types.Int64`, `types.List`, etc.
- Null vs Unknown: null = user did not set; unknown = value will be known after apply (planned computed)

---

## This Provider: Conventions

- **Package**: `internal/provider`
- **File naming**: `resource_<name>.go`, `resource_<name>_test.go`, `data_source_<name>.go`
- **Provider client**: `*apiClient` (wraps retryable HTTP client + customerID + basePath)
- **Client injection**: Configure method casts `req.ProviderData.(*apiClient)`
- **ID helper**: `rsId()` returns a Computed StringAttribute with `UseStateForUnknown`
- **Import helper**: `importSplitId(ctx, req, resp, boolAttr, idAttr)` for compound import IDs like `"true,drive-123"`
- **Simple import**: `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`
- **Registration**: Add constructor to `Resources()` or `DataSources()` in `provider.go`
- **Google API booleans**: Always use `ForceSendFields` to send false-valued booleans (Google API client uses omitempty)
- **404 handling**: Read → `resp.State.RemoveResource(ctx)` (resource deleted externally); Delete → return silently (idempotent)
- **Testing**: Mock HTTP server with `setupTestServer` + `setupTestClient`, no real API calls

---

## Adding a New Resource

1. Create `internal/provider/resource_<name>.go`
2. Define model struct(s) with `tfsdk` tags
3. Implement the resource:

```go
var (
    _ resource.Resource                = &fooResource{}
    _ resource.ResourceWithImportState = &fooResource{}
)

func newFoo() resource.Resource { return &fooResource{} }

type fooResource struct {
    client *apiClient
}

type fooResourceModel struct {
    Id   types.String `tfsdk:"id"`
    Name types.String `tfsdk:"name"`
}

func (r *fooResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_foo"
}

func (r *fooResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "id":   rsId(),
            "name": schema.StringAttribute{Required: true},
        },
    }
}

func (r *fooResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
    if req.ProviderData == nil {
        return
    }
    client, ok := req.ProviderData.(*apiClient)
    if !ok {
        resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *apiClient, got: %T", req.ProviderData))
        return
    }
    r.client = client
}
```

4. Implement Create, Read, Update, Delete (see guide: `references/guides/resource-lifecycle.md`)
5. Implement ImportState
6. Register in `provider.go`: add `newFoo` to `Resources()` return slice
7. Create `internal/provider/resource_foo_test.go` (see guide: `references/guides/testing.md`)

---

## Adding a Data Source

```go
var _ datasource.DataSource = &barDataSource{}

func newBarDataSource() datasource.DataSource { return &barDataSource{} }

type barDataSource struct {
    client *apiClient
}

type barDataSourceModel struct {
    Id   types.String `tfsdk:"id"`
    Name types.String `tfsdk:"name"`
}

func (d *barDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_bar"
}

func (d *barDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "id":   schema.StringAttribute{Computed: true},
            "name": schema.StringAttribute{Required: true},
        },
    }
}

func (d *barDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
    if req.ProviderData == nil {
        return
    }
    client, ok := req.ProviderData.(*apiClient)
    if !ok {
        resp.Diagnostics.AddError("Unexpected DataSource Configure Type", fmt.Sprintf("Expected *apiClient, got: %T", req.ProviderData))
        return
    }
    d.client = client
}

func (d *barDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
    var data barDataSourceModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
    if resp.Diagnostics.HasError() {
        return
    }
    // API call, populate data fields...
    resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
```

Register: add `newBarDataSource` to `DataSources()` in `provider.go`.

---

## Schema Design Quick-Reference

| Schema Type                                                                                    | Go Model Type            | When to Use                    |
| ---------------------------------------------------------------------------------------------- | ------------------------ | ------------------------------ |
| `schema.StringAttribute{Required: true}`                                                       | `types.String`           | User must provide              |
| `schema.StringAttribute{Optional: true}`                                                       | `types.String`           | User may provide               |
| `schema.StringAttribute{Computed: true}`                                                       | `types.String`           | Server-generated only          |
| `schema.StringAttribute{Optional: true, Computed: true}`                                       | `types.String`           | User provides OR server fills  |
| `schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)}` | `types.Bool`             | Bool with known default        |
| `schema.ListAttribute{ElementType: types.StringType}`                                          | `types.List`             | List of primitives             |
| `schema.SingleNestedBlock{Attributes: ...}`                                                    | `*nestedModel` (pointer) | Nested object (optional block) |

### Plan Modifiers

| Modifier                                  | Use Case                                   |
| ----------------------------------------- | ------------------------------------------ |
| `stringplanmodifier.UseStateForUnknown()` | Computed value stable across updates (IDs) |
| `stringplanmodifier.RequiresReplace()`    | Changing this forces resource recreation   |

Full details: `references/guides/schema-design.md`

---

## Testing Patterns

### Test Infrastructure

```go
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
    "googleworkspace": providerserver.NewProtocol6WithError(New("test")()),
}

const testProviderConfig = `
provider "googleworkspace" {
  access_token            = "test-token"
  service_account         = "test@test.iam.gserviceaccount.com"
  impersonated_user_email = "admin@test.com"
  customer_id             = "C00000000"
}
`
```

### Test Structure

```go
func TestAccFoo_Basic(t *testing.T) {
    server := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch {
        case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/foos"):
            jsonResponse(w, 200, map[string]any{"id": "foo-123", "name": "test"})
        case r.Method == "GET" && strings.Contains(r.URL.Path, "/foos/foo-123"):
            jsonResponse(w, 200, map[string]any{"id": "foo-123", "name": "test"})
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

### Running Tests

```bash
go test ./internal/provider/ -v -run TestAcc
go test ./internal/provider/ -v -run TestAccDrive
```

Full details: `references/guides/testing.md`

---

## State Upgrade

When changing a resource schema in a breaking way (e.g., changing a list block to SingleNestedBlock):

1. Increment `Version` in the schema
2. Implement `resource.ResourceWithUpgradeState` interface
3. Parse raw JSON state and write to current model

```go
func (r *fooResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {
            StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
                var raw map[string]json.RawMessage
                if err := json.Unmarshal(req.RawState.JSON, &raw); err != nil {
                    resp.Diagnostics.AddError("State Upgrade Error", fmt.Sprintf("Unable to parse raw state: %s", err))
                    return
                }
                // Parse old format, build new model, set state
                resp.Diagnostics.Append(resp.State.Set(ctx, &newModel)...)
            },
        },
    }
}
```

Full details: `references/guides/state-management.md`

---

## Reference Docs

### Topic Guides (synthesized, task-oriented)

| Guide                                         | Contents                                              |
| --------------------------------------------- | ----------------------------------------------------- |
| `references/guides/resource-lifecycle.md`     | CRUD methods, interface contracts, registration       |
| `references/guides/data-source-lifecycle.md`  | Data source pattern, Read method                      |
| `references/guides/schema-design.md`          | Attributes, blocks, types, nested models              |
| `references/guides/plan-modification.md`      | UseStateForUnknown, RequiresReplace, custom modifiers |
| `references/guides/state-management.md`       | Import, state upgrade, private state                  |
| `references/guides/validation.md`             | Attribute validators, resource-level validation       |
| `references/guides/testing.md`                | Acceptance tests, mock server, test steps             |
| `references/guides/provider-configuration.md` | Provider setup, client injection, servers             |
| `references/guides/functions.md`              | Provider-defined functions (Terraform 1.8+)           |

### Framework Reference (verbatim, 148 files)

Key entry points in `references/framework/`:

| File                                 | Contents                         |
| ------------------------------------ | -------------------------------- |
| `resources/index.mdx`                | Resource interface, registration |
| `resources/create.mdx`               | Create method contract           |
| `resources/read.mdx`                 | Read method, refresh state       |
| `resources/update.mdx`               | Update method, in-place changes  |
| `resources/delete.mdx`               | Delete method                    |
| `resources/configure.mdx`            | Client injection into resources  |
| `resources/import.mdx`               | Import state support             |
| `resources/plan-modification.mdx`    | Plan modifiers                   |
| `resources/state-upgrade.mdx`        | State upgrade for schema changes |
| `data-sources/index.mdx`             | Data source interface            |
| `handling-data/schemas.mdx`          | Schema definition                |
| `handling-data/accessing-values.mdx` | Reading config/plan/state        |
| `handling-data/writing-state.mdx`    | Writing to response state        |
| `handling-data/attributes/index.mdx` | All attribute types              |
| `handling-data/blocks/index.mdx`     | All block types                  |
| `handling-data/types/index.mdx`      | Type system (Go value types)     |
| `validation.mdx`                     | Validation patterns              |
| `diagnostics.mdx`                    | Error/warning diagnostics        |
| `acctests.mdx`                       | Acceptance testing setup         |
| `debugging.mdx`                      | Debugging providers              |
| `providers/index.mdx`                | Provider interface               |
| `provider-servers.mdx`               | Provider server (main.go)        |
| `functions/implementation.mdx`       | Provider functions               |
| `migrating/index.mdx`                | SDKv2 migration overview         |
