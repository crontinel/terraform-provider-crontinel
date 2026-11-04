package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &heartbeatResource{}
	_ resource.ResourceWithConfigure   = &heartbeatResource{}
	_ resource.ResourceWithImportState = &heartbeatResource{}
)

func NewHeartbeatResource() resource.Resource { return &heartbeatResource{} }

type heartbeatResource struct{ client *Client }

type heartbeatModel struct {
	App             types.String `tfsdk:"app"`
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	IntervalMinutes types.Int64  `tfsdk:"interval_minutes"`
	GraceMinutes    types.Int64  `tfsdk:"grace_minutes"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	Tags            types.Set    `tfsdk:"tags"`
	Version         types.String `tfsdk:"version"`
}

type heartbeatAPI struct {
	ID              int64    `json:"id"`
	Name            string   `json:"name"`
	IntervalMinutes int64    `json:"interval_minutes"`
	GraceMinutes    *int64   `json:"grace_minutes"`
	Enabled         bool     `json:"enabled"`
	Tags            []string `json:"tags"`
	Version         string   `json:"version"`
}

func (r *heartbeatResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_heartbeat"
}

func (r *heartbeatResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Heartbeat monitor: alerts when no ping arrives within the interval plus grace. Changing `app` or `name` replaces the monitor.",
		Attributes: map[string]schema.Attribute{
			"app": schema.StringAttribute{Required: true, Description: "App slug.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{Required: true, Description: "The monitor's key.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"interval_minutes": schema.Int64Attribute{Required: true, Description: "Expected minutes between pings (1 to 10080)."},
			"grace_minutes": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Description: "Extra minutes before alerting (1 to 1440). Leave unset to use the server default; the value in effect is shown."},
			"enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"tags":    schema.SetAttribute{Optional: true, ElementType: types.StringType, Description: "Up to 10 lowercase slugs."},
			"version": schema.StringAttribute{Computed: true, Description: "Changes whenever the record changes."},
		},
	}
}

func (r *heartbeatResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (m *heartbeatModel) apply(a *heartbeatAPI) {
	m.ID = types.StringValue(strconv.FormatInt(a.ID, 10))
	m.Name = types.StringValue(a.Name)
	m.IntervalMinutes = types.Int64Value(a.IntervalMinutes)
	if a.GraceMinutes != nil {
		m.GraceMinutes = types.Int64Value(*a.GraceMinutes)
	} else {
		m.GraceMinutes = types.Int64Null()
	}
	m.Enabled = types.BoolValue(a.Enabled)
	m.Tags = reconcileTags(m.Tags, a.Tags)
	m.Version = types.StringValue(a.Version)
}

func (r *heartbeatResource) save(ctx context.Context, plan *heartbeatModel, ifVersion, action string) (*heartbeatAPI, error) {
	tags, diags := tagsFromSet(ctx, plan.Tags)
	if diags.HasError() {
		return nil, fmt.Errorf("invalid tags")
	}
	body := map[string]any{"name": plan.Name.ValueString(), "interval_minutes": plan.IntervalMinutes.ValueInt64(),
		"enabled": plan.Enabled.ValueBool(), "tags": tags}
	if !plan.GraceMinutes.IsNull() && !plan.GraceMinutes.IsUnknown() {
		body["grace_minutes"] = plan.GraceMinutes.ValueInt64()
	}
	if ifVersion != "" {
		body["if_version"] = ifVersion
	}
	var out heartbeatAPI
	if _, err := r.client.Do(ctx, "PUT", appPath(plan.App.ValueString(), "heartbeats"), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *heartbeatResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan heartbeatModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.save(ctx, &plan, "", "creating")
	if err != nil {
		apiDiag(&resp.Diagnostics, "creating the heartbeat", err)
		return
	}
	plan.apply(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *heartbeatResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state heartbeatModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out heartbeatAPI
	if _, err := r.client.Do(ctx, "GET", appPath(state.App.ValueString(), "heartbeats")+"/"+state.ID.ValueString(), nil, &out); err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		apiDiag(&resp.Diagnostics, "reading the heartbeat", err)
		return
	}
	state.apply(&out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *heartbeatResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state heartbeatModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.save(ctx, &plan, state.Version.ValueString(), "updating")
	if err != nil {
		apiDiag(&resp.Diagnostics, "updating the heartbeat", err)
		return
	}
	plan.apply(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *heartbeatResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state heartbeatModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.Do(ctx, "DELETE", appPath(state.App.ValueString(), "heartbeats")+"/"+state.ID.ValueString(), nil, nil); err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.IsNotFound() {
			return
		}
		apiDiag(&resp.Diagnostics, "deleting the heartbeat", err)
	}
}

func (r *heartbeatResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByAppAndID(ctx, req, resp)
}
