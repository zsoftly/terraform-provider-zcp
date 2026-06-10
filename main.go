package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

var version = "dev"

// providerAddress is set at build time via -ldflags to match the target registry.
// Default is the OpenTofu registry (primary). The Terraform build overrides this
// to registry.terraform.io/zsoftly/zcp so the binary self-reports correctly for
// non-dev_overrides local installs.
var providerAddress = "registry.opentofu.org/zsoftly/zcp"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: providerAddress,
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}
