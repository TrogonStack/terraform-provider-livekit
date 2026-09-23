package provider

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/livekit/protocol/livekit"
	"github.com/twitchtv/twirp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func requireCLIVersionHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-LIVEKIT-CLI-VERSION") == "" {
			_ = twirp.WriteError(w, twirp.NewError(twirp.Malformed, "livekit-cli version header is required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func errAgentNotFound() error {
	return twirp.NewError(twirp.Internal, "failed to get agent")
}

func errDeleteAgentNotFound() error {
	return twirp.NewError(twirp.Internal, "The agent could not be found. Please check the agent ID and try again.")
}

type fakeCloudAgent struct {
	mu             sync.Mutex
	nextID         int
	undeployed     bool
	kindUnreported bool
	agents         map[string]*livekit.AgentInfo
	regions        map[string][]string
	secrets        map[string]map[string]*livekit.AgentSecret
}

func newFakeCloudAgent() *fakeCloudAgent {
	return &fakeCloudAgent{
		agents:  make(map[string]*livekit.AgentInfo),
		regions: make(map[string][]string),
		secrets: make(map[string]map[string]*livekit.AgentSecret),
	}
}

func (f *fakeCloudAgent) deployments(agentId string) []*livekit.AgentDeployment {
	if f.undeployed {
		return nil
	}
	regions := f.regions[agentId]
	deployments := make([]*livekit.AgentDeployment, 0, len(regions))
	for _, region := range regions {
		deployments = append(deployments, &livekit.AgentDeployment{
			Region:  region,
			AgentId: agentId,
			Status:  "running",
		})
	}
	return deployments
}

func (f *fakeCloudAgent) CreateAgentV2(_ context.Context, req *livekit.CreateAgentV2Request) (*livekit.CreateAgentV2Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	agentId := fmt.Sprintf("agent-%d", f.nextID)
	now := timestamppb.Now()

	f.agents[agentId] = &livekit.AgentInfo{
		AgentId:    agentId,
		AgentName:  agentId,
		Version:    "1",
		DeployedAt: now,
	}
	f.regions[agentId] = append([]string(nil), req.Regions...)

	secrets := make(map[string]*livekit.AgentSecret, len(req.Secrets))
	for _, s := range req.Secrets {
		secrets[s.Name] = &livekit.AgentSecret{
			Name:      s.Name,
			Value:     s.Value,
			Kind:      s.Kind,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	f.secrets[agentId] = secrets

	return &livekit.CreateAgentV2Response{
		AgentId:       agentId,
		Status:        "running",
		ServerRegions: req.Regions,
	}, nil
}

func (f *fakeCloudAgent) ListAgents(_ context.Context, req *livekit.ListAgentsRequest) (*livekit.ListAgentsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []*livekit.AgentInfo
	for id, info := range f.agents {
		if req.AgentId != "" && req.AgentId != id {
			continue
		}
		if req.AgentName != "" && req.AgentName != info.AgentName {
			continue
		}
		clone, _ := proto.Clone(info).(*livekit.AgentInfo)
		clone.AgentDeployments = f.deployments(id)
		out = append(out, clone)
	}
	return &livekit.ListAgentsResponse{Agents: out}, nil
}

func (f *fakeCloudAgent) UpdateAgent(_ context.Context, req *livekit.UpdateAgentRequest) (*livekit.UpdateAgentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.agents[req.AgentId]; !ok {
		return nil, twirp.NewError(twirp.NotFound, "agent not found: "+req.AgentId)
	}
	f.regions[req.AgentId] = append([]string(nil), req.Regions...)

	return &livekit.UpdateAgentResponse{Success: true}, nil
}

func (f *fakeCloudAgent) DeleteAgent(_ context.Context, req *livekit.DeleteAgentRequest) (*livekit.DeleteAgentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.agents[req.AgentId]; !ok {
		return nil, errDeleteAgentNotFound()
	}

	delete(f.agents, req.AgentId)
	delete(f.regions, req.AgentId)
	delete(f.secrets, req.AgentId)

	return &livekit.DeleteAgentResponse{Success: true}, nil
}

func (f *fakeCloudAgent) ListAgentSecrets(_ context.Context, req *livekit.ListAgentSecretsRequest) (*livekit.ListAgentSecretsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.agents[req.AgentId]; !ok {
		return nil, errAgentNotFound()
	}

	secrets := f.secrets[req.AgentId]
	out := make([]*livekit.AgentSecret, 0, len(secrets))
	for _, s := range secrets {
		clone, _ := proto.Clone(s).(*livekit.AgentSecret)
		clone.Value = nil
		if f.kindUnreported {
			clone.Kind = livekit.AgentSecretKind_AGENT_SECRET_KIND_UNKNOWN
		}
		out = append(out, clone)
	}
	return &livekit.ListAgentSecretsResponse{Secrets: out}, nil
}

func (f *fakeCloudAgent) UpdateAgentSecrets(_ context.Context, req *livekit.UpdateAgentSecretsRequest) (*livekit.UpdateAgentSecretsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.agents[req.AgentId]; !ok {
		return nil, errAgentNotFound()
	}

	secrets, ok := f.secrets[req.AgentId]
	if !ok || req.Overwrite {
		secrets = make(map[string]*livekit.AgentSecret)
	}

	now := timestamppb.Now()
	for _, s := range req.Secrets {
		createdAt := now
		if existing, ok := secrets[s.Name]; ok {
			createdAt = existing.CreatedAt
		}
		secrets[s.Name] = &livekit.AgentSecret{
			Name:      s.Name,
			Value:     s.Value,
			Kind:      s.Kind,
			CreatedAt: createdAt,
			UpdatedAt: now,
		}
	}
	for _, name := range req.Remove {
		delete(secrets, name)
	}
	f.secrets[req.AgentId] = secrets

	return &livekit.UpdateAgentSecretsResponse{Success: true}, nil
}

func (f *fakeCloudAgent) CreateAgent(context.Context, *livekit.CreateAgentRequest) (*livekit.CreateAgentResponse, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "not implemented in fake")
}

func (f *fakeCloudAgent) PromoteAgent(context.Context, *livekit.PromoteAgentRequest) (*livekit.PromoteAgentResponse, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "not implemented in fake")
}

func (f *fakeCloudAgent) ListAgentVersions(context.Context, *livekit.ListAgentVersionsRequest) (*livekit.ListAgentVersionsResponse, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "not implemented in fake")
}

func (f *fakeCloudAgent) RestartAgent(context.Context, *livekit.RestartAgentRequest) (*livekit.RestartAgentResponse, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "not implemented in fake")
}

func (f *fakeCloudAgent) DeployAgent(context.Context, *livekit.DeployAgentRequest) (*livekit.DeployAgentResponse, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "not implemented in fake")
}

func (f *fakeCloudAgent) DeployAgentV2(context.Context, *livekit.DeployAgentV2Request) (*livekit.DeployAgentV2Response, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "not implemented in fake")
}

func (f *fakeCloudAgent) RollbackAgent(context.Context, *livekit.RollbackAgentRequest) (*livekit.RollbackAgentResponse, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "not implemented in fake")
}

func (f *fakeCloudAgent) GetClientSettings(context.Context, *livekit.ClientSettingsRequest) (*livekit.ClientSettingsResponse, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "not implemented in fake")
}

func (f *fakeCloudAgent) CreatePrivateLink(context.Context, *livekit.CreatePrivateLinkRequest) (*livekit.CreatePrivateLinkResponse, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "not implemented in fake")
}

func (f *fakeCloudAgent) DestroyPrivateLink(context.Context, *livekit.DestroyPrivateLinkRequest) (*livekit.DestroyPrivateLinkResponse, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "not implemented in fake")
}

func (f *fakeCloudAgent) ListPrivateLinks(context.Context, *livekit.ListPrivateLinksRequest) (*livekit.ListPrivateLinksResponse, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "not implemented in fake")
}

func (f *fakeCloudAgent) GetPrivateLinkStatus(context.Context, *livekit.GetPrivateLinkStatusRequest) (*livekit.GetPrivateLinkStatusResponse, error) {
	return nil, twirp.NewError(twirp.Unimplemented, "not implemented in fake")
}

var _ livekit.CloudAgent = (*fakeCloudAgent)(nil)
