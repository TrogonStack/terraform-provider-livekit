# Provider Configuration

## Overview

The provider is the top-level component that:

1. Defines its own configuration schema (auth credentials, project URL)
2. Creates the API client during Configure
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
func (p *livekitProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
    resp.TypeName = "livekit"
    resp.Version = p.version
}
```

`TypeName` becomes the prefix for all resource names (`livekit_agent`, `livekit_agent_secret`).

### Schema

Provider schema defines what goes in the `provider "livekit" {}` block:

```go
func (p *livekitProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "url": schema.StringAttribute{
                Optional:            true,
                MarkdownDescription: "The URL of the LiveKit Cloud project, e.g. `https://my-project.livekit.cloud`.",
            },
            "api_key": schema.StringAttribute{
                Optional:            true,
                MarkdownDescription: "The LiveKit API key.",
            },
            "api_secret": schema.StringAttribute{
                Optional:            true,
                Sensitive:           true,
                MarkdownDescription: "The LiveKit API secret.",
            },
        },
    }
}
```

### Configure

Configure creates the API client and makes it available to resources:

```go
func (p *livekitProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
    var data livekitProviderModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
    if resp.Diagnostics.HasError() {
        return
    }

    if testAPIClient != nil {
        resp.DataSourceData = testAPIClient
        resp.ResourceData = testAPIClient
        return
    }

    url := data.URL.ValueString()
    if url == "" {
        url = os.Getenv("LIVEKIT_URL")
    }
    if url == "" {
        resp.Diagnostics.AddError("Configuration Error", "url must be set, either in the provider configuration or the LIVEKIT_URL environment variable")
        return
    }

    // apiKey, apiSecret resolved the same way from LIVEKIT_API_KEY / LIVEKIT_API_SECRET...

    agentClient, err := lksdk.NewAgentClient(url, apiKey, apiSecret, lksdk.WithHTTPClient(newRetryableClient()))
    if err != nil {
        resp.Diagnostics.AddError("Configuration Error", "Unable to create LiveKit agent client: "+err.Error())
        return
    }

    client := &apiClient{agent: agentClient}
    resp.DataSourceData = client
    resp.ResourceData = client
}
```

### Resource/DataSource Registration

```go
func (p *livekitProvider) Resources(ctx context.Context) []func() resource.Resource {
    return []func() resource.Resource{
        newAgent,
        newAgentSecret,
    }
}

func (p *livekitProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
    return []func() datasource.DataSource{}
}
```

## Client Data Flow

```
provider.Configure()
    -> resp.ResourceData = client
    -> resp.DataSourceData = client

resource.Configure()
    -> req.ProviderData == client (same pointer)
    -> r.client = req.ProviderData.(*apiClient)

resource.Create/Read/Update/Delete()
    -> r.client.agent.CreateAgentV2(ctx, ...)
    -> r.client.agent.ListAgents(ctx, ...)
    -> r.client.agent.UpdateAgentSecrets(ctx, ...)
```

`*apiClient` is a one-field struct (`client.go`):

```go
type apiClient struct {
    agent *lksdk.AgentClient
}
```

`lksdk.AgentClient` (from `github.com/livekit/server-sdk-go/v2`) signs an agent-admin JWT from `api_key`/`api_secret` and dials the CloudAgent Twirp service at `url`.

## Environment Variable Fallbacks

Provider config values can fall back to environment variables:

```go
apiKey := data.APIKey.ValueString()
if apiKey == "" {
    apiKey = os.Getenv("LIVEKIT_API_KEY")
}
```

This provider supports:

| Attribute    | Environment variable |
| -------------- | ----------------------- |
| `url`        | `LIVEKIT_URL`         |
| `api_key`    | `LIVEKIT_API_KEY`     |
| `api_secret` | `LIVEKIT_API_SECRET`  |

The provider adds an error diagnostic if a value is missing from both configuration and environment.

## Test Bypass

Tests inject a mock client via the package-level `testAPIClient` variable:

```go
var testAPIClient *apiClient

func (p *livekitProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
    // ...
    if testAPIClient != nil {
        resp.DataSourceData = testAPIClient
        resp.ResourceData = testAPIClient
        return
    }
    // ... real authentication logic
}
```

`setupTestClient` in `provider_test.go` is what actually sets `testAPIClient`, pointed at an `httptest.Server` serving a fake `CloudAgent` implementation (see `references/guides/testing.md`).

## Provider Server (main.go)

The entry point wraps the provider into a gRPC server:

```go
package main

import (
    "context"
    "log"

    "github.com/TrogonStack/terraform-provider-livekit/internal/provider"
    "github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version string = "dev"

func main() {
    err := providerserver.Serve(
        context.Background(),
        provider.New(version),
        providerserver.ServeOpts{
            Address: "registry.terraform.io/trogonstack/livekit",
        },
    )
    if err != nil {
        log.Fatal(err)
    }
}
```

## Related Framework References

| File                                             | Contents                                    |
| ---------------------------------------------------- | ---------------------------------------------- |
| `framework/providers/index.mdx`                  | Provider interface, metadata, schema        |
| `framework/providers/validate-configuration.mdx` | Provider-level validation                   |
| `framework/provider-servers.mdx`                 | Server setup, protocol versions, debug mode |
| `framework/resources/configure.mdx`              | How resources receive provider data         |
| `framework/data-sources/configure.mdx`           | How data sources receive provider data      |
