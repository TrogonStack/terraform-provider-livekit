package provider

import (
	"fmt"

	"github.com/livekit/protocol/livekit"
)

type secretKind string

const (
	secretKindEnvironment secretKind = "environment"
	secretKindFile        secretKind = "file"
)

func (k secretKind) proto() livekit.AgentSecretKind {
	if k == secretKindFile {
		return livekit.AgentSecretKind_AGENT_SECRET_KIND_FILE
	}
	return livekit.AgentSecretKind_AGENT_SECRET_KIND_ENVIRONMENT
}

func secretKindFromProto(k livekit.AgentSecretKind) secretKind {
	if k == livekit.AgentSecretKind_AGENT_SECRET_KIND_FILE {
		return secretKindFile
	}
	return secretKindEnvironment
}

func parseSecretKind(s string) (secretKind, error) {
	switch secretKind(s) {
	case secretKindEnvironment, "":
		return secretKindEnvironment, nil
	case secretKindFile:
		return secretKindFile, nil
	default:
		return "", fmt.Errorf("unknown secret kind %q, must be one of: environment, file", s)
	}
}
