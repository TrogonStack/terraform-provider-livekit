# Provider Configuration

## Overview

The provider is the top-level component that:

1. Defines its own configuration schema (auth credentials, project settings)
2. Creates API clients during Configure
3. Passes client data to resources and data sources
4. Registers all available resources and data sources

## Provider Interface

```go
type Provider interface {
    Metadata(context.Context, MetadataRequest, *MetadataResponse)
    Schema(context.Context, SchemaRequest, *SchemaResponse)
    Configure(context.Context, ConfigureRequest, *ConfigureResponse)
    Resources(context.Context) []func() resource.Resource
    DataSources(context.Context) []func() datasource.DataSource
}
```

## This Provider's Structure

### Metadata

```go
func (p *googleworkspaceProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
    resp.TypeName = "googleworkspace"
    resp.Version = p.version
}
```

`TypeName` becomes the prefix for all resource names (`googleworkspace_drive`, `googleworkspace_user`, etc.).

### Schema

Provider schema defines what goes in the `provider "googleworkspace" {}` block:

```go
func (p *googleworkspaceProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "access_token":            schema.StringAttribute{Optional: true, Sensitive: true},
            "service_account":         schema.StringAttribute{Optional: true},
            "impersonated_user_email": schema.StringAttribute{Optional: true},
            "customer_id":             schema.StringAttribute{Optional: true},
            "oauth_scopes":            schema.ListAttribute{Optional: true, ElementType: types.StringType},
            "retry_on":               schema.ListAttribute{Optional: true, ElementType: types.Int64Type},
        },
    }
}
```

### Configure

Configure creates the API client and makes it available to resources:

```go
func (p *googleworkspaceProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
    var data googleworkspaceProviderModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
    if resp.Diagnostics.HasError() {
        return
    }

    // Build client...
    client := &apiClient{
        client:     httpClient,
        customerID: customerID,
    }

    // Make client available to resources and data sources
    resp.DataSourceData = client
    resp.ResourceData = client
}
```

### Resource/DataSource Registration

```go
func (p *googleworkspaceProvider) Resources(ctx context.Context) []func() resource.Resource {
    return []func() resource.Resource{
        newDrive,
        newDrivePermission,
        newOrgUnit,
        newGroup,
        newGroupMembers,
        newGroupSettings,
        newRoleAssignment,
        newUser,
    }
}

func (p *googleworkspaceProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
    return []func() datasource.DataSource{
        newRoleDataSource,
    }
}
```

## Client Data Flow

```
provider.Configure()
    → resp.ResourceData = client
    → resp.DataSourceData = client

resource.Configure()
    → req.ProviderData == client (same pointer)
    → r.client = req.ProviderData.(*apiClient)

resource.Create/Read/Update/Delete()
    → r.client.NewDriveService(ctx)
    → r.client.NewDirectoryService(ctx)
```

## Environment Variable Fallbacks

Provider config values can fall back to environment variables:

```go
serviceAccount := data.ServiceAccount.ValueString()
if serviceAccount == "" {
    serviceAccount = os.Getenv("SERVICE_ACCOUNT")
}
```

This provider supports:

- `SERVICE_ACCOUNT` — service account email
- `SUBJECT` — impersonated user email
- `GOOGLEWORKSPACE_CUSTOMER_ID` — customer ID

## Test Bypass

Tests inject a mock client via the package-level `testAPIClient` variable:

```go
var testAPIClient *apiClient

func (p *googleworkspaceProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
    // ...
    if testAPIClient != nil {
        resp.DataSourceData = testAPIClient
        resp.ResourceData = testAPIClient
        return
    }
    // ... real authentication logic
}
```

## Provider Server (main.go)

The entry point wraps the provider into a gRPC server:

```go
package main

import (
    "context"
    "flag"
    "log"

    "github.com/hashicorp/terraform-plugin-framework/providerserver"
    "github.com/way-platform/terraform-provider-googleworkspace/internal/provider"
)

var version = "dev"

func main() {
    var debug bool
    flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers")
    flag.Parse()

    opts := providerserver.ServeOpts{
        Address: "registry.terraform.io/way-platform/googleworkspace",
        Debug:   debug,
    }

    err := providerserver.Serve(context.Background(), provider.New(version), opts)
    if err != nil {
        log.Fatal(err.Error())
    }
}
```

## Related Framework References

| File                                             | Contents                                    |
| ------------------------------------------------ | ------------------------------------------- |
| `framework/providers/index.mdx`                  | Provider interface, metadata, schema        |
| `framework/providers/validate-configuration.mdx` | Provider-level validation                   |
| `framework/provider-servers.mdx`                 | Server setup, protocol versions, debug mode |
| `framework/resources/configure.mdx`              | How resources receive provider data         |
| `framework/data-sources/configure.mdx`           | How data sources receive provider data      |
