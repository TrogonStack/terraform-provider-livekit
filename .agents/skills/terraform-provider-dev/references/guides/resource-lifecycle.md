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

- `resource.ResourceWithConfigure`: receive provider client
- `resource.ResourceWithImportState`: support `terraform import`
- `resource.ResourceWithUpgradeState`: handle schema migrations
- `resource.ResourceWithModifyPlan`: resource-level plan modification
- `resource.ResourceWithValidateConfig`: resource-level validation

Both resources in this provider (`agentResource`, `agentSecretResource`) implement `resource.Resource` and `resource.ResourceWithImportState`. Neither currently needs the others.

## Registration

Add a constructor function to the provider's `Resources()` method:

```go
func newFoo() resource.Resource { return &fooResource{} }

// In provider.go:
func (p *livekitProvider) Resources(ctx context.Context) []func() resource.Resource {
    return []func() resource.Resource{
        newAgent,
        newAgentSecret,
        newFoo,
    }
}
```

## Metadata

Sets the resource type name as it appears in Terraform configurations:

```go
func (r *agentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_agent"
}
```

This produces `livekit_agent` as the resource type (`req.ProviderTypeName` is `"livekit"`, set in the provider's own `Metadata`).

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

The `nil` check is required: Configure is called during validation when provider data is not yet available.

## Create

Contract:

- Read plan data from `req.Plan`
- Perform the API creation call
- Set ALL attribute values (including computed) in `resp.State`
- Unknown values in plan MUST become known in state (error otherwise)
- On error, the resource is marked tainted for recreation on next plan

`agentResource.Create` calls `CreateAgentV2`, then immediately re-reads the agent through `ListAgents` (`findAgent`) because `CreateAgentV2Response` does not carry every field the schema needs (`agent_name`, `version`, `deployed_at`):

```go
func (r *agentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
    var plan agentResourceModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() {
        return
    }

    var regions []string
    if !plan.Regions.IsNull() && !plan.Regions.IsUnknown() {
        resp.Diagnostics.Append(plan.Regions.ElementsAs(ctx, &regions, false)...)
        if resp.Diagnostics.HasError() {
            return
        }
    }

    created, err := r.client.agent.CreateAgentV2(ctx, &livekit.CreateAgentV2Request{Regions: regions})
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to create agent: %s", err))
        return
    }

    plan.Id = types.StringValue(created.AgentId)
    plan.AgentId = types.StringValue(created.AgentId)

    info, err := r.findAgent(ctx, created.AgentId)
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read created agent: %s", err))
        return
    }
    if info == nil {
        resp.Diagnostics.AddError("API Error", "Agent was created but could not be found immediately afterward")
        return
    }

    resp.Diagnostics.Append(applyAgentInfo(ctx, &plan, info)...)
    resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
```

Note that `CreateAgentV2` has no `Success`/`Message` fields to check; only `UpdateAgent`, `DeleteAgent`, and `UpdateAgentSecrets` return that pair (see "Checking API Success" below).

## Read

Contract:

- Read prior state from `req.State`
- Perform the API read call
- If the resource no longer exists: call `resp.State.RemoveResource(ctx)` and return
- Otherwise, update all state values to reflect current API state

The CloudAgent API has no `GetAgent` RPC, so Read (and Create/Update) all go through `ListAgents` filtered client-side by ID:

```go
func (r *agentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
    var state agentResourceModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() {
        return
    }

    info, err := r.findAgent(ctx, state.Id.ValueString())
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read agent: %s", err))
        return
    }
    if info == nil {
        resp.State.RemoveResource(ctx)
        return
    }

    state.AgentId = types.StringValue(info.AgentId)
    resp.Diagnostics.Append(applyAgentInfo(ctx, &state, info)...)
    resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *agentResource) findAgent(ctx context.Context, id string) (*livekit.AgentInfo, error) {
    listResp, err := r.client.agent.ListAgents(ctx, &livekit.ListAgentsRequest{AgentId: id})
    if err != nil {
        if isNotFound(err) {
            return nil, nil
        }
        return nil, err
    }
    for _, a := range listResp.Agents {
        if a.AgentId == id {
            return a, nil
        }
    }
    return nil, nil
}
```

`findAgent` collapses both ways an agent can be "gone" (a `twirp.NotFound` error, or a filtered list that simply does not contain the ID) into a single `nil, nil` result, so Read only has one branch to handle.

## Update

Contract:

- Read plan data from `req.Plan` (the desired new state)
- Perform the API update call
- Set state to reflect the actual post-update values
- All values in state MUST match plan values (or Terraform produces an "inconsistent result" error)

```go
func (r *agentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
    var plan agentResourceModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() {
        return
    }

    var regions []string
    if !plan.Regions.IsNull() && !plan.Regions.IsUnknown() {
        resp.Diagnostics.Append(plan.Regions.ElementsAs(ctx, &regions, false)...)
        if resp.Diagnostics.HasError() {
            return
        }
    }

    updated, err := r.client.agent.UpdateAgent(ctx, &livekit.UpdateAgentRequest{
        AgentId: plan.Id.ValueString(),
        Regions: regions,
    })
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to update agent: %s", err))
        return
    }
    if !updated.Success {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to update agent: %s", updated.Message))
        return
    }

    info, err := r.findAgent(ctx, plan.Id.ValueString())
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read updated agent: %s", err))
        return
    }
    if info == nil {
        resp.Diagnostics.AddError("API Error", "Agent was updated but could not be found afterward")
        return
    }

    resp.Diagnostics.Append(applyAgentInfo(ctx, &plan, info)...)
    resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
```

## Delete

Contract:

- Read prior state from `req.State`
- Perform the API deletion
- If already deleted: return without error (idempotent)
- No need to modify state: framework removes it automatically on success

```go
func (r *agentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
    var state agentResourceModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() {
        return
    }

    deleted, err := r.client.agent.DeleteAgent(ctx, &livekit.DeleteAgentRequest{AgentId: state.Id.ValueString()})
    if err != nil {
        if isNotFound(err) {
            return
        }
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to delete agent: %s", err))
        return
    }
    if !deleted.Success {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to delete agent: %s", deleted.Message))
        return
    }
}
```

## Checking API Success

`CreateAgentV2` and `ListAgents`/`ListAgentSecrets` return their data directly, but `UpdateAgent`, `DeleteAgent`, and `UpdateAgentSecrets` wrap a `Success bool` and `Message string` on top of the usual transport `error`. A `nil` error only means the RPC round-tripped; always check `resp.Success` too:

```go
updated, err := r.client.agent.UpdateAgent(ctx, req)
if err != nil {
    resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to update agent: %s", err))
    return
}
if !updated.Success {
    resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to update agent: %s", updated.Message))
    return
}
```

## Related Framework References

| File                                | Contents                                  |
| ------------------------------------ | -------------------------------------------- |
| `framework/resources/index.mdx`     | Resource type definition, full interface  |
| `framework/resources/create.mdx`    | Create method details and caveats         |
| `framework/resources/read.mdx`      | Read method and state refresh             |
| `framework/resources/update.mdx`    | Update method and plan consistency        |
| `framework/resources/delete.mdx`    | Delete method                             |
| `framework/resources/configure.mdx` | Configure method, provider data injection |
