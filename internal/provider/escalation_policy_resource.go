package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &escalationPolicyResource{}
	_ resource.ResourceWithConfigure   = &escalationPolicyResource{}
	_ resource.ResourceWithImportState = &escalationPolicyResource{}
)

func NewEscalationPolicyResource() resource.Resource { return &escalationPolicyResource{} }

type escalationPolicyResource struct{ client *Client }

type escalationStepModel struct {
	WaitMinutes types.Int64 `tfsdk:"wait_minutes"`
	ChannelIDs  types.Set   `tfsdk:"channel_ids"`
}

type escalationPolicyModel struct {
	App     types.String          `tfsdk:"app"`
	ID      types.String          `tfsdk:"id"`
	Steps   []escalationStepModel `tfsdk:"steps"`
	Version types.String          `tfsdk:"version"`
}

type escalationStepAPI struct {
	WaitMinutes int64   `json:"wait_minutes"`
	ChannelIDs  []int64 `json:"channel_ids"`
}

type escalationPolicyAPI struct {
	Steps   []escalationStepAPI `json:"steps"`
	Version string              `json:"version"`
}

func (r *escalationPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_policy"
}

func (r *escalationPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The app's escalation policy: up to 5 ordered steps. If an alert is not acknowledged or recovered, each step notifies its channels after its wait. Channels in a step are held back from the first notification until that step. One policy per app. Alert channels are not managed by this provider; pass the ids from the dashboard. Only alerts opened after the policy is first added escalate.",
		Attributes: map[string]schema.Attribute{
			"app":     schema.StringAttribute{Required: true, Description: "App slug.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"version": schema.StringAttribute{Computed: true, Description: "Changes whenever the policy changes."},
		},
		Blocks: map[string]schema.Block{
			"steps": schema.ListNestedBlock{
				Description: "Escalation steps, in order (at most 5).",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"wait_minutes": schema.Int64Attribute{Required: true, Description: "Minutes after the previous step (or the first notification), 5 to 1440."},
						"channel_ids":  schema.SetAttribute{Required: true, ElementType: types.Int64Type, Description: "Alert channel ids of this app or its organization (at least one)."},
					},
				},
			},
		},
	}
}

func (r *escalationPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (m *escalationPolicyModel) apply(a *escalationPolicyAPI) {
	m.ID = m.App
	m.Version = types.StringValue(a.Version)
	steps := make([]escalationStepModel, len(a.Steps))
	for i, s := range a.Steps {
		var prior types.Set
		if i < len(m.Steps) {
			prior = m.Steps[i].ChannelIDs
		}
		steps[i] = escalationStepModel{WaitMinutes: types.Int64Value(s.WaitMinutes), ChannelIDs: int64ListToSet(prior, s.ChannelIDs)}
	}
	m.Steps = steps
}

func (r *escalationPolicyResource) save(ctx context.Context, plan *escalationPolicyModel, ifVersion string) (*escalationPolicyAPI, error) {
	steps := make([]map[string]any, 0, len(plan.Steps))
	for _, s := range plan.Steps {
		steps = append(steps, map[string]any{"wait_minutes": s.WaitMinutes.ValueInt64(), "channel_ids": int64SetToList(s.ChannelIDs)})
	}
	body := map[string]any{"steps": steps}
	if ifVersion != "" {
		body["if_version"] = ifVersion
	}
	var out escalationPolicyAPI
	if _, err := r.client.Do(ctx, "PUT", appPath(plan.App.ValueString(), "escalation"), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *escalationPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan escalationPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.save(ctx, &plan, "")
	if err != nil {
		apiDiag(&resp.Diagnostics, "setting the escalation policy", err)
		return
	}
	plan.apply(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *escalationPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state escalationPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out escalationPolicyAPI
	if _, err := r.client.Do(ctx, "GET", appPath(state.App.ValueString(), "escalation"), nil, &out); err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		apiDiag(&resp.Diagnostics, "reading the escalation policy", err)
		return
	}
	// Removed outside Terraform: an empty policy means the resource is gone.
	if len(out.Steps) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}
	state.apply(&out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *escalationPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state escalationPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.save(ctx, &plan, state.Version.ValueString())
	if err != nil {
		apiDiag(&resp.Diagnostics, "updating the escalation policy", err)
		return
	}
	plan.apply(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *escalationPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state escalationPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"steps": []any{}}
	if _, err := r.client.Do(ctx, "PUT", appPath(state.App.ValueString(), "escalation"), body, nil); err != nil {
		apiDiag(&resp.Diagnostics, "removing the escalation policy", err)
	}
}

// ImportState takes the app slug.
func (r *escalationPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
