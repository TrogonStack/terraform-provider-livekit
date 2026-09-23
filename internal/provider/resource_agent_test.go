package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/livekit/protocol/livekit"
)

func TestAccAgent_Basic(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east", "us-west"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("livekit_agent.test", "id"),
					resource.TestCheckResourceAttrSet("livekit_agent.test", "agent_id"),
					resource.TestCheckResourceAttrSet("livekit_agent.test", "agent_name"),
					resource.TestCheckResourceAttrSet("livekit_agent.test", "version"),
					resource.TestCheckResourceAttrSet("livekit_agent.test", "deployed_at"),
					resource.TestCheckResourceAttr("livekit_agent.test", "regions.#", "2"),
					resource.TestCheckTypeSetElemAttr("livekit_agent.test", "regions.*", "us-east"),
					resource.TestCheckTypeSetElemAttr("livekit_agent.test", "regions.*", "us-west"),
				),
			},
		},
	})
}

func TestAccAgent_UpdateRegions(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east"]
}
`,
				Check: resource.TestCheckResourceAttr("livekit_agent.test", "regions.#", "1"),
			},
			{
				Config: testProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east", "eu-west"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("livekit_agent.test", "regions.#", "2"),
					resource.TestCheckTypeSetElemAttr("livekit_agent.test", "regions.*", "eu-west"),
				),
			},
		},
	})
}

func TestAccAgent_Import(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east"]
}
`,
			},
			{
				ResourceName:      "livekit_agent.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["livekit_agent.test"].Primary.ID, nil
				},
			},
		},
	})
}

func TestAccAgent_DeleteNotFound(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	config := testProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east"]
}
`

	var agentId string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					agentId = s.RootModule().Resources["livekit_agent.test"].Primary.ID
					return nil
				},
			},
			{
				PreConfig: func() {
					fake.mu.Lock()
					delete(fake.agents, agentId)
					delete(fake.regions, agentId)
					delete(fake.secrets, agentId)
					fake.mu.Unlock()
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccAgent_RegionsKeptBeforeFirstDeployment(t *testing.T) {
	fake := newFakeCloudAgent()
	fake.undeployed = true
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig + `
resource "livekit_agent" "test" {
  regions = ["us-east"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("livekit_agent.test", "regions.#", "1"),
					resource.TestCheckTypeSetElemAttr("livekit_agent.test", "regions.*", "us-east"),
				),
			},
		},
	})
}
