package provider

import (
	lksdk "github.com/livekit/server-sdk-go/v2"
)

type apiClient struct {
	agent *lksdk.AgentClient
}
