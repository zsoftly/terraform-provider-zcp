package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func validateServiceType(val string) validator.StringResponse {
	v := planServiceTypeValidator{}
	req := validator.StringRequest{
		ConfigValue: types.StringValue(val),
		Path:        path.Root("service"),
	}
	var resp validator.StringResponse
	v.ValidateString(context.Background(), req, &resp)
	return resp
}

func TestPlanServiceTypeValidator_knownValues(t *testing.T) {
	for _, svc := range validPlanServiceTypes {
		resp := validateServiceType(svc)
		if resp.Diagnostics.HasError() {
			t.Errorf("valid service %q rejected: %s", svc, resp.Diagnostics.Errors()[0].Detail())
		}
	}
}

func TestPlanServiceTypeValidator_invalidValue(t *testing.T) {
	resp := validateServiceType("NotAService")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for unknown service type, got none")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Invalid plan service type" {
		t.Errorf("unexpected summary: %q", got)
	}
}

func TestPlanServiceTypeValidator_nullIsSkipped(t *testing.T) {
	v := planServiceTypeValidator{}
	req := validator.StringRequest{
		ConfigValue: types.StringNull(),
		Path:        path.Root("service"),
	}
	var resp validator.StringResponse
	v.ValidateString(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("null value should be a no-op, got: %v", resp.Diagnostics)
	}
}
