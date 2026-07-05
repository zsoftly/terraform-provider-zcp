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

// stringOneOfValidator enforces that a string attribute is one of a fixed set
// of (case-sensitive) values.
type stringOneOfValidator struct {
	allowed []string
}

func (v stringOneOfValidator) Description(_ context.Context) string {
	return fmt.Sprintf("must be one of: %s", strings.Join(v.allowed, ", "))
}

func (v stringOneOfValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringOneOfValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	for _, a := range v.allowed {
		if a == val {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid value",
		fmt.Sprintf("%q is not valid. Must be one of: %s.", val, strings.Join(v.allowed, ", ")),
	)
}

// int64AtLeastValidator enforces a minimum value on an Int64 attribute.
type int64AtLeastValidator struct {
	min int64
}

func (v int64AtLeastValidator) Description(_ context.Context) string {
	return fmt.Sprintf("must be at least %d", v.min)
}

func (v int64AtLeastValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v int64AtLeastValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueInt64() < v.min {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Value too small",
			fmt.Sprintf("%d is below the minimum of %d.", req.ConfigValue.ValueInt64(), v.min),
		)
	}
}
