package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &outcomeRuleResource{}
	_ resource.ResourceWithConfigure   = &outcomeRuleResource{}
	_ resource.ResourceWithImportState = &outcomeRuleResource{}
)

func NewOutcomeRuleResource() resource.Resource { return &outcomeRuleResource{} }

type outcomeRuleResource struct{ client *Client }

type outcomeRuleModel struct {
	App         types.String  `tfsdk:"app"`
	ID          types.String  `tfsdk:"id"`
	Command     types.String  `tfsdk:"command"`
	JobName     types.String  `tfsdk:"job_name"`
	Environment types.String  `tfsdk:"environment"`
	Expression  types.String  `tfsdk:"expression"`
	Key         types.String  `tfsdk:"key"`
	Kind        types.String  `tfsdk:"kind"`
	Threshold   types.Float64 `tfsdk:"threshold"`
	Missing     types.String  `tfsdk:"missing"`
	Version     types.String  `tfsdk:"version"`
}

type outcomeRuleAPI struct {
	ID          string  `json:"id"`
	Command     string  `json:"command"`
	JobName     *string `json:"job_name"`
	Environment *string `json:"environment"`
	Expression  *string `json:"expression"`
	Key         string  `json:"key"`
	Kind        string  `json:"kind"`
	Threshold   float64 `json:"threshold"`
	Missing     *string `json:"missing"`
	Version     string  `json:"version"`
}

func (r *outcomeRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_outcome_rule"
}

func (r *outcomeRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "What a run must report to count as done, for example `processed_records` at least 1. A run that exits 0 but reports less fails. `app`, `command`, `job_name`, `environment`, `expression`, `key` and `kind` identify the rule; changing one replaces it. Only `threshold` and `missing` update in place.",
		Attributes: map[string]schema.Attribute{
			"app":         schema.StringAttribute{Required: true, Description: "App slug.", PlanModifiers: replace},
			"id":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"command":     schema.StringAttribute{Required: true, Description: "The command the run reports.", PlanModifiers: replace},
			"job_name":    schema.StringAttribute{Optional: true, Description: "Match on the receipt's job_name as well.", PlanModifiers: replace},
			"environment": schema.StringAttribute{Optional: true, Description: "Match on the receipt's environment as well.", PlanModifiers: replace},
			"expression":  schema.StringAttribute{Optional: true, Description: "Match on the schedule expression as well.", PlanModifiers: replace},
			"key":         schema.StringAttribute{Required: true, Description: "Metric or timestamp name in the run receipt, for example processed_records.", PlanModifiers: replace},
			"kind": schema.StringAttribute{Required: true, Description: "numeric_min, numeric_max or freshness.", PlanModifiers: replace,
				Validators: []validator.String{stringvalidator.OneOf("numeric_min", "numeric_max", "freshness")}},
			"threshold": schema.Float64Attribute{Required: true, Description: "The bound, or the maximum age in seconds for freshness (0 to 31536000)."},
			"missing": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("unknown"),
				Description: "When a run reports no value: unknown (its own alert, never a success) or fail.",
				Validators:  []validator.String{stringvalidator.OneOf("unknown", "fail")}},
			"version": schema.StringAttribute{Computed: true, Description: "Changes whenever the record changes."},
		},
	}
}

func (r *outcomeRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (m *outcomeRuleModel) apply(a *outcomeRuleAPI) {
	m.ID = types.StringValue(a.ID)
	m.Command = types.StringValue(a.Command)
	m.JobName = strOrNull(a.JobName)
	m.Environment = strOrNull(a.Environment)
	m.Expression = strOrNull(a.Expression)
	m.Key = types.StringValue(a.Key)
	m.Kind = types.StringValue(a.Kind)
	m.Threshold = types.Float64Value(a.Threshold)
	if a.Missing != nil {
		m.Missing = types.StringValue(*a.Missing)
	} else {
		m.Missing = types.StringValue("unknown")
	}
	m.Version = types.StringValue(a.Version)
}

func (r *outcomeRuleResource) save(ctx context.Context, plan *outcomeRuleModel, ifVersion string) (*outcomeRuleAPI, error) {
	body := map[string]any{"command": plan.Command.ValueString(), "key": plan.Key.ValueString(), "kind": plan.Kind.ValueString(),
		"threshold": plan.Threshold.ValueFloat64(), "missing": plan.Missing.ValueString()}
	for name, v := range map[string]types.String{"job_name": plan.JobName, "environment": plan.Environment, "expression": plan.Expression} {
		if p := ptrOrNil(v); p != nil {
			body[name] = *p
		}
	}
	if ifVersion != "" {
		body["if_version"] = ifVersion
	}
	var out outcomeRuleAPI
	if _, err := r.client.Do(ctx, "PUT", appPath(plan.App.ValueString(), "outcome-rules"), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *outcomeRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan outcomeRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.save(ctx, &plan, "")
	if err != nil {
		apiDiag(&resp.Diagnostics, "creating the outcome rule", err)
		return
	}
	plan.apply(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *outcomeRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state outcomeRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out outcomeRuleAPI
	if _, err := r.client.Do(ctx, "GET", appPath(state.App.ValueString(), "outcome-rules")+"/"+state.ID.ValueString(), nil, &out); err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		apiDiag(&resp.Diagnostics, "reading the outcome rule", err)
		return
	}
	state.apply(&out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *outcomeRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state outcomeRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.save(ctx, &plan, state.Version.ValueString())
	if err != nil {
		apiDiag(&resp.Diagnostics, "updating the outcome rule", err)
		return
	}
	plan.apply(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *outcomeRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state outcomeRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.Do(ctx, "DELETE", appPath(state.App.ValueString(), "outcome-rules")+"/"+state.ID.ValueString(), nil, nil); err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.IsNotFound() {
			return
		}
		apiDiag(&resp.Diagnostics, "deleting the outcome rule", err)
	}
}

func (r *outcomeRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByAppAndID(ctx, req, resp)
}
