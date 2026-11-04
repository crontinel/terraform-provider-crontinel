package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func clientFrom(data any, diags *diag.Diagnostics) *Client {
	if data == nil {
		return nil // provider not configured yet (validation phase)
	}
	c, ok := data.(*Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("Expected *Client, got %T.", data))
		return nil
	}
	return c
}

const staleMessage = "This record was changed in Crontinel after Terraform last read it, so the update was refused and nothing was saved. Run terraform plan again to see the difference."

// apiConflictDiag returns the explanation when an update was refused because the record changed, else "".
func apiConflictDiag(err error) string {
	if apiErr, ok := err.(*APIError); ok && apiErr.IsConflict() && strings.Contains(apiErr.Message, "changed since you read it") {
		return staleMessage
	}
	return ""
}

// apiDiag turns an API error into a diagnostic that says what to do next.
func apiDiag(diags *diag.Diagnostics, action string, err error) {
	if msg := apiConflictDiag(err); msg != "" {
		diags.AddError("Changed outside Terraform", msg)
		return
	}
	diags.AddError("Crontinel API error while "+action, err.Error())
}

func tagsFromSet(ctx context.Context, set types.Set) ([]string, diag.Diagnostics) {
	tags := []string{}
	if set.IsNull() || set.IsUnknown() {
		return tags, nil
	}
	diags := set.ElementsAs(ctx, &tags, false)
	return tags, diags
}

func tagsToSet(tags []string) types.Set {
	if len(tags) == 0 {
		return types.SetNull(types.StringType)
	}
	values := make([]attr.Value, len(tags))
	for i, t := range tags {
		values[i] = types.StringValue(t)
	}
	return types.SetValueMust(types.StringType, values)
}

// reconcileTags keeps an explicit empty set (tags = []) as empty instead of turning it into null.
func reconcileTags(prior types.Set, tags []string) types.Set {
	if len(tags) == 0 && !prior.IsNull() && !prior.IsUnknown() && len(prior.Elements()) == 0 {
		return prior
	}
	return tagsToSet(tags)
}

func strOrNull(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

func ptrOrNil(s types.String) *string {
	if s.IsNull() || s.IsUnknown() || s.ValueString() == "" {
		return nil
	}
	v := s.ValueString()
	return &v
}

// importByAppAndID accepts "<app slug>/<id>".
func importByAppAndID(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Use <app slug>/<id>, for example billing/42. Find the id in the configuration API.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
