package main

import (
	"context"
	"log"

	"github.com/TrogonStack/terraform-provider-livekit/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version string = "dev"

func main() {
	err := providerserver.Serve(
		context.Background(),
		provider.New(version),
		providerserver.ServeOpts{
			Address: "registry.terraform.io/trogonstack/livekit",
		},
	)
	if err != nil {
		log.Fatal(err)
	}
}
