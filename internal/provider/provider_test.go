package provider

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"livekit": providerserver.NewProtocol6WithError(New("test")()),
}

// testProviderConfig is a minimal provider config that passes schema validation.
// The testAPIClient override bypasses all authentication, so these values are unused.
const testProviderConfig = `
provider "livekit" {
  url        = "https://test.livekit.cloud"
  api_key    = "test-key"
  api_secret = "test-secret"
}
`

func setupTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(requireCLIVersionHeader(handler))
	t.Cleanup(server.Close)
	return server
}

// setupTestClient sets the package-level testAPIClient with an agent client pointing
// at the test server. Must be called before running terraform-plugin-testing steps.
func setupTestClient(t *testing.T, server *httptest.Server) {
	t.Helper()
	t.Setenv("LK_AGENTS_URL", server.URL)
	agentClient, err := lksdk.NewAgentClient(server.URL, "test-key", "test-secret", lksdk.WithHTTPClient(server.Client()), cliVersionHeaderOption())
	if err != nil {
		t.Fatalf("failed to create agent client: %v", err)
	}
	testAPIClient = &apiClient{agent: agentClient}
	t.Cleanup(func() { testAPIClient = nil })
}

func TestRequireCLIVersionHeader(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))

	req, err := http.NewRequest(http.MethodPost, server.URL+"/twirp/livekit.CloudAgent/ListAgents", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected request without X-LIVEKIT-CLI-VERSION header to be rejected, got status %d", resp.StatusCode)
	}
}

func TestAgentGoneErrorsAreNotFound(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	ctx := t.Context()

	_, err := testAPIClient.agent.DeleteAgent(ctx, &livekit.DeleteAgentRequest{AgentId: "does-not-exist"})
	if err == nil || !isNotFound(err) {
		t.Fatalf("expected DeleteAgent for a missing agent to return a not-found error, got: %v", err)
	}

	_, err = testAPIClient.agent.UpdateAgentSecrets(ctx, &livekit.UpdateAgentSecretsRequest{
		AgentId: "does-not-exist",
		Remove:  []string{"MY_SECRET"},
	})
	if err == nil || !isNotFound(err) {
		t.Fatalf("expected UpdateAgentSecrets for a missing agent to return a not-found error, got: %v", err)
	}

	_, err = testAPIClient.agent.ListAgentSecrets(ctx, &livekit.ListAgentSecretsRequest{AgentId: "does-not-exist"})
	if err == nil || !isNotFound(err) {
		t.Fatalf("expected ListAgentSecrets for a missing agent to return a not-found error, got: %v", err)
	}
}

func TestCLIVersionHeaderSent(t *testing.T) {
	fake := newFakeCloudAgent()
	server := setupTestServer(t, livekit.NewCloudAgentServer(fake))
	setupTestClient(t, server)

	if _, err := testAPIClient.agent.ListAgents(t.Context(), &livekit.ListAgentsRequest{}); err != nil {
		t.Fatalf("expected request with the CLI version header to succeed, got: %v", err)
	}
}
