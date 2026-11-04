package main

import (
	"context"
	"flag"
	"log"

	"github.com/crontinel/terraform-provider-crontinel/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// version is set by goreleaser.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with debugger support")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/crontinel/crontinel",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
