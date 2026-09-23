# State Management

## Import

Import lets practitioners bring existing resources under Terraform management without recreating them.

### Simple Import (PassthroughID)

When the import ID is the same as the resource's `id` attribute:

```go
var _ resource.ResourceWithImportState = &fooResource{}

func (r *fooResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
```

Usage: `terraform import googleworkspace_user.example "user-id-123"`

### Compound Import (Split ID)

When import needs multiple values. This provider uses `importSplitId`:

```go
func (r *driveResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    importSplitId(ctx, req, resp, "use_domain_admin_access", "id")
}
```

Usage: `terraform import googleworkspace_drive.example "true,drive-id-123"`

The `importSplitId` helper splits on comma and sets each part to the corresponding attribute path.

### Custom Import Logic

For complex imports that need API lookups:

```go
func (r *fooResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    // req.ID contains whatever the user passed to `terraform import`
    parts := strings.SplitN(req.ID, "/", 2)
    if len(parts) != 2 {
        resp.Diagnostics.AddError("Invalid Import ID", "Expected format: parent/name")
        return
    }

    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("parent"), parts[0])...)
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}
```

After ImportState sets the minimal attributes, Terraform calls Read to fill in the rest.

## State Upgrade

When you change a resource schema in a breaking way, existing state in `.tfstate` files won't match the new schema. State upgraders transform old state to the new format transparently.

### When to Use

- Changing a list block to SingleNestedBlock
- Renaming attributes
- Changing attribute types (e.g., string to int)
- Restructuring nested objects

### Implementation

1. Increment `Version` in the schema:

```go
func (r *fooResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Version: 1, // Was 0, now 1
        // ... current schema ...
    }
}
```

2. Implement `resource.ResourceWithUpgradeState`:

```go
var _ resource.ResourceWithUpgradeState = &fooResource{}

func (r *fooResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {
            StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
                // Parse raw JSON from old state format
                var raw map[string]json.RawMessage
                if err := json.Unmarshal(req.RawState.JSON, &raw); err != nil {
                    resp.Diagnostics.AddError("State Upgrade Error",
                        fmt.Sprintf("Unable to parse raw state: %s", err))
                    return
                }

                // Extract values from old format
                var id string
                _ = json.Unmarshal(raw["id"], &id)

                // Handle structural changes (e.g., list → single nested)
                var name string
                if nameRaw, ok := raw["name"]; ok {
                    var nameList []map[string]string
                    if err := json.Unmarshal(nameRaw, &nameList); err == nil && len(nameList) > 0 {
                        name = nameList[0]["value"]
                    }
                }

                // Write to current model
                state := fooResourceModel{
                    Id:   types.StringValue(id),
                    Name: types.StringValue(name),
                }
                resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
            },
        },
    }
}
```

### Key Points

- The map key is the OLD schema version (upgrade FROM version X)
- `req.RawState.JSON` contains the raw JSON bytes of the old state
- Parse manually — the old state shape does not match your current model struct
- After upgrade, Terraform calls Read to refresh state with current API data
- Multiple upgraders can be chained (0→1, 1→2, etc.)

## Private State

Store provider-internal data that is not visible in plan output. Useful for:

- ETags or version tokens for optimistic concurrency
- Internal identifiers that shouldn't be user-visible
- Cached metadata to avoid extra API calls

```go
var _ resource.ResourceWithPrivateState = &fooResource{}

// In Create or Update:
resp.Private.SetKey(ctx, "etag", []byte(apiResponse.Etag))

// In Read or Update:
etagBytes, diags := req.Private.GetKey(ctx, "etag")
etag := string(etagBytes)
```

## Writing State

### Full Model Write

Most common — write the entire model struct to state:

```go
resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
```

### Individual Attribute Write

Set a single attribute by path:

```go
resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), "new-id")...)
```

### Removing Resource from State

When Read discovers the resource no longer exists:

```go
resp.State.RemoveResource(ctx)
```

This tells Terraform the resource was deleted externally and needs recreation.

## Related Framework References

| File                                           | Contents                          |
| ---------------------------------------------- | --------------------------------- |
| `framework/resources/import.mdx`               | Import state documentation        |
| `framework/resources/state-upgrade.mdx`        | State upgrade details             |
| `framework/resources/private-state.mdx`        | Private state storage             |
| `framework/resources/state-move.mdx`           | State move between resource types |
| `framework/handling-data/writing-state.mdx`    | Writing to response state         |
| `framework/handling-data/accessing-values.mdx` | Reading from state/plan/config    |
