resource "livekit_agent_secret" "openai_key" {
  agent_id         = livekit_agent.voice_assistant.id
  name             = "OPENAI_API_KEY"
  value_wo         = var.openai_api_key
  value_wo_version = 1
}
