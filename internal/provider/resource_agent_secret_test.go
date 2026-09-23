package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/livekit/protocol/livekit"
)

func TestAccAgentSecret_Basic(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig + `
resource "livekit_agent" "test" {}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "MY_SECRET"
  value_wo         = "top-secret"
  value_wo_version = 1
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("livekit_agent_secret.test", "id"),
					resource.TestCheckResourceAttr("livekit_agent_secret.test", "name", "MY_SECRET"),
					resource.TestCheckResourceAttr("livekit_agent_secret.test", "kind", "environment"),
					resource.TestCheckResourceAttrSet("livekit_agent_secret.test", "created_at"),
					resource.TestCheckResourceAttrSet("livekit_agent_secret.test", "updated_at"),
					func(s *terraform.State) error {
						agentId := s.RootModule().Resources["livekit_agent.test"].Primary.ID
						fake.mu.Lock()
						defer fake.mu.Unlock()
						secret, ok := fake.secrets[agentId]["MY_SECRET"]
						if !ok {
							t.Fatalf("expected secret MY_SECRET to exist for agent %s", agentId)
						}
						if string(secret.Value) != "top-secret" {
							t.Fatalf("expected secret value %q, got %q", "top-secret", string(secret.Value))
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccAgentSecret_Rotate(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	checkValue := func(want string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			agentId := s.RootModule().Resources["livekit_agent.test"].Primary.ID
			fake.mu.Lock()
			defer fake.mu.Unlock()
			secret, ok := fake.secrets[agentId]["MY_SECRET"]
			if !ok {
				t.Fatalf("expected secret MY_SECRET to exist for agent %s", agentId)
			}
			if string(secret.Value) != want {
				t.Fatalf("expected secret value %q, got %q", want, string(secret.Value))
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig + `
resource "livekit_agent" "test" {}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "MY_SECRET"
  value_wo         = "version-one"
  value_wo_version = 1
}
`,
				Check: checkValue("version-one"),
			},
			{
				Config: testProviderConfig + `
resource "livekit_agent" "test" {}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "MY_SECRET"
  value_wo         = "version-two"
  value_wo_version = 2
}
`,
				Check: checkValue("version-two"),
			},
		},
	})
}

func TestAccAgentSecret_Import(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig + `
resource "livekit_agent" "test" {}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "MY_SECRET"
  value_wo         = "top-secret"
  value_wo_version = 1
}
`,
			},
			{
				ResourceName:            "livekit_agent_secret.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"value_wo_version"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["livekit_agent_secret.test"].Primary.ID, nil
				},
			},
		},
	})
}

func TestAccAgentSecret_DeleteNotFound(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	config := testProviderConfig + `
resource "livekit_agent" "test" {}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "MY_SECRET"
  value_wo         = "top-secret"
  value_wo_version = 1
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
					delete(fake.secrets[agentId], "MY_SECRET")
					fake.mu.Unlock()
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccAgentSecret_KindChangeInPlace(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig + `
resource "livekit_agent" "test" {}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "MY_SECRET"
  kind             = "environment"
  value_wo         = "top-secret"
  value_wo_version = 1
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
				Config: testProviderConfig + `
resource "livekit_agent" "test" {}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "MY_SECRET"
  kind             = "file"
  value_wo         = "top-secret"
  value_wo_version = 1
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("livekit_agent_secret.test", "kind", "file"),
					func(s *terraform.State) error {
						agentId := s.RootModule().Resources["livekit_agent.test"].Primary.ID
						fake.mu.Lock()
						defer fake.mu.Unlock()
						secret, ok := fake.secrets[agentId]["MY_SECRET"]
						if !ok {
							t.Fatalf("expected secret MY_SECRET to still exist for agent %s", agentId)
						}
						if secret.Kind != livekit.AgentSecretKind_AGENT_SECRET_KIND_FILE {
							t.Fatalf("expected secret kind to be updated to file, got %v", secret.Kind)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccAgentSecret_ReadRemovesResourceWhenAgentGone(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	config := testProviderConfig + `
resource "livekit_agent" "test" {}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "MY_SECRET"
  value_wo         = "top-secret"
  value_wo_version = 1
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

func TestAccAgentSecret_FileKindNotReportedByAPI(t *testing.T) {
	fake := newFakeCloudAgent()
	fake.kindUnreported = true
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig + `
resource "livekit_agent" "test" {}

resource "livekit_agent_secret" "test" {
  agent_id         = livekit_agent.test.id
  name             = "config.json"
  kind             = "file"
  value_wo         = "{}"
  value_wo_version = 1
}
`,
				Check: resource.TestCheckResourceAttr("livekit_agent_secret.test", "kind", "file"),
			},
		},
	})
}
