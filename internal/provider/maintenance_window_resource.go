package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &maintenanceWindowResource{}
	_ resource.ResourceWithConfigure   = &maintenanceWindowResource{}
	_ resource.ResourceWithImportState = &maintenanceWindowResource{}
)

func NewMaintenanceWindowResource() resource.Resource { return &maintenanceWindowResource{} }

type maintenanceWindowResource struct{ client *Client }

type maintenanceWindowModel struct {
	App             types.String `tfsdk:"app"`
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Type            types.String `tfsdk:"type"`
	StartsAt        instantValue `tfsdk:"starts_at"`
	EndsAt          instantValue `tfsdk:"ends_at"`
	Days            types.Set    `tfsdk:"days"`
	StartTime       types.String `tfsdk:"start_time"`
	DurationMinutes types.Int64  `tfsdk:"duration_minutes"`
	MonitorIDs      types.Set    `tfsdk:"monitor_ids"`
	Tags            types.Set    `tfsdk:"tags"`
	Version         types.String `tfsdk:"version"`
}

type maintenanceWindowAPI struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	StartsAt        *string  `json:"starts_at"`
	EndsAt          *string  `json:"ends_at"`
	Days            []int64  `json:"days"`
	StartTime       *string  `json:"start_time"`
	DurationMinutes *int64   `json:"duration_minutes"`
	MonitorIDs      []int64  `json:"monitor_ids"`
	Tags            []string `json:"tags"`
	Version         string   `json:"version"`
}

func (r *maintenanceWindowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance_window"
}

func (r *maintenanceWindowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A maintenance window: while it is open, alerts in its scope send no notifications and don't escalate. Incidents are still recorded. Changing `app` or `name` replaces the window.",
		Attributes: map[string]schema.Attribute{
			"app": schema.StringAttribute{Required: true, Description: "App slug.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"id":  schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{Required: true, Description: "The window's key (up to 100 characters).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type": schema.StringAttribute{Required: true, Description: "`once` (needs starts_at and ends_at) or `weekly` (needs days, start_time and duration_minutes).",
				Validators: []validator.String{stringvalidator.OneOf("once", "weekly")}},
			"starts_at":        schema.StringAttribute{Optional: true, CustomType: instantType{}, Description: "For `once`: start, as an RFC 3339 time with an offset, for example 2026-12-01T22:00:00+00:00. The same moment written in another offset is not a change."},
			"ends_at":          schema.StringAttribute{Optional: true, CustomType: instantType{}, Description: "For `once`: end, same format. Must be after starts_at."},
			"days":             schema.SetAttribute{Optional: true, ElementType: types.Int64Type, Description: "For `weekly`: days of the week, 0 (Sunday) to 6 (Saturday)."},
			"start_time":       schema.StringAttribute{Optional: true, Description: "For `weekly`: start as HH:MM in the app's time zone."},
			"duration_minutes": schema.Int64Attribute{Optional: true, Description: "For `weekly`: length in minutes, 5 to 10080."},
			"monitor_ids": schema.SetAttribute{Optional: true, ElementType: types.Int64Type,
				Description: "Limit the window to these monitors (for example `crontinel_cron_schedule.x.id`). Leave unset to cover the whole app."},
			"tags":    schema.SetAttribute{Optional: true, ElementType: types.StringType, Description: "Up to 10 lowercase slugs."},
			"version": schema.StringAttribute{Computed: true, Description: "Changes whenever the record changes."},
		},
	}
}

func (r *maintenanceWindowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func int64SetToList(set types.Set) []int64 {
	out := []int64{}
	if set.IsNull() || set.IsUnknown() {
		return out
	}
	for _, e := range set.Elements() {
		if v, ok := e.(types.Int64); ok {
			out = append(out, v.ValueInt64())
		}
	}
	return out
}

func int64ListToSet(prior types.Set, values []int64) types.Set {
	if len(values) == 0 {
		if !prior.IsNull() && !prior.IsUnknown() && len(prior.Elements()) == 0 {
			return prior
		}
		return types.SetNull(types.Int64Type)
	}
	elems := make([]attr.Value, len(values))
	for i, v := range values {
		elems[i] = types.Int64Value(v)
	}
	return types.SetValueMust(types.Int64Type, elems)
}

func (m *maintenanceWindowModel) apply(a *maintenanceWindowAPI) {
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Type = types.StringValue(a.Type)
	m.StartsAt = instantOrNull(a.StartsAt)
	m.EndsAt = instantOrNull(a.EndsAt)
	m.Days = int64ListToSet(m.Days, a.Days)
	m.StartTime = strOrNull(a.StartTime)
	if a.DurationMinutes != nil {
		m.DurationMinutes = types.Int64Value(*a.DurationMinutes)
	} else {
		m.DurationMinutes = types.Int64Null()
	}
	m.MonitorIDs = int64ListToSet(m.MonitorIDs, a.MonitorIDs)
	m.Tags = reconcileTags(m.Tags, a.Tags)
	m.Version = types.StringValue(a.Version)
}

func (r *maintenanceWindowResource) save(ctx context.Context, plan *maintenanceWindowModel, ifVersion string) (*maintenanceWindowAPI, error) {
	tags, _ := tagsFromSet(ctx, plan.Tags)
	body := map[string]any{"name": plan.Name.ValueString(), "type": plan.Type.ValueString(), "tags": tags,
		"monitor_ids": int64SetToList(plan.MonitorIDs)}
	if !plan.StartsAt.IsNull() && !plan.StartsAt.IsUnknown() {
		body["starts_at"] = plan.StartsAt.ValueString()
	}
	if !plan.EndsAt.IsNull() && !plan.EndsAt.IsUnknown() {
		body["ends_at"] = plan.EndsAt.ValueString()
	}
	if !plan.Days.IsNull() {
		body["days"] = int64SetToList(plan.Days)
	}
	if p := ptrOrNil(plan.StartTime); p != nil {
		body["start_time"] = *p
	}
	if !plan.DurationMinutes.IsNull() {
		body["duration_minutes"] = plan.DurationMinutes.ValueInt64()
	}
	if ifVersion != "" {
		body["if_version"] = ifVersion
	}
	var out maintenanceWindowAPI
	if _, err := r.client.Do(ctx, "PUT", appPath(plan.App.ValueString(), "maintenance-windows"), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *maintenanceWindowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan maintenanceWindowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.save(ctx, &plan, "")
	if err != nil {
		apiDiag(&resp.Diagnostics, "creating the maintenance window", err)
		return
	}
	plan.apply(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *maintenanceWindowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state maintenanceWindowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out maintenanceWindowAPI
	if _, err := r.client.Do(ctx, "GET", appPath(state.App.ValueString(), "maintenance-windows")+"/"+state.ID.ValueString(), nil, &out); err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		apiDiag(&resp.Diagnostics, "reading the maintenance window", err)
		return
	}
	state.apply(&out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *maintenanceWindowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state maintenanceWindowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.save(ctx, &plan, state.Version.ValueString())
	if err != nil {
		apiDiag(&resp.Diagnostics, "updating the maintenance window", err)
		return
	}
	plan.apply(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *maintenanceWindowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state maintenanceWindowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.Do(ctx, "DELETE", appPath(state.App.ValueString(), "maintenance-windows")+"/"+state.ID.ValueString(), nil, nil); err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.IsNotFound() {
			return
		}
		apiDiag(&resp.Diagnostics, "deleting the maintenance window", err)
	}
}

func (r *maintenanceWindowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByAppAndID(ctx, req, resp)
}
