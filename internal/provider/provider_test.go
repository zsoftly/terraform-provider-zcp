package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/zsoftly/terraform-provider-zcp/internal/provider"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"zcp": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func TestProvider(t *testing.T) {
	_, err := testAccProtoV6ProviderFactories["zcp"]()
	if err != nil {
		t.Fatalf("unexpected error creating provider: %s", err)
	}
}
