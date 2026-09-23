# Data Source Lifecycle

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

- `datasource.DataSourceWithConfigure` — receive provider client
- `datasource.DataSourceWithValidateConfig` — configuration validation

## Registration

```go
func newBarDataSource() datasource.DataSource { return &barDataSource{} }

// In provider.go:
func (p *googleworkspaceProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
    return []func() datasource.DataSource{
        newBarDataSource,
    }
}
```

## Metadata

```go
func (d *barDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_bar"
}
```

## Schema

Data source schemas use `datasource/schema` package (not `resource/schema`):

```go
import "github.com/hashicorp/terraform-plugin-framework/datasource/schema"

func (d *barDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "id":   schema.StringAttribute{Computed: true},
            "name": schema.StringAttribute{Required: true},
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
func (d *barDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
- Perform API call to find the data
- If not found: add an error diagnostic (data sources must find their target)
- Set all attribute values in `resp.State`

```go
func (d *barDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
    var data barDataSourceModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
    if resp.Diagnostics.HasError() {
        return
    }

    svc, err := d.client.NewDirectoryService(ctx)
    if err != nil {
        resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create service: %s", err))
        return
    }

    // Lookup by name, page through results if needed
    name := data.Name.ValueString()
    var found *api.Item
    err = svc.Items.List(d.client.customerID).Pages(ctx, func(page *api.Items) error {
        for _, item := range page.Items {
            if item.Name == name {
                found = item
            }
        }
        return nil
    })
    if err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to list items: %s", err))
        return
    }

    if found == nil {
        resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Item %q not found", name))
        return
    }

    data.Id = types.StringValue(found.Id)
    resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
```

## Data Sources vs Resources

| Aspect           | Resource                     | Data Source          |
| ---------------- | ---------------------------- | -------------------- |
| Purpose          | Manage lifecycle (CRUD)      | Read-only lookup     |
| Methods          | Create, Read, Update, Delete | Read only            |
| Import           | Supported                    | N/A                  |
| Plan modifiers   | Yes                          | No                   |
| Defaults         | Yes                          | No                   |
| State management | Full lifecycle               | Refreshed every plan |
| Not found        | RemoveResource (drift)       | Error diagnostic     |

## Related Framework References

| File                                                | Contents                            |
| --------------------------------------------------- | ----------------------------------- |
| `framework/data-sources/index.mdx`                  | Data source interface, registration |
| `framework/data-sources/configure.mdx`              | Configure method                    |
| `framework/data-sources/validate-configuration.mdx` | Validation                          |
| `framework/data-sources/timeouts.mdx`               | Timeout support                     |
