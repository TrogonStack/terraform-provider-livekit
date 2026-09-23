package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/livekit/protocol/livekit"
)

var (
	_ resource.Resource                = &agentSecretResource{}
	_ resource.ResourceWithImportState = &agentSecretResource{}
)

func newAgentSecret() resource.Resource { return &agentSecretResource{} }

type agentSecretResource struct {
	client *apiClient
}

type agentSecretResourceModel struct {
	Id             types.String `tfsdk:"id"`
	AgentId        types.String `tfsdk:"agent_id"`
	Name           types.String `tfsdk:"name"`
	Kind           types.String `tfsdk:"kind"`
	ValueWo        types.String `tfsdk:"value_wo"`
	ValueWoVersion types.Int64  `tfsdk:"value_wo_version"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func (r *agentSecretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agent_secret"
}

func (r *agentSecretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manages a secret deployed to a LiveKit Cloud agent.

The secret value is a write-only argument (` + "`value_wo`" + `) and never lands in Terraform
state. Pair it with ` + "`value_wo_version`" + `: changing the version is what tells the
provider to send a new value. Changing ` + "`kind`" + `, ` + "`agent_id`" + `, or
` + "`name`" + ` replaces the resource.

The value is not importable. After ` + "`terraform import`" + `, set ` + "`value_wo`" + ` and
` + "`value_wo_version`" + ` in configuration and apply to synchronize it.`,
		Attributes: map[string]schema.Attribute{
			"id": rsId(),
			"agent_id": schema.StringAttribute{
				Required:      true,
				PlanModifiers: replace,
			},
			"name": schema.StringAttribute{
				Required:      true,
				PlanModifiers: replace,
			},
			"kind": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(string(secretKindEnvironment)),
				MarkdownDescription: "One of `environment` or `file`. Defaults to `environment`.",
				Validators: []validator.String{
					stringvalidator.OneOf(string(secretKindEnvironment), string(secretKindFile)),
				},
				PlanModifiers: replace,
			},
			"value_wo": schema.StringAttribute{
				Required:            true,
				WriteOnly:           true,
				Sensitive:           true,
				MarkdownDescription: "The secret value. Write-only: never persisted to state or plan.",
			},
			"value_wo_version": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "An arbitrary version number. Bump it to rotate `value_wo`.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC3339 timestamp of when the secret was created.",
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC3339 timestamp of when the secret was last updated.",
			},
		},
	}
}

func (r *agentSecretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *agentSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan agentSecretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var valueWo types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("value_wo"), &valueWo)...)
	if resp.Diagnostics.HasError() {
		return
	}

	kind, err := parseSecretKind(plan.Kind.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid Configuration", err.Error())
		return
	}

	agentId := plan.AgentId.ValueString()
	name := plan.Name.ValueString()

	updateResp, err := r.client.agent.UpdateAgentSecrets(ctx, &livekit.UpdateAgentSecretsRequest{
		AgentId:   agentId,
		Overwrite: false,
		Secrets: []*livekit.AgentSecret{
			{
				Name:  name,
				Value: []byte(valueWo.ValueString()),
				Kind:  kind.proto(),
			},
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to create agent secret: %s", err))
		return
	}
	if !updateResp.Success {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to create agent secret: %s", updateResp.Message))
		return
	}

	plan.Id = types.StringValue(agentId + "/" + name)

	secret, err := r.findSecret(ctx, agentId, name)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read created agent secret: %s", err))
		return
	}
	if secret == nil {
		resp.Diagnostics.AddError("API Error", "Agent secret was created but could not be found immediately afterward")
		return
	}

	applyAgentSecret(&plan, secret)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *agentSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state agentSecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, err := r.findSecret(ctx, state.AgentId.ValueString(), state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read agent secret: %s", err))
		return
	}
	if secret == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	applyAgentSecret(&state, secret)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *agentSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan agentSecretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var valueWo types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("value_wo"), &valueWo)...)
	if resp.Diagnostics.HasError() {
		return
	}

	kind, err := parseSecretKind(plan.Kind.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid Configuration", err.Error())
		return
	}

	agentId := plan.AgentId.ValueString()
	name := plan.Name.ValueString()

	updateResp, err := r.client.agent.UpdateAgentSecrets(ctx, &livekit.UpdateAgentSecretsRequest{
		AgentId:   agentId,
		Overwrite: false,
		Secrets: []*livekit.AgentSecret{
			{
				Name:  name,
				Value: []byte(valueWo.ValueString()),
				Kind:  kind.proto(),
			},
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to update agent secret: %s", err))
		return
	}
	if !updateResp.Success {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to update agent secret: %s", updateResp.Message))
		return
	}

	secret, err := r.findSecret(ctx, agentId, name)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read updated agent secret: %s", err))
		return
	}
	if secret == nil {
		resp.Diagnostics.AddError("API Error", "Agent secret was updated but could not be found afterward")
		return
	}

	applyAgentSecret(&plan, secret)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *agentSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state agentSecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteResp, err := r.client.agent.UpdateAgentSecrets(ctx, &livekit.UpdateAgentSecretsRequest{
		AgentId: state.AgentId.ValueString(),
		Remove:  []string{state.Name.ValueString()},
	})
	if err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to delete agent secret: %s", err))
		return
	}
	if !deleteResp.Success {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to delete agent secret: %s", deleteResp.Message))
		return
	}
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

func (r *agentSecretResource) findSecret(ctx context.Context, agentId, name string) (*livekit.AgentSecret, error) {
	listResp, err := r.client.agent.ListAgentSecrets(ctx, &livekit.ListAgentSecretsRequest{AgentId: agentId})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, s := range listResp.Secrets {
		if s.Name == name {
			return s, nil
		}
	}
	return nil, nil
}

func applyAgentSecret(model *agentSecretResourceModel, secret *livekit.AgentSecret) {
	if secret.Kind != livekit.AgentSecretKind_AGENT_SECRET_KIND_UNKNOWN || model.Kind.IsNull() {
		model.Kind = types.StringValue(string(secretKindFromProto(secret.Kind)))
	}
	if secret.CreatedAt != nil {
		model.CreatedAt = types.StringValue(secret.CreatedAt.AsTime().Format(time.RFC3339))
	} else {
		model.CreatedAt = types.StringNull()
	}
	if secret.UpdatedAt != nil {
		model.UpdatedAt = types.StringValue(secret.UpdatedAt.AsTime().Format(time.RFC3339))
	} else {
		model.UpdatedAt = types.StringNull()
	}
}
