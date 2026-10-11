package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/instance"
	"github.com/zsoftly/zcp-cli/pkg/api/scheduler"
)

var _ resource.Resource = &vmBackupScheduleResource{}
var _ resource.ResourceWithConfigure = &vmBackupScheduleResource{}
var _ resource.ResourceWithImportState = &vmBackupScheduleResource{}
var _ resource.ResourceWithValidateConfig = &vmBackupScheduleResource{}

type vmBackupScheduleService interface {
	Get(context.Context, string) (*scheduler.Policy, error)
	Create(context.Context, scheduler.CreateRequest) (*scheduler.Policy, error)
	Update(context.Context, string, scheduler.UpdateRequest) (*scheduler.Policy, error)
	Delete(context.Context, string) (*scheduler.ActionResponse, error)
	Pause(context.Context, string) (*scheduler.Policy, error)
	Resume(context.Context, string) (*scheduler.Policy, error)
}
type vmBackupScheduleInstanceLister interface {
	List(context.Context, string, string) ([]instance.VirtualMachine, error)
}

type vmBackupScheduleResource struct {
	svc            vmBackupScheduleService
	instances      vmBackupScheduleInstanceLister
	defaultProject string
}

type vmBackupScheduleModel struct {
	ID             types.String `tfsdk:"id"`
	VirtualMachine types.String `tfsdk:"virtual_machine"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Interval       types.String `tfsdk:"interval"`
	Day            types.Int64  `tfsdk:"day"`
	At             types.String `tfsdk:"at"`
	Timezone       types.String `tfsdk:"timezone"`
	Retention      types.Int64  `tfsdk:"retention"`
	Immediate      types.Bool   `tfsdk:"immediate"`
	Paused         types.Bool   `tfsdk:"paused"`
	Region         types.String `tfsdk:"region"`
	Project        types.String `tfsdk:"project"`
	Status         types.String `tfsdk:"status"`
}

func NewVMBackupScheduleResource() resource.Resource { return &vmBackupScheduleResource{} }
func (r *vmBackupScheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_backup_schedule"
}
func (r *vmBackupScheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceString := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{MarkdownDescription: "Manages a VM backup schedule.", Attributes: map[string]schema.Attribute{
		"id":              schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"virtual_machine": schema.StringAttribute{Required: true, PlanModifiers: replaceString},
		"name":            schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), requiresReplaceIfConfigured()}},
		"description":     schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), requiresReplaceIfConfigured()}},
		"interval":        schema.StringAttribute{Required: true},
		"day":             schema.Int64Attribute{Optional: true},
		"at":              schema.StringAttribute{Required: true},
		"timezone":        schema.StringAttribute{Required: true},
		"retention":       schema.Int64Attribute{Required: true},
		"immediate":       schema.BoolAttribute{Optional: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
		"paused":          schema.BoolAttribute{Optional: true, Computed: true},
		"region":          schema.StringAttribute{Required: true, PlanModifiers: replaceString},
		"project":         schema.StringAttribute{Optional: true, PlanModifiers: replaceString},
		"status":          schema.StringAttribute{Computed: true},
	}}
}
func (r *vmBackupScheduleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = scheduler.NewService(pd.Client)
	r.instances = instance.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}
func (r *vmBackupScheduleResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m vmBackupScheduleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || m.Interval.IsUnknown() {
		return
	}
	if err := validateVMBackupSchedule(m); err != nil {
		resp.Diagnostics.AddError("Invalid VM backup schedule", err.Error())
	}
}
func validateVMBackupSchedule(m vmBackupScheduleModel) error {
	if m.Interval.IsUnknown() {
		return nil
	}
	interval := m.Interval.ValueString()
	switch interval {
	case "hourly", "dailyAt", "weeklyOn", "monthlyOn":
	default:
		return fmt.Errorf("interval must be hourly, dailyAt, weeklyOn, or monthlyOn")
	}
	if !m.Timezone.IsUnknown() && (m.Timezone.IsNull() || strings.TrimSpace(m.Timezone.ValueString()) == "") {
		return fmt.Errorf("timezone is required")
	}
	if !m.Timezone.IsUnknown() && m.Timezone.ValueString() == "Local" {
		return fmt.Errorf("timezone must be an IANA timezone, not Local")
	}
	if !m.Timezone.IsUnknown() {
		if _, err := time.LoadLocation(m.Timezone.ValueString()); err != nil {
			return fmt.Errorf("timezone must be a valid IANA timezone: %w", err)
		}
	}
	if m.Retention.IsNull() || (!m.Retention.IsUnknown() && m.Retention.ValueInt64() < 1) {
		return fmt.Errorf("retention must be at least 1")
	}
	if !m.At.IsUnknown() && (m.At.IsNull() || func() bool {
		_, err := time.Parse("15:04", m.At.ValueString())
		return err != nil || !strings.HasSuffix(m.At.ValueString(), ":00")
	}()) {
		return fmt.Errorf("at must be a whole hour in HH:00 format")
	}
	if interval == "weeklyOn" {
		if m.Day.IsNull() || (!m.Day.IsUnknown() && (m.Day.ValueInt64() < 0 || m.Day.ValueInt64() > 6)) {
			return fmt.Errorf("day must be 0 through 6 for weeklyOn")
		}
		return nil
	}
	if interval == "monthlyOn" {
		if m.Day.IsNull() || (!m.Day.IsUnknown() && (m.Day.ValueInt64() < 1 || m.Day.ValueInt64() > 28)) {
			return fmt.Errorf("day must be 1 through 28 for monthlyOn")
		}
		return nil
	}
	if !m.Day.IsNull() && !m.Day.IsUnknown() {
		return fmt.Errorf("day is valid only with weeklyOn or monthlyOn")
	}
	return nil
}
func (r *vmBackupScheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m vmBackupScheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vm_backup_schedule cannot be created: bearer_token is missing.")
		return
	}
	if err := validateVMBackupSchedule(m); err != nil {
		resp.Diagnostics.AddError("Invalid VM backup schedule", err.Error())
		return
	}
	if r.instances == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vm_backup_schedule cannot be created: bearer_token is missing.")
		return
	}
	project := r.defaultProject
	if !m.Project.IsNull() && !m.Project.IsUnknown() {
		project = m.Project.ValueString()
	}
	vms, err := r.instances.List(ctx, m.Region.ValueString(), project)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list virtual machines", err.Error())
		return
	}
	found := false
	for _, vm := range vms {
		if vm.Slug == m.VirtualMachine.ValueString() {
			found = true
			break
		}
	}
	if !found {
		resp.Diagnostics.AddError("Virtual machine not found", fmt.Sprintf("No virtual machine with slug %q exists in region %q and project %q.", m.VirtualMachine.ValueString(), m.Region.ValueString(), project))
		return
	}
	var day *int
	if !m.Day.IsNull() {
		v := int(m.Day.ValueInt64())
		day = &v
	}
	var name, desc *string
	if !m.Name.IsNull() {
		v := m.Name.ValueString()
		name = &v
	}
	if !m.Description.IsNull() {
		v := m.Description.ValueString()
		desc = &v
	}
	immediate := 0
	if !m.Immediate.IsNull() && m.Immediate.ValueBool() {
		immediate = 1
	}
	p, err := r.svc.Create(ctx, scheduler.CreateRequest{Service: "Virtual Machine", Slug: m.VirtualMachine.ValueString(), Action: "VM Backup", Name: name, Description: desc, Interval: m.Interval.ValueString(), Day: day, At: m.At.ValueString(), Timezone: m.Timezone.ValueString(), RetentionPolicy: int(m.Retention.ValueInt64()), TakeOneImmediately: immediate, Config: scheduler.Config{TakeOneImmediately: immediate}})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create VM backup schedule", err.Error())
		return
	}
	if err := r.validatePolicy(&m, p, false); err != nil {
		r.setModel(&m, p)
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		resp.Diagnostics.AddError("VM backup schedule is outside configured scope", err.Error())
		return
	}
	if !m.Paused.IsNull() && m.Paused.ValueBool() {
		created := p
		p, err = r.svc.Pause(ctx, p.ID)
		if err != nil {
			r.setModel(&m, created)
			resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
			resp.Diagnostics.AddError("Failed to pause VM backup schedule", err.Error())
			return
		}
	}
	r.setModel(&m, p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *vmBackupScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m vmBackupScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vm_backup_schedule cannot be read: bearer_token is missing.")
		return
	}
	p, err := r.svc.Get(ctx, m.ID.ValueString())
	if apierrors.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read VM backup schedule", err.Error())
		return
	}
	if err := r.validatePolicy(&m, p, true); err != nil {
		resp.Diagnostics.AddError("VM backup schedule is outside configured scope", err.Error())
		return
	}
	r.setModel(&m, p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *vmBackupScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan vmBackupScheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var state vmBackupScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vm_backup_schedule cannot be updated: bearer_token is missing.")
		return
	}
	if err := validateVMBackupSchedule(plan); err != nil {
		resp.Diagnostics.AddError("Invalid VM backup schedule", err.Error())
		return
	}
	current, err := r.svc.Get(ctx, state.ID.ValueString())
	if apierrors.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read VM backup schedule", err.Error())
		return
	}
	if err := r.validatePolicy(&state, current, false); err != nil {
		resp.Diagnostics.AddError("VM backup schedule is outside configured scope", err.Error())
		return
	}
	var day *int
	if !plan.Day.IsNull() {
		v := int(plan.Day.ValueInt64())
		day = &v
	}
	p, err := r.svc.Update(ctx, state.ID.ValueString(), scheduler.UpdateRequest{Interval: plan.Interval.ValueString(), Day: day, At: plan.At.ValueString(), Timezone: plan.Timezone.ValueString(), RetentionPolicy: int(plan.Retention.ValueInt64())})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update VM backup schedule", err.Error())
		return
	}
	if err := r.validatePolicy(&state, p, false); err != nil {
		resp.Diagnostics.AddError("VM backup schedule is outside configured scope", err.Error())
		return
	}
	isPaused := strings.EqualFold(p.Status, "paused")
	wantPaused := isPaused
	if !plan.Paused.IsNull() && !plan.Paused.IsUnknown() {
		wantPaused = plan.Paused.ValueBool()
	}
	if wantPaused != isPaused {
		if wantPaused {
			p, err = r.svc.Pause(ctx, p.ID)
		} else {
			p, err = r.svc.Resume(ctx, p.ID)
		}
		if err != nil {
			resp.Diagnostics.AddError("Failed to change VM backup schedule status", err.Error())
			return
		}
	}
	r.setModel(&plan, p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *vmBackupScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m vmBackupScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_vm_backup_schedule cannot be deleted: bearer_token is missing.")
		return
	}
	p, err := r.svc.Get(ctx, m.ID.ValueString())
	if apierrors.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read VM backup schedule", err.Error())
		return
	}
	if err := r.validatePolicy(&m, p, true); err != nil {
		resp.Diagnostics.AddError("VM backup schedule is outside configured scope", err.Error())
		return
	}
	result, err := r.svc.Delete(ctx, m.ID.ValueString())
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete VM backup schedule", err.Error())
		return
	}
	if err == nil && !strings.EqualFold(result.Status, "success") {
		resp.Diagnostics.AddError("Failed to delete VM backup schedule", fmt.Sprintf("API returned status %q: %s", result.Status, result.Message))
	}
}
func (r *vmBackupScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importPositional(ctx, req, resp, []string{"id", "region", "project"}, 2, "<id>/<region>[/<project>]")
}
func (r *vmBackupScheduleResource) setModel(m *vmBackupScheduleModel, p *scheduler.Policy) {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.Description = types.StringValue(p.Description)
	m.Interval = types.StringValue(p.Interval)
	if p.Day == nil {
		m.Day = types.Int64Null()
	} else {
		m.Day = types.Int64Value(int64(*p.Day))
	}
	m.At = types.StringValue(p.At)
	m.Timezone = types.StringValue(p.Timezone)
	m.Retention = types.Int64Value(int64(p.RetentionPolicy))
	m.Paused = types.BoolValue(strings.EqualFold(p.Status, "paused"))
	m.Status = types.StringValue(p.Status)
	if p.Actionable != nil {
		m.VirtualMachine = types.StringValue(p.Actionable.Slug)
	}
	if p.Region != nil {
		m.Region = types.StringValue(p.Region.Slug)
	}
	if p.Project != nil && !m.Project.IsNull() {
		m.Project = types.StringValue(p.Project.Slug)
	}
}

func (r *vmBackupScheduleResource) ensureScope(m *vmBackupScheduleModel, p *scheduler.Policy, allowOrphan bool) error {
	if p == nil {
		return fmt.Errorf("scheduler response did not include a policy")
	}
	if p.ID == "" {
		return fmt.Errorf("scheduler response did not include a policy ID")
	}
	if !strings.EqualFold(p.Action, "Virtual Machine Backup") && !strings.EqualFold(p.Action, "VM Backup") {
		return fmt.Errorf("scheduler policy %q has action %q, expected Virtual Machine Backup", p.ID, p.Action)
	}
	if p.Actionable == nil || p.Actionable.Slug == "" {
		if !allowOrphan {
			return fmt.Errorf("scheduler policy %q did not return a virtual machine", p.ID)
		}
	} else if !m.VirtualMachine.IsNull() && !m.VirtualMachine.IsUnknown() && p.Actionable.Slug != m.VirtualMachine.ValueString() {
		return fmt.Errorf("scheduler policy %q belongs to virtual machine %q, expected %q", p.ID, p.Actionable.Slug, m.VirtualMachine.ValueString())
	}
	if p.Region == nil || p.Region.Slug == "" {
		return fmt.Errorf("scheduler policy %q did not return a region", p.ID)
	}
	if p.Region.Slug != m.Region.ValueString() {
		return fmt.Errorf("scheduler policy %q is in region %q, expected %q", p.ID, p.Region.Slug, m.Region.ValueString())
	}
	expectedProject := r.defaultProject
	if !m.Project.IsNull() && !m.Project.IsUnknown() {
		expectedProject = m.Project.ValueString()
	}
	if expectedProject == "" {
		return nil
	}
	if p.Project == nil || p.Project.Slug != expectedProject {
		actual := ""
		if p.Project != nil {
			actual = p.Project.Slug
		}
		return fmt.Errorf("scheduler policy %q is in project %q, expected %q", p.ID, actual, expectedProject)
	}
	return nil
}

func (r *vmBackupScheduleResource) validatePolicy(m *vmBackupScheduleModel, p *scheduler.Policy, allowOrphan bool) error {
	return r.ensureScope(m, p, allowOrphan)
}
