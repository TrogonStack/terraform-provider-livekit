package provider

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

func rsId() schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The unique ID of this resource.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

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
