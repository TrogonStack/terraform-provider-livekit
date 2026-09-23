package provider

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/livekit/protocol/livekit"
)

var (
	_ resource.Resource                = &agentResource{}
	_ resource.ResourceWithImportState = &agentResource{}
)

func newAgent() resource.Resource { return &agentResource{} }

type agentResource struct {
	client *apiClient
}

type agentResourceModel struct {
	Id         types.String `tfsdk:"id"`
	AgentId    types.String `tfsdk:"agent_id"`
	Regions    types.Set    `tfsdk:"regions"`
	AgentName  types.String `tfsdk:"agent_name"`
	Version    types.String `tfsdk:"version"`
	DeployedAt types.String `tfsdk:"deployed_at"`
}

func (r *agentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agent"
}

func (r *agentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a LiveKit Cloud agent.\n\nSecrets are managed separately with the `livekit_agent_secret` resource.",
		Attributes: map[string]schema.Attribute{
			"id": rsId(),
			"agent_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The agent ID assigned by LiveKit Cloud. Identical to `id`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"regions": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "The regions the agent is deployed to. Leave unset to let LiveKit Cloud pick a " +
					"default region. Until the agent has a deployment, the configured value is kept as is.",
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"agent_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The agent name. Not settable through the API.",
			},
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The currently deployed agent version.",
			},
			"deployed_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC3339 timestamp of the last deployment.",
			},
		},
	}
}

func (r *agentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *agentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("agent_id"), req.ID)...)
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

func applyAgentInfo(ctx context.Context, model *agentResourceModel, info *livekit.AgentInfo) diag.Diagnostics {
	var diags diag.Diagnostics

	model.AgentName = types.StringValue(info.AgentName)
	model.Version = types.StringValue(info.Version)
	if info.DeployedAt != nil {
		model.DeployedAt = types.StringValue(info.DeployedAt.AsTime().Format(time.RFC3339))
	} else {
		model.DeployedAt = types.StringNull()
	}

	regionSet := make(map[string]struct{}, len(info.AgentDeployments))
	for _, d := range info.AgentDeployments {
		if d.Region != "" {
			regionSet[d.Region] = struct{}{}
		}
	}
	regions := make([]string, 0, len(regionSet))
	for region := range regionSet {
		regions = append(regions, region)
	}
	sort.Strings(regions)

	switch {
	case len(regions) > 0:
		set, d := types.SetValueFrom(ctx, types.StringType, regions)
		diags.Append(d...)
		model.Regions = set
	case model.Regions.IsUnknown():
		model.Regions = types.SetValueMust(types.StringType, []attr.Value{})
	}

	return diags
}
