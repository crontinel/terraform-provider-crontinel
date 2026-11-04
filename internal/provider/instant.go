package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/attr/xattr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// instantType is an RFC 3339 time. Two values are equal when they are the same moment, so
// 2030-12-01T22:00:00+06:00 and 2030-12-01T16:00:00+00:00 are not a change. The server
// returns times in UTC; this keeps the offset the configuration used.
type instantType struct{ basetypes.StringType }

var (
	_ basetypes.StringTypable                    = instantType{}
	_ basetypes.StringValuableWithSemanticEquals = instantValue{}
	_ xattr.ValidateableAttribute                = instantValue{}
)

func (t instantType) Equal(o attr.Type) bool {
	other, ok := o.(instantType)
	return ok && t.StringType.Equal(other.StringType)
}

func (t instantType) String() string { return "provider.instantType" }

func (t instantType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return instantValue{StringValue: in}, nil
}

func (t instantType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	v, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	sv, ok := v.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", v)
	}
	return instantValue{StringValue: sv}, nil
}

func (t instantType) ValueType(_ context.Context) attr.Value { return instantValue{} }

type instantValue struct{ basetypes.StringValue }

func (v instantValue) Equal(o attr.Value) bool {
	other, ok := o.(instantValue)
	return ok && v.StringValue.Equal(other.StringValue)
}

func (v instantValue) Type(_ context.Context) attr.Type { return instantType{} }

func (v instantValue) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	other, ok := newValuable.(instantValue)
	if !ok {
		diags.AddError("Semantic equality check error", fmt.Sprintf("Expected instantValue, got %T.", newValuable))
		return false, diags
	}
	a, errA := time.Parse(time.RFC3339, v.ValueString())
	b, errB := time.Parse(time.RFC3339, other.ValueString())
	if errA != nil || errB != nil {
		return v.ValueString() == other.ValueString(), diags
	}
	return a.Equal(b), diags
}

func (v instantValue) ValidateAttribute(_ context.Context, req xattr.ValidateAttributeRequest, resp *xattr.ValidateAttributeResponse) {
	if v.IsNull() || v.IsUnknown() {
		return
	}
	if _, err := time.Parse(time.RFC3339, v.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid time",
			fmt.Sprintf("%q is not an RFC 3339 time with an offset, for example 2030-12-01T22:00:00+00:00.", v.ValueString()))
	}
}

func instantOrNull(s *string) instantValue {
	if s == nil {
		return instantValue{StringValue: basetypes.NewStringNull()}
	}
	return instantValue{StringValue: basetypes.NewStringValue(*s)}
}
