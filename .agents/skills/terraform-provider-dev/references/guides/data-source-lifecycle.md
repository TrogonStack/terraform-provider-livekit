# Data Source Lifecycle

The provider does not define any data sources today (`DataSources()` in `provider.go` returns an empty slice). Everything below is the pattern to follow when one is added, built from the same client and lookup helpers the two resources already use.

## Interface

A data source must implement `datasource.DataSource`:

```go
type DataSource interface {
    Metadata(context.Context, MetadataRequest, *MetadataResponse)
    Schema(context.Context, SchemaRequest, *SchemaResponse)
    Read(context.Context, ReadRequest, *ReadResponse)
}
```

Optional interfaces:

- `datasource.DataSourceWithConfigure`: receive provider client
- `datasource.DataSourceWithValidateConfig`: configuration validation

## Registration

```go
func newAgentDataSource() datasource.DataSource { return &agentDataSource{} }

// In provider.go:
func (p *livekitProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
    return []func() datasource.DataSource{
        newAgentDataSource,
    }
}
```

## Metadata

```go
func (d *agentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_agent"
}
```

## Schema

Data source schemas use `datasource/schema` package (not `resource/schema`):

```go
import "github.com/hashicorp/terraform-plugin-framework/datasource/schema"

func (d *agentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "agent_id":   schema.StringAttribute{Required: true},
            "agent_name": schema.StringAttribute{Computed: true},
            "version":    schema.StringAttribute{Computed: true},
        },
    }
}
```

Key differences from resource schemas:

- No plan modifiers (no plan phase for data sources)
- No defaults (no apply phase)
- Attributes are either Required (lookup key) or Computed (returned value)
- Optional attributes serve as optional filter criteria

## Configure

Same pattern as resources:

```go
func (d *agentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
    if req.ProviderData == nil {
        return
    }
    client, ok := req.ProviderData.(*apiClient)
    if !ok {
        resp.Diagnostics.AddError("Unexpected DataSource Configure Type",
            fmt.Sprintf("Expected *apiClient, got: %T", req.ProviderData))
        return
    }
    d.client = client
}
```

## Read

Contract:

- Read configuration from `req.Config` (the user-provided lookup criteria)
- Perform the API call to find the data
- If not found: add an error diagnostic (data sources must find their target)
- Set all attribute values in `resp.State`

The CloudAgent API has no single-item get RPC; both resources look an item up by filtering `ListAgents`/`ListAgentSecrets` client-side (see `agentResource.findAgent` and `agentSecretResource.findSecret` in `resource_agent.go` / `resource_agent_secret.go`). A data source would follow the same shape, but treat "not found" as an error instead of a state removal:

```go
func (d *agentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
    var data agentDataSourceModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
    if resp.Diagnostics.HasError() {
        return
    }

    agentId := data.AgentId.ValueString()
    listResp, err := d.client.agent.ListAgents(ctx, &livekit.ListAgentsRequest{AgentId: agentId})
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to list agents: %s", err))
        return
    }

    var found *livekit.AgentInfo
    for _, a := range listResp.Agents {
        if a.AgentId == agentId {
            found = a
        }
    }
    if found == nil {
        resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Agent %q not found", agentId))
        return
    }

    data.AgentName = types.StringValue(found.AgentName)
    data.Version = types.StringValue(found.Version)
    resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
```

## Data Sources vs Resources

| Aspect           | Resource                     | Data Source          |
| ------------------ | ------------------------------- | ----------------------- |
| Purpose          | Manage lifecycle (CRUD)      | Read-only lookup     |
| Methods          | Create, Read, Update, Delete | Read only            |
| Import           | Supported                    | N/A                  |
| Plan modifiers   | Yes                          | No                   |
| Defaults         | Yes                          | No                   |
| State management | Full lifecycle               | Refreshed every plan |
| Not found        | RemoveResource (drift)       | Error diagnostic     |

## Related Framework References

| File                                                | Contents                            |
| ------------------------------------------------------ | -------------------------------------- |
| `framework/data-sources/index.mdx`                  | Data source interface, registration |
| `framework/data-sources/configure.mdx`              | Configure method                    |
| `framework/data-sources/validate-configuration.mdx` | Validation                          |
| `framework/data-sources/timeouts.mdx`               | Timeout support                     |
