package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// timeoutsNull returns a correctly-typed null value for the "timeouts" block
// in the given schema. Use this in test value maps whenever a resource has a
// timeouts block and you want to represent "no timeouts configured".
func timeoutsNull(t *testing.T, schResp resource.SchemaResponse) tftypes.Value {
	t.Helper()
	tfType := schResp.Schema.Type().TerraformType(context.Background())
	obj, ok := tfType.(tftypes.Object)
	if !ok {
		t.Fatalf("schema root type is not tftypes.Object, got %T", tfType)
	}
	timeoutsType, ok := obj.AttributeTypes["timeouts"]
	if !ok {
		t.Fatal("schema has no 'timeouts' block — was it added to the resource schema?")
	}
	return tftypes.NewValue(timeoutsType, nil)
}
