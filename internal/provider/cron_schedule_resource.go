package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &cronScheduleResource{}
	_ resource.ResourceWithConfigure   = &cronScheduleResource{}
	_ resource.ResourceWithImportState = &cronScheduleResource{}
)

func NewCronScheduleResource() resource.Resource { return &cronScheduleResource{} }

type cronScheduleResource struct{ client *Client }

type cronScheduleModel struct {
	App               types.String `tfsdk:"app"`
	ID                types.String `tfsdk:"id"`
	Command           types.String `tfsdk:"command"`
	Expression        types.String `tfsdk:"expression"`
	Timezone          types.String `tfsdk:"timezone"`
	GraceSeconds      types.Int64  `tfsdk:"grace_seconds"`
	MaxRuntimeSeconds types.Int64  `tfsdk:"max_runtime_seconds"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	Tags              types.Set    `tfsdk:"tags"`
	PingURL           types.String `tfsdk:"ping_url"`
	Version           types.String `tfsdk:"version"`
}

type cronScheduleAPI struct {
	ID                int64    `json:"id"`
	Command           string   `json:"command"`
	Expression        string   `json:"expression"`
	Timezone          string   `json:"timezone"`
	GraceSeconds      int64    `json:"grace_seconds"`
	MaxRuntimeSeconds int64    `json:"max_runtime_seconds"`
	Enabled           bool     `json:"enabled"`
	Tags              []string `json:"tags"`
	PingURL           string   `json:"ping_url"`
	Version           string   `json:"version"`
}

func (r *cronScheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cron_schedule"
}

func (r *cronScheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Expected schedule for a job that checks in to Crontinel. Alerts when a run does not start on time, runs too long, or never finishes. Changing `app` or `command` replaces the monitor and restarts its checking.",
		Attributes: map[string]schema.Attribute{
			"app": schema.StringAttribute{Required: true, Description: "App slug, as in the dashboard URL and the configuration API path.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"command": schema.StringAttribute{Required: true, Description: "The exact command your runs report. This is the monitor's key.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"expression": schema.StringAttribute{Required: true, Description: "Five-field cron expression."},
			"timezone":   schema.StringAttribute{Required: true, Description: "IANA time zone the expression runs in, for example UTC or Europe/Berlin."},
			"grace_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(300),
				Description: "How late a start may be before it alerts (0 to 86400). Default 300."},
			"max_runtime_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(3600),
				Description: "How long a run may stay open before it alerts as unfinished (60 to 604800). Default 3600."},
			"enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"tags":    schema.SetAttribute{Optional: true, ElementType: types.StringType, Description: "Up to 10 lowercase slugs (letters, digits, - and _)."},
			"ping_url": schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description: "Heartbeat URL for this schedule. A credential, shown by the API only when the monitor is created."},
			"version": schema.StringAttribute{Computed: true, Description: "Changes whenever the record changes. Used to refuse updates over edits made outside Terraform."},
		},
	}
}

func (r *cronScheduleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (m *cronScheduleModel) body(ctx context.Context, ifVersion string) (map[string]any, error) {
	tags, diags := tagsFromSet(ctx, m.Tags)
	if diags.HasError() {
		return nil, fmt.Errorf("invalid tags")
	}
	body := map[string]any{
		"command": m.Command.ValueString(), "expression": m.Expression.ValueString(), "timezone": m.Timezone.ValueString(),
		"grace_seconds": m.GraceSeconds.ValueInt64(), "max_runtime_seconds": m.MaxRuntimeSeconds.ValueInt64(),
		"enabled": m.Enabled.ValueBool(), "tags": tags,
	}
	if ifVersion != "" {
		body["if_version"] = ifVersion
	}
	return body, nil
}

func (m *cronScheduleModel) apply(a *cronScheduleAPI) {
	m.ID = types.StringValue(strconv.FormatInt(a.ID, 10))
	m.Command = types.StringValue(a.Command)
	m.Expression = types.StringValue(a.Expression)
	m.Timezone = types.StringValue(a.Timezone)
	m.GraceSeconds = types.Int64Value(a.GraceSeconds)
	m.MaxRuntimeSeconds = types.Int64Value(a.MaxRuntimeSeconds)
	m.Enabled = types.BoolValue(a.Enabled)
	m.Tags = reconcileTags(m.Tags, a.Tags)
	m.Version = types.StringValue(a.Version)
	if a.PingURL != "" {
		m.PingURL = types.StringValue(a.PingURL)
	}
}

func (r *cronScheduleResource) save(ctx context.Context, plan *cronScheduleModel, ifVersion string, diagsOut interface{ AddError(string, string) }, action string) bool {
	body, err := plan.body(ctx, ifVersion)
	if err != nil {
		diagsOut.AddError("Invalid configuration", err.Error())
		return false
	}
	var out cronScheduleAPI
	if _, err := r.client.Do(ctx, "PUT", appPath(plan.App.ValueString(), "cron-schedules"), body, &out); err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.IsConflict() && !strings.Contains(apiErr.Message, "changed since you read it") {
			diagsOut.AddError("A schedule for this command already exists", apiErr.Error()+". Import it with terraform import, or remove it first.")
			return false
		}
		if d := apiConflictDiag(err); d != "" {
			diagsOut.AddError("Changed outside Terraform", d)
			return false
		}
		diagsOut.AddError("Crontinel API error while "+action, err.Error())
		return false
	}
	plan.apply(&out)
	return true
}

func (r *cronScheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cronScheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.PingURL = types.StringNull()
	if !r.save(ctx, &plan, "", &resp.Diagnostics, "creating the cron schedule") {
		return
	}
	if plan.PingURL.IsNull() || plan.PingURL.IsUnknown() {
		plan.PingURL = types.StringNull() // the monitor already existed; the API shows the URL only at creation
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *cronScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cronScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out cronScheduleAPI
	_, err := r.client.Do(ctx, "GET", appPath(state.App.ValueString(), "cron-schedules")+"/"+state.ID.ValueString(), nil, &out)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		apiDiag(&resp.Diagnostics, "reading the cron schedule", err)
		return
	}
	state.apply(&out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *cronScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state cronScheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.PingURL = state.PingURL
	if !r.save(ctx, &plan, state.Version.ValueString(), &resp.Diagnostics, "updating the cron schedule") {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *cronScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cronScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.Do(ctx, "DELETE", appPath(state.App.ValueString(), "cron-schedules")+"/"+state.ID.ValueString(), nil, nil)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.IsNotFound() {
			return
		}
		apiDiag(&resp.Diagnostics, "deleting the cron schedule", err)
	}
}

func (r *cronScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByAppAndID(ctx, req, resp)
}
