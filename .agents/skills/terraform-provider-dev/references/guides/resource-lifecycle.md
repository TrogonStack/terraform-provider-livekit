# Resource Lifecycle

## Interface

A resource must implement `resource.Resource`:

```go
type Resource interface {
    Metadata(context.Context, MetadataRequest, *MetadataResponse)
    Schema(context.Context, SchemaRequest, *SchemaResponse)
    Create(context.Context, CreateRequest, *CreateResponse)
    Read(context.Context, ReadRequest, *ReadResponse)
    Update(context.Context, UpdateRequest, *UpdateResponse)
    Delete(context.Context, DeleteRequest, *DeleteResponse)
}
```

Optional interfaces:

- `resource.ResourceWithConfigure` — receive provider client
- `resource.ResourceWithImportState` — support `terraform import`
- `resource.ResourceWithUpgradeState` — handle schema migrations
- `resource.ResourceWithModifyPlan` — resource-level plan modification
- `resource.ResourceWithValidateConfig` — resource-level validation

## Registration

Add a constructor function to the provider's `Resources()` method:

```go
func newFoo() resource.Resource { return &fooResource{} }

// In provider.go:
func (p *googleworkspaceProvider) Resources(ctx context.Context) []func() resource.Resource {
    return []func() resource.Resource{
        newFoo,
    }
}
```

## Metadata

Sets the resource type name as it appears in Terraform configurations:

```go
func (r *fooResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_foo"
}
```

This produces `googleworkspace_foo` as the resource type.

## Configure

Receive the provider-configured API client:

```go
func (r *fooResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
    if req.ProviderData == nil {
        return
    }
    client, ok := req.ProviderData.(*apiClient)
    if !ok {
        resp.Diagnostics.AddError("Unexpected Resource Configure Type",
            fmt.Sprintf("Expected *apiClient, got: %T", req.ProviderData))
        return
    }
    r.client = client
}
```

The `nil` check is required — Configure is called during validation when provider data is not yet available.

## Create

Contract:

- Read plan data from `req.Plan`
- Perform API creation call
- Set ALL attribute values (including computed) in `resp.State`
- Unknown values in plan MUST become known in state (error otherwise)
- On error, the resource is marked tainted for recreation on next plan

```go
func (r *fooResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
    var plan fooResourceModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() {
        return
    }

    // API call to create...
    created, err := svc.Foos.Create(apiRequest).Do()
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to create foo: %s", err))
        return
    }

    // Set computed values from API response
    plan.Id = types.StringValue(created.Id)

    resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
```

## Read

Contract:

- Read prior state from `req.State`
- Perform API read call
- If resource no longer exists (404): call `resp.State.RemoveResource(ctx)` and return
- Otherwise, update all state values to reflect current API state

```go
func (r *fooResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
    var state fooResourceModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() {
        return
    }

    result, err := svc.Foos.Get(state.Id.ValueString()).Do()
    if err != nil {
        if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == 404 {
            resp.State.RemoveResource(ctx)
            return
        }
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read foo: %s", err))
        return
    }

    state.Name = types.StringValue(result.Name)
    resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
```

## Update

Contract:

- Read plan data from `req.Plan` (the desired new state)
- Perform API update call
- Set state to reflect the actual post-update values
- All values in state MUST match plan values (or Terraform produces "inconsistent result" error)

```go
func (r *fooResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
    var plan fooResourceModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() {
        return
    }

    apiReq := &api.Foo{
        Name:            plan.Name.ValueString(),
        ForceSendFields: []string{"SomeBool"}, // send false-valued booleans
    }

    updated, err := svc.Foos.Update(plan.Id.ValueString(), apiReq).Do()
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to update foo: %s", err))
        return
    }

    // Set any server-computed values
    plan.ComputedField = types.StringValue(updated.ComputedField)
    resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
```

## Delete

Contract:

- Read prior state from `req.State`
- Perform API deletion
- If already deleted (404): return without error (idempotent)
- No need to modify state — framework removes it automatically on success

```go
func (r *fooResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
    var state fooResourceModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() {
        return
    }

    err := svc.Foos.Delete(state.Id.ValueString()).Do()
    if err != nil {
        if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == 404 {
            return
        }
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to delete foo: %s", err))
        return
    }
}
```

## Google API: ForceSendFields

Google API Go client uses `omitempty` on all struct fields. Boolean `false`, integer `0`, and empty strings are omitted from the JSON request body unless listed in `ForceSendFields`. Always include boolean fields:

```go
apiReq := &api.Resource{
    Enabled:         plan.Enabled.ValueBool(),
    Restricted:      plan.Restricted.ValueBool(),
    ForceSendFields: []string{"Enabled", "Restricted"},
}
```

## Related Framework References

| File                                | Contents                                  |
| ----------------------------------- | ----------------------------------------- |
| `framework/resources/index.mdx`     | Resource type definition, full interface  |
| `framework/resources/create.mdx`    | Create method details and caveats         |
| `framework/resources/read.mdx`      | Read method and state refresh             |
| `framework/resources/update.mdx`    | Update method and plan consistency        |
| `framework/resources/delete.mdx`    | Delete method                             |
| `framework/resources/configure.mdx` | Configure method, provider data injection |
