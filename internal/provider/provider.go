package provider

import (
	"context"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/twitchtv/twirp"
)

// cliVersionHeaderOption sets the X-LIVEKIT-CLI-VERSION header on every CloudAgent
// request. The API rejects requests missing it with a malformed-request error,
// regardless of caller.
func cliVersionHeaderOption() lksdk.AgentClientOption {
	return lksdk.WithTwirpClientOptions(twirp.WithClientHooks(&twirp.ClientHooks{
		RequestPrepared: func(ctx context.Context, r *http.Request) (context.Context, error) {
			r.Header.Set("X-LIVEKIT-CLI-VERSION", lksdk.Version)
			return ctx, nil
		},
	}))
}

var _ provider.Provider = &livekitProvider{}

// testAPIClient is set by tests to bypass authentication and inject a mock client.
var testAPIClient *apiClient

type livekitProvider struct {
	version string
}

type livekitProviderModel struct {
	URL       types.String `tfsdk:"url"`
	APIKey    types.String `tfsdk:"api_key"`
	APISecret types.String `tfsdk:"api_secret"`
}

func (p *livekitProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "livekit"
	resp.Version = p.version
}

func (p *livekitProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manage LiveKit Cloud agents with Terraform.

The provider covers the LiveKit Cloud Agents API: creating and updating agents
and managing the secrets they deploy with.

## Authentication

Every request is signed with a JWT built from ` + "`api_key`" + ` and ` + "`api_secret`" + `,
carrying an agent admin grant. Generate a key pair from the LiveKit Cloud
dashboard under Settings > API Keys.

## Environment variables

| Attribute    | Environment variable |
| ------------ | --------------------- |
| ` + "`url`" + `         | ` + "`LIVEKIT_URL`" + `         |
| ` + "`api_key`" + `     | ` + "`LIVEKIT_API_KEY`" + `     |
| ` + "`api_secret`" + `  | ` + "`LIVEKIT_API_SECRET`" + `  |`,
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

func (p *livekitProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data livekitProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// In tests, skip authentication and use the injected mock client.
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

	apiKey := data.APIKey.ValueString()
	if apiKey == "" {
		apiKey = os.Getenv("LIVEKIT_API_KEY")
	}
	if apiKey == "" {
		resp.Diagnostics.AddError("Configuration Error", "api_key must be set, either in the provider configuration or the LIVEKIT_API_KEY environment variable")
		return
	}

	apiSecret := data.APISecret.ValueString()
	if apiSecret == "" {
		apiSecret = os.Getenv("LIVEKIT_API_SECRET")
	}
	if apiSecret == "" {
		resp.Diagnostics.AddError("Configuration Error", "api_secret must be set, either in the provider configuration or the LIVEKIT_API_SECRET environment variable")
		return
	}

	agentClient, err := lksdk.NewAgentClient(url, apiKey, apiSecret, lksdk.WithHTTPClient(newRetryableClient()), cliVersionHeaderOption())
	if err != nil {
		resp.Diagnostics.AddError("Configuration Error", "Unable to create LiveKit agent client: "+err.Error())
		return
	}

	client := &apiClient{agent: agentClient}
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *livekitProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newAgent,
		newAgentSecret,
	}
}

func (p *livekitProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &livekitProvider{
			version: version,
		}
	}
}
