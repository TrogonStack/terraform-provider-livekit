package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
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

// setupTestServer creates an httptest.Server with the given handler and registers
// cleanup. Returns the server.
func setupTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// setupTestClient sets the package-level testAPIClient with an agent client pointing
// at the test server. Must be called before running terraform-plugin-testing steps.
func setupTestClient(t *testing.T, server *httptest.Server) {
	t.Helper()
	t.Setenv("LK_AGENTS_URL", server.URL)
	agentClient, err := lksdk.NewAgentClient(server.URL, "test-key", "test-secret", lksdk.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatalf("failed to create agent client: %v", err)
	}
	testAPIClient = &apiClient{agent: agentClient}
	t.Cleanup(func() { testAPIClient = nil })
}
