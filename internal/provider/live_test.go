package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

const liveProviderConfig = `
provider "livekit" {}
`

func requireLiveCredentials(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run live acceptance tests against a real LiveKit Cloud project")
	}
	for _, name := range []string{"LIVEKIT_URL", "LIVEKIT_API_KEY", "LIVEKIT_API_SECRET"} {
		if os.Getenv(name) == "" {
			t.Skipf("set %s to run live acceptance tests against a real LiveKit Cloud project", name)
		}
	}
}

func liveAgentClient(t *testing.T) *lksdk.AgentClient {
	t.Helper()
	client, err := lksdk.NewAgentClient(
		os.Getenv("LIVEKIT_URL"),
		os.Getenv("LIVEKIT_API_KEY"),
		os.Getenv("LIVEKIT_API_SECRET"),
		cliVersionHeaderOption(),
	)
	if err != nil {
		t.Fatalf("failed to build live agent client: %v", err)
	}
	return client
}

func TestLive_Agent(t *testing.T) {
	requireLiveCredentials(t)

	client := liveAgentClient(t)
	var agentId string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(_ *terraform.State) error {
			return checkLiveAgentGone(client, agentId)
		},
		Steps: []resource.TestStep{
			{
				Config: liveProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("livekit_agent.test", "id"),
					resource.TestCheckResourceAttrSet("livekit_agent.test", "agent_name"),
					resource.TestCheckResourceAttrSet("livekit_agent.test", "version"),
					resource.TestCheckResourceAttrSet("livekit_agent.test", "deployed_at"),
					resource.TestCheckResourceAttr("livekit_agent.test", "regions.#", "1"),
					resource.TestCheckTypeSetElemAttr("livekit_agent.test", "regions.*", "us-east"),
					func(s *terraform.State) error {
						agentId = s.RootModule().Resources["livekit_agent.test"].Primary.ID
						return nil
					},
				),
			},
			{
				ResourceName:      "livekit_agent.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["livekit_agent.test"].Primary.ID, nil
				},
			},
			{
				Config: liveProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east", "eu-central"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("livekit_agent.test", "regions.#", "2"),
					resource.TestCheckTypeSetElemAttr("livekit_agent.test", "regions.*", "us-east"),
					resource.TestCheckTypeSetElemAttr("livekit_agent.test", "regions.*", "eu-central"),
				),
			},
		},
	})
}

func TestLive_AgentSecret(t *testing.T) {
	requireLiveCredentials(t)

	client := liveAgentClient(t)
	var agentId string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		CheckDestroy: func(_ *terraform.State) error {
			return checkLiveAgentGone(client, agentId)
		},
		Steps: []resource.TestStep{
			{
				Config: liveProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east"]
}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "TF_LIVE_TEST_SECRET"
  kind             = "environment"
  value_wo         = "tf-live-test-v1"
  value_wo_version = 1
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("livekit_agent_secret.test", "id"),
					resource.TestCheckResourceAttr("livekit_agent_secret.test", "kind", "environment"),
					resource.TestCheckResourceAttrSet("livekit_agent_secret.test", "created_at"),
					resource.TestCheckResourceAttrSet("livekit_agent_secret.test", "updated_at"),
					func(s *terraform.State) error {
						agentId = s.RootModule().Resources["livekit_agent.test"].Primary.ID
						return nil
					},
				),
			},
			{
				Config: liveProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east"]
}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "TF_LIVE_TEST_SECRET"
  kind             = "environment"
  value_wo         = "tf-live-test-v2"
  value_wo_version = 2
}
`,
				Check: resource.TestCheckResourceAttr("livekit_agent_secret.test", "kind", "environment"),
			},
			{
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("livekit_agent_secret.test", plancheck.ResourceActionUpdate),
					},
				},
				Config: liveProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east"]
}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "TF_LIVE_TEST_SECRET"
  kind             = "file"
  value_wo         = "tf-live-test-v2"
  value_wo_version = 2
}
`,
				Check: resource.TestCheckResourceAttr("livekit_agent_secret.test", "kind", "file"),
			},
			{
				ResourceName:            "livekit_agent_secret.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"value_wo", "value_wo_version"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["livekit_agent_secret.test"].Primary.ID, nil
				},
			},
		},
	})
}

func TestLive_AgentSecret_FileKind(t *testing.T) {
	requireLiveCredentials(t)

	client := liveAgentClient(t)
	var agentId string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		CheckDestroy: func(_ *terraform.State) error {
			return checkLiveAgentGone(client, agentId)
		},
		Steps: []resource.TestStep{
			{
				Config: liveProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east"]
}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "tf-live-test-config.json"
  kind             = "file"
  value_wo         = "{}"
  value_wo_version = 1
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("livekit_agent_secret.test", "kind", "file"),
					func(s *terraform.State) error {
						agentId = s.RootModule().Resources["livekit_agent.test"].Primary.ID
						return nil
					},
				),
			},
		},
	})
}

func checkLiveAgentGone(client *lksdk.AgentClient, agentId string) error {
	if agentId == "" {
		return fmt.Errorf("no agent ID was captured to verify destruction")
	}
	resp, err := client.ListAgents(context.Background(), &livekit.ListAgentsRequest{AgentId: agentId})
	if err != nil {
		return fmt.Errorf("failed to list agents while verifying destroy of %s: %w", agentId, err)
	}
	if len(resp.Agents) != 0 {
		return fmt.Errorf("expected agent %s to be destroyed, but it still exists", agentId)
	}
	return nil
}
