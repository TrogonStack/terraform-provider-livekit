# State Management

## Import

Import lets practitioners bring existing resources under Terraform management without recreating them.

### Simple Import (PassthroughID)

When the import ID is the same as the resource's `id` attribute. This is what `livekit_agent` uses, since its `id` is just the agent ID:

```go
var _ resource.ResourceWithImportState = &agentResource{}

func (r *agentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("agent_id"), req.ID)...)
}
```

Usage: `terraform import livekit_agent.example "agent-id-123"`

The extra `SetAttribute` call is needed because `agent_id` is a separate computed attribute from `id`; passthrough only populates `id`.

### Compound Import (Custom Parsing)

When import needs multiple values. `livekit_agent_secret`'s `id` is `<agent_id>/<name>`, so its `ImportState` parses that compound ID with the `parseAgentSecretImportID` helper in `helpers.go`:

```go
type agentSecretImportID struct {
    AgentID string
    Name    string
}

func parseAgentSecretImportID(raw string) (agentSecretImportID, error) {
    parts := strings.SplitN(raw, "/", 2)
    if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
        return agentSecretImportID{}, fmt.Errorf("expected import ID in the format <agent_id>/<name>, got: %q", raw)
    }
    return agentSecretImportID{AgentID: parts[0], Name: parts[1]}, nil
}

func (r *agentSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    parsed, err := parseAgentSecretImportID(req.ID)
    if err != nil {
        resp.Diagnostics.AddError("Invalid Import ID", err.Error())
        return
    }
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parsed.AgentID+"/"+parsed.Name)...)
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("agent_id"), parsed.AgentID)...)
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parsed.Name)...)
}
```

Usage: `terraform import livekit_agent_secret.example "agent-id-123/MY_SECRET"`

Because `value_wo` is write-only, it is never populated by import; the acceptance test for this resource uses `ImportStateVerifyIgnore: []string{"value_wo_version"}` (see `references/guides/testing.md`). Practitioners must set `value_wo` and `value_wo_version` in configuration and apply once after importing, to synchronize the secret's value.

After `ImportState` sets the minimal attributes, Terraform calls Read to fill in the rest.

## State Upgrade

When you change a resource schema in a breaking way, existing state in `.tfstate` files won't match the new schema. State upgraders transform old state to the new format transparently. Neither `livekit_agent` nor `livekit_agent_secret` has needed one yet: both schemas set no `Version` (so it defaults to `0`), and neither implements `resource.ResourceWithUpgradeState`.

### When to Use

- Changing a list block to SingleNestedBlock
- Renaming attributes
- Changing attribute types (e.g., string to int)
- Restructuring nested objects, e.g., if `regions` ever moved from a flat `Set` to a list of nested deployment objects

### Implementation

1. Increment `Version` in the schema:

```go
func (r *agentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Version: 1, // Was 0, now 1
        // ... current schema ...
    }
}
```

2. Implement `resource.ResourceWithUpgradeState`:

```go
var _ resource.ResourceWithUpgradeState = &agentResource{}

func (r *agentResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
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

                var agentId string
                _ = json.Unmarshal(raw["agent_id"], &agentId)

                state := agentResourceModel{
                    AgentId: types.StringValue(agentId),
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
- Parse manually: the old state shape does not match your current model struct
- After upgrade, Terraform calls Read to refresh state with current API data
- Multiple upgraders can be chained (0->1, 1->2, etc.)

## Private State

Store provider-internal data that is not visible in plan output. Useful for:

- ETags or version tokens for optimistic concurrency
- Internal identifiers that shouldn't be user-visible
- Cached metadata to avoid extra API calls

Not used anywhere in this provider today (there is no per-request ETag or token in the CloudAgent API surface the resources call). The pattern, if it's ever needed:

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

Most common: write the entire model struct to state:

```go
resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
```

### Individual Attribute Write

Set a single attribute by path. Both `ImportState` implementations in this provider use this to populate the ID fields before Terraform's follow-up Read:

```go
resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("agent_id"), parsed.AgentID)...)
```

### Removing Resource from State

When Read discovers the resource no longer exists. Both `agentResource.Read` and `agentSecretResource.Read` do this when their respective `find*` helper returns `nil, nil` (see `references/guides/resource-lifecycle.md`):

```go
resp.State.RemoveResource(ctx)
```

This tells Terraform the resource was deleted externally and needs recreation.

## Related Framework References

| File                                           | Contents                          |
| --------------------------------------------------- | -------------------------------------- |
| `framework/resources/import.mdx`               | Import state documentation        |
| `framework/resources/state-upgrade.mdx`        | State upgrade details             |
| `framework/resources/private-state.mdx`        | Private state storage             |
| `framework/resources/state-move.mdx`           | State move between resource types |
| `framework/handling-data/writing-state.mdx`    | Writing to response state         |
| `framework/handling-data/accessing-values.mdx` | Reading from state/plan/config    |
