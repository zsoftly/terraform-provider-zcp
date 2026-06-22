package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/zsoftly/zcp-cli/pkg/api/plan"
)

// validPlanServiceTypes lists all service type strings accepted by the ZCP plan API.
var validPlanServiceTypes = []string{
	string(plan.ServiceVM),
	string(plan.ServiceVirtualRouter),
	string(plan.ServiceBlockStorage),
	string(plan.ServiceLoadBalancer),
	string(plan.ServiceKubernetes),
	string(plan.ServiceIPAddress),
	string(plan.ServiceVMSnapshot),
	string(plan.ServiceMyTemplate),
	string(plan.ServiceISO),
	string(plan.ServiceBackups),
}

// planServiceTypeValidator ensures the service attribute is a known ServiceType.
type planServiceTypeValidator struct{}

func (v planServiceTypeValidator) Description(_ context.Context) string {
	return fmt.Sprintf("must be one of: %s", strings.Join(validPlanServiceTypes, ", "))
}

func (v planServiceTypeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v planServiceTypeValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	for _, t := range validPlanServiceTypes {
		if t == val {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid plan service type",
		fmt.Sprintf("%q is not a valid service type. Must be one of: %s.", val, strings.Join(validPlanServiceTypes, ", ")),
	)
}
