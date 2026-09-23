terraform {
  required_providers {
    livekit = {
      source = "trogonstack/livekit"
    }
  }
}

provider "livekit" {
  url        = "https://my-project.livekit.cloud"
  api_key    = "APIxxxxxxxx"
  api_secret = "secretxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
}
