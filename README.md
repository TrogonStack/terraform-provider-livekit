# terraform-provider-livekit

**A Terraform provider that manages LiveKit Cloud agents as code.** It covers the CloudAgent API behind a single provider configuration.

**The provider turns agent deployment into declarative resources.** An agent's deployment regions and its per-agent secrets both become Terraform resources with full create, read, update, delete, and import support. Authentication runs through a LiveKit API key and secret, which the provider uses to sign an admin-scoped JWT for every request, so no separate credential store is involved.

**It exists because agent configuration otherwise drifts between dashboard and deploy scripts.** Regions get adjusted by hand in the LiveKit Cloud dashboard, secrets get rotated ad hoc, and neither leaves the kind of record that a code review or a rollout pipeline can rely on. Expressing agents and their secrets as Terraform configuration puts those changes under review, makes them reproducible across projects, and lets agent state live alongside the rest of your infrastructure.

**It is useful to teams already running Terraform** who want their LiveKit Cloud agents deployed and rotated by the same pipeline as their other infrastructure, without hand-editing regions or secrets in the dashboard.

## Provider configuration

```hcl
provider "livekit" {
  url        = "https://my-project.livekit.cloud"
  api_key    = "APIxxxxxxxx"
  api_secret = "secretxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
}
```

| Attribute    | Environment variable | Description                                                       |
| ------------ | --------------------- | ------------------------------------------------------------------ |
| `url`        | `LIVEKIT_URL`         | The LiveKit Cloud project URL. Required.                           |
| `api_key`    | `LIVEKIT_API_KEY`     | LiveKit API key. Required.                                         |
| `api_secret` | `LIVEKIT_API_SECRET`  | LiveKit API secret, used to sign the admin JWT. Required, sensitive.|

Every attribute falls back to its environment variable when unset in configuration, and the provider fails with a diagnostic if a value is missing from both places.

## Resources

| Type                    | API        |
| ----------------------- | ---------- |
| `livekit_agent`         | CloudAgent |
| `livekit_agent_secret`  | CloudAgent |

## Example

```hcl
terraform {
  required_providers {
    livekit = {
      source = "trogonstack/livekit"
    }
  }
}

resource "livekit_agent" "voice_assistant" {
  regions = ["us-east", "eu-west"]
}

resource "livekit_agent_secret" "openai_key" {
  agent_id         = livekit_agent.voice_assistant.id
  name             = "OPENAI_API_KEY"
  value_wo         = var.openai_api_key
  value_wo_version = 1
}
```

`value_wo` is write-only: it is read from configuration on create and update, sent to LiveKit Cloud, and never stored in state. Bump `value_wo_version` whenever `value_wo` changes to tell the provider to send the new value.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup, test workflow, and release process.
