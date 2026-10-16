package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/scheduler"
	"github.com/zsoftly/zcp-cli/pkg/api/volume"
)

const (
	blockStorageSchedulerService = "Block Storage"
	blockStorageSnapshotAction   = "Block Storage Snapshot"
)

var _ resource.Resource = &volumeSnapshotScheduleResource{}
var _ resource.ResourceWithConfigure = &volumeSnapshotScheduleResource{}
var _ resource.ResourceWithImportState = &volumeSnapshotScheduleResource{}
var _ resource.ResourceWithValidateConfig = &volumeSnapshotScheduleResource{}

type volumeSnapshotScheduleService interface {
	Get(context.Context, string) (*scheduler.Policy, error)
	Create(context.Context, scheduler.CreateRequest) (*scheduler.Policy, error)
	Update(context.Context, string, scheduler.UpdateRequest) (*scheduler.Policy, error)
	Delete(context.Context, string) (*scheduler.ActionResponse, error)
	Pause(context.Context, string) (*scheduler.Policy, error)
	Resume(context.Context, string) (*scheduler.Policy, error)
}

type volumeSnapshotScheduleVolumeLister interface {
	List(context.Context, string, string) ([]volume.Volume, error)
}

type volumeSnapshotScheduleResource struct {
	svc            volumeSnapshotScheduleService
	volumes        volumeSnapshotScheduleVolumeLister
	defaultProject string
}

type volumeSnapshotScheduleModel struct {
	ID          types.String `tfsdk:"id"`
	Volume      types.String `tfsdk:"volume"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Interval    types.String `tfsdk:"interval"`
	Day         types.Int64  `tfsdk:"day"`
	At          types.String `tfsdk:"at"`
	Timezone    types.String `tfsdk:"timezone"`
	Retention   types.Int64  `tfsdk:"retention"`
	Immediate   types.Bool   `tfsdk:"immediate"`
	Paused      types.Bool   `tfsdk:"paused"`
	Region      types.String `tfsdk:"region"`
	Project     types.String `tfsdk:"project"`
	Status      types.String `tfsdk:"status"`
}

func NewVolumeSnapshotScheduleResource() resource.Resource { return &volumeSnapshotScheduleResource{} }

func (r *volumeSnapshotScheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_volume_snapshot_schedule"
}

func (r *volumeSnapshotScheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceString := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{MarkdownDescription: "Manages a block storage snapshot schedule.", Attributes: map[string]schema.Attribute{
		"id":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"volume":      schema.StringAttribute{Required: true, PlanModifiers: replaceString},
		"name":        schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), requiresReplaceIfConfigured()}},
		"description": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), requiresReplaceIfConfigured()}},
		"interval":    schema.StringAttribute{Required: true},
		"day":         schema.Int64Attribute{Optional: true},
		"at":          schema.StringAttribute{Required: true},
		"timezone":    schema.StringAttribute{Required: true},
		"retention":   schema.Int64Attribute{Required: true},
		"immediate":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{requiresReplaceBoolUnlessAdopting()}},
		"paused":      schema.BoolAttribute{Optional: true, Computed: true},
		"region":      schema.StringAttribute{Required: true, PlanModifiers: replaceString},
		"project":     schema.StringAttribute{Optional: true, PlanModifiers: replaceString},
		"status":      schema.StringAttribute{Computed: true},
	}}
}

func (r *volumeSnapshotScheduleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *ProviderData, got %T.", req.ProviderData))
		return
	}
	r.svc = scheduler.NewService(pd.Client)
	r.volumes = volume.NewService(pd.Client)
	r.defaultProject = pd.DefaultProject
}

func (r *volumeSnapshotScheduleResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m volumeSnapshotScheduleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || m.Interval.IsUnknown() {
		return
	}
	if err := validateVolumeSnapshotSchedule(m); err != nil {
		resp.Diagnostics.AddError("Invalid block storage snapshot schedule", err.Error())
	}
}

func validateVolumeSnapshotSchedule(m volumeSnapshotScheduleModel) error {
	if m.Interval.IsUnknown() {
		return nil
	}
	switch m.Interval.ValueString() {
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
	if m.Interval.ValueString() == "weeklyOn" {
		if m.Day.IsNull() || (!m.Day.IsUnknown() && (m.Day.ValueInt64() < 0 || m.Day.ValueInt64() > 6)) {
			return fmt.Errorf("day must be 0 through 6 for weeklyOn")
		}
		return nil
	}
	if m.Interval.ValueString() == "monthlyOn" {
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

func (r *volumeSnapshotScheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m volumeSnapshotScheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil || r.volumes == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume_snapshot_schedule cannot be created: bearer_token is missing.")
		return
	}
	if err := validateVolumeSnapshotSchedule(m); err != nil {
		resp.Diagnostics.AddError("Invalid block storage snapshot schedule", err.Error())
		return
	}
	project := r.projectOrDefault(m.Project)
	volumes, err := r.volumes.List(ctx, m.Region.ValueString(), project)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list block storage volumes", err.Error())
		return
	}
	found := false
	for _, v := range volumes {
		if v.Slug == m.Volume.ValueString() {
			found = true
			break
		}
	}
	if !found {
		resp.Diagnostics.AddError("Block storage volume not found", fmt.Sprintf("No block storage volume with slug %q exists in region %q and project %q.", m.Volume.ValueString(), m.Region.ValueString(), project))
		return
	}
	p, err := r.svc.Create(ctx, r.createRequest(&m))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create block storage snapshot schedule", err.Error())
		return
	}
	if err := r.validatePolicy(&m, p, false); err != nil {
		p, err = r.verifyCreatedPolicy(ctx, &m, p, err)
		if err != nil {
			resp.Diagnostics.AddError("Unable to verify created block storage snapshot schedule", err.Error())
			return
		}
	}
	if !m.Paused.IsNull() && m.Paused.ValueBool() {
		created := p
		p, err = r.svc.Pause(ctx, p.ID)
		if err != nil {
			r.setModel(&m, created)
			resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
			resp.Diagnostics.AddError("Failed to pause block storage snapshot schedule", err.Error())
			return
		}
		if err := r.validatePolicy(&m, p, false); err != nil {
			r.setModel(&m, created)
			resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
			resp.Diagnostics.AddError("Block storage snapshot schedule is outside configured scope", fmt.Sprintf("Pause response for scheduler policy %q cannot be managed because %s", schedulerPolicyID(p), err))
			return
		}
	}
	r.setModel(&m, p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// verifyCreatedPolicy only adopts a policy after the service returns a complete,
// matching read-back for the ID returned by Create. A contradictory create
// response is never followed by a mutating request.
func (r *volumeSnapshotScheduleResource) verifyCreatedPolicy(ctx context.Context, m *volumeSnapshotScheduleModel, created *scheduler.Policy, validationErr error) (*scheduler.Policy, error) {
	policyID := schedulerPolicyID(created)
	if policyID == "" {
		return nil, fmt.Errorf("the create response could not be verified because it did not include a policy ID. The policy was not saved to Terraform state. %s: %w", createdPolicyRecoveryInstructions(policyID), validationErr)
	}
	if r.hasExplicitPolicyScopeMismatch(m, created) {
		return nil, fmt.Errorf("created scheduler policy %q was not saved to Terraform state because its response contradicts the configured scope: %w. %s", policyID, validationErr, createdPolicyRecoveryInstructions(policyID))
	}
	verified, err := r.svc.Get(ctx, policyID)
	if err != nil {
		return nil, fmt.Errorf("created scheduler policy %q was not saved to Terraform state because its read-back could not be verified: %w. %s", policyID, err, createdPolicyRecoveryInstructions(policyID))
	}
	if schedulerPolicyID(verified) != policyID {
		return nil, fmt.Errorf("created scheduler policy %q was not saved to Terraform state because its read-back returned policy %q. %s", policyID, schedulerPolicyID(verified), createdPolicyRecoveryInstructions(policyID))
	}
	if err := r.validatePolicy(m, verified, false); err != nil {
		return nil, fmt.Errorf("created scheduler policy %q was not saved to Terraform state because its read-back did not match the configured scope: %w. %s", policyID, err, createdPolicyRecoveryInstructions(policyID))
	}
	return verified, nil
}

func createdPolicyRecoveryInstructions(policyID string) string {
	if policyID == "" {
		return "Locate the matching newly created schedule in the dashboard. Confirm its action, volume, region, and project before acting. If it is the confirmed matching schedule, pause it to stop future runs, then either import it into matching configuration or, if it is disposable, remove its snapshots and delete the policy before retrying apply"
	}
	return fmt.Sprintf("Inspect scheduler policy %q in the dashboard. Confirm its action, volume, region, and project before acting. If it is the confirmed matching schedule, pause it to stop future runs, then either import it into matching configuration or, if it is disposable, remove its snapshots and delete the policy before retrying apply", policyID)
}

func (r *volumeSnapshotScheduleResource) hasExplicitPolicyScopeMismatch(m *volumeSnapshotScheduleModel, p *scheduler.Policy) bool {
	if p == nil {
		return false
	}
	if p.Action != "" && !strings.EqualFold(p.Action, blockStorageSnapshotAction) {
		return true
	}
	if p.Actionable != nil && p.Actionable.Slug != "" && !m.Volume.IsNull() && !m.Volume.IsUnknown() && p.Actionable.Slug != m.Volume.ValueString() {
		return true
	}
	if p.Region != nil && p.Region.Slug != "" && p.Region.Slug != m.Region.ValueString() {
		return true
	}
	expectedProject := r.projectOrDefault(m.Project)
	if expectedProject != "" && p.Project != nil && p.Project.Slug != "" && p.Project.Slug != expectedProject {
		return true
	}
	return false
}

func (r *volumeSnapshotScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m volumeSnapshotScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume_snapshot_schedule cannot be read: bearer_token is missing.")
		return
	}
	p, err := r.svc.Get(ctx, m.ID.ValueString())
	if apierrors.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read block storage snapshot schedule", err.Error())
		return
	}
	if err := r.validatePolicy(&m, p, true); err != nil {
		resp.Diagnostics.AddError("Block storage snapshot schedule is outside configured scope", err.Error())
		return
	}
	r.setModel(&m, p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *volumeSnapshotScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state volumeSnapshotScheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume_snapshot_schedule cannot be updated: bearer_token is missing.")
		return
	}
	if err := validateVolumeSnapshotSchedule(plan); err != nil {
		resp.Diagnostics.AddError("Invalid block storage snapshot schedule", err.Error())
		return
	}
	current, err := r.svc.Get(ctx, state.ID.ValueString())
	if apierrors.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read block storage snapshot schedule", err.Error())
		return
	}
	if err := r.validatePolicy(&state, current, false); err != nil {
		resp.Diagnostics.AddError("Block storage snapshot schedule is outside configured scope", err.Error())
		return
	}
	p, err := r.svc.Update(ctx, state.ID.ValueString(), r.updateRequest(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update block storage snapshot schedule", err.Error())
		return
	}
	if err := r.validatePolicy(&state, p, false); err != nil {
		resp.Diagnostics.AddError("Block storage snapshot schedule is outside configured scope", err.Error())
		return
	}
	updated := p
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
			resp.Diagnostics.AddError("Failed to change block storage snapshot schedule status", err.Error())
			return
		}
		if err := r.validatePolicy(&state, p, false); err != nil {
			r.setModel(&plan, updated)
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError("Block storage snapshot schedule is outside configured scope", fmt.Sprintf("Status response for scheduler policy %q cannot be managed because %s", schedulerPolicyID(p), err))
			return
		}
	}
	r.setModel(&plan, p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *volumeSnapshotScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m volumeSnapshotScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider not configured", "zcp_volume_snapshot_schedule cannot be deleted: bearer_token is missing.")
		return
	}
	p, err := r.svc.Get(ctx, m.ID.ValueString())
	if apierrors.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read block storage snapshot schedule", err.Error())
		return
	}
	if err := r.validatePolicy(&m, p, true); err != nil {
		resp.Diagnostics.AddError("Block storage snapshot schedule is outside configured scope", err.Error())
		return
	}
	result, err := r.svc.Delete(ctx, m.ID.ValueString())
	if err != nil && !apierrors.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete block storage snapshot schedule", err.Error())
		return
	}
	if err == nil && !strings.EqualFold(result.Status, "success") {
		resp.Diagnostics.AddError("Failed to delete block storage snapshot schedule", fmt.Sprintf("API returned status %q: %s", result.Status, result.Message))
	}
}

func (r *volumeSnapshotScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importPositional(ctx, req, resp, []string{"id", "region", "project"}, 2, "<id>/<region>[/<project>]")
}

func (r *volumeSnapshotScheduleResource) projectOrDefault(project types.String) string {
	if !project.IsNull() && !project.IsUnknown() {
		return project.ValueString()
	}
	return r.defaultProject
}

func schedulerPolicyID(policy *scheduler.Policy) string {
	if policy == nil {
		return ""
	}
	return policy.ID
}

func (r *volumeSnapshotScheduleResource) createRequest(m *volumeSnapshotScheduleModel) scheduler.CreateRequest {
	var day *int
	if !m.Day.IsNull() {
		value := int(m.Day.ValueInt64())
		day = &value
	}
	var name, description *string
	if !m.Name.IsNull() {
		value := m.Name.ValueString()
		name = &value
	}
	if !m.Description.IsNull() {
		value := m.Description.ValueString()
		description = &value
	}
	immediate := 0
	if !m.Immediate.IsNull() && m.Immediate.ValueBool() {
		immediate = 1
	}
	return scheduler.CreateRequest{Service: blockStorageSchedulerService, Slug: m.Volume.ValueString(), Action: blockStorageSnapshotAction, Name: name, Description: description, Interval: m.Interval.ValueString(), Day: day, At: m.At.ValueString(), Timezone: m.Timezone.ValueString(), RetentionPolicy: int(m.Retention.ValueInt64()), TakeOneImmediately: immediate, Config: scheduler.Config{TakeOneImmediately: immediate}}
}

func (r *volumeSnapshotScheduleResource) updateRequest(m *volumeSnapshotScheduleModel) scheduler.UpdateRequest {
	var day *int
	if !m.Day.IsNull() {
		value := int(m.Day.ValueInt64())
		day = &value
	}
	return scheduler.UpdateRequest{Interval: m.Interval.ValueString(), Day: day, At: m.At.ValueString(), Timezone: m.Timezone.ValueString(), RetentionPolicy: int(m.Retention.ValueInt64())}
}

func (r *volumeSnapshotScheduleResource) setModel(m *volumeSnapshotScheduleModel, p *scheduler.Policy) {
	m.ID, m.Name, m.Description = types.StringValue(p.ID), types.StringValue(p.Name), types.StringValue(p.Description)
	m.Interval, m.At, m.Timezone = types.StringValue(p.Interval), types.StringValue(p.At), types.StringValue(p.Timezone)
	if p.Day == nil {
		m.Day = types.Int64Null()
	} else {
		m.Day = types.Int64Value(int64(*p.Day))
	}
	m.Retention, m.Paused, m.Status = types.Int64Value(int64(p.RetentionPolicy)), types.BoolValue(strings.EqualFold(p.Status, "paused")), types.StringValue(p.Status)
	if p.Actionable != nil {
		m.Volume = types.StringValue(p.Actionable.Slug)
	}
	if p.Region != nil {
		m.Region = types.StringValue(p.Region.Slug)
	}
	if p.Project != nil && !m.Project.IsNull() {
		m.Project = types.StringValue(p.Project.Slug)
	}
}

func (r *volumeSnapshotScheduleResource) validatePolicy(m *volumeSnapshotScheduleModel, p *scheduler.Policy, allowOrphan bool) error {
	if p == nil {
		return fmt.Errorf("scheduler response did not include a policy")
	}
	if p.ID == "" {
		return fmt.Errorf("scheduler response did not include a policy ID")
	}
	if !strings.EqualFold(p.Action, blockStorageSnapshotAction) {
		return fmt.Errorf("scheduler policy %q has action %q, expected Block Storage Snapshot", p.ID, p.Action)
	}
	if p.Actionable == nil || p.Actionable.Slug == "" {
		if !allowOrphan {
			return fmt.Errorf("scheduler policy %q did not return a block storage volume", p.ID)
		}
	} else if !m.Volume.IsNull() && !m.Volume.IsUnknown() && p.Actionable.Slug != m.Volume.ValueString() {
		return fmt.Errorf("scheduler policy %q belongs to block storage volume %q, expected %q", p.ID, p.Actionable.Slug, m.Volume.ValueString())
	}
	if p.Region == nil || p.Region.Slug == "" {
		return fmt.Errorf("scheduler policy %q did not return a region", p.ID)
	}
	if p.Region.Slug != m.Region.ValueString() {
		return fmt.Errorf("scheduler policy %q is in region %q, expected %q", p.ID, p.Region.Slug, m.Region.ValueString())
	}
	expectedProject := r.projectOrDefault(m.Project)
	if expectedProject == "" {
		return nil
	}
	actual := ""
	if p.Project != nil {
		actual = p.Project.Slug
	}
	if actual != expectedProject {
		return fmt.Errorf("scheduler policy %q is in project %q, expected %q", p.ID, actual, expectedProject)
	}
	return nil
}
