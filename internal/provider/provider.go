package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const defaultURL = "https://app.crontinel.com"

var _ provider.Provider = &crontinelProvider{}

type crontinelProvider struct{ version string }

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &crontinelProvider{version: version} }
}

type providerModel struct {
	URL    types.String `tfsdk:"url"`
	APIKey types.String `tfsdk:"api_key"`
}

func (p *crontinelProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "crontinel"
	resp.Version = p.version
}

func (p *crontinelProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Crontinel monitors as code through the configuration API.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Optional:    true,
				Description: "Crontinel origin. Defaults to " + defaultURL + ". Can also be set with CRONTINEL_URL.",
			},
			"api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "A generated ct_mcp_ key with monitor:read and monitor:write. Can also be set with CRONTINEL_API_KEY. This is not an app ingest key.",
			},
		},
	}
}

func (p *crontinelProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// A value still unknown at plan time (for example from another resource) is filled in at apply.
	if cfg.URL.IsUnknown() || cfg.APIKey.IsUnknown() {
		return
	}
	url := os.Getenv("CRONTINEL_URL")
	if !cfg.URL.IsNull() {
		url = cfg.URL.ValueString()
	}
	if url == "" {
		url = defaultURL
	}
	key := os.Getenv("CRONTINEL_API_KEY")
	if !cfg.APIKey.IsNull() {
		key = cfg.APIKey.ValueString()
	}
	if key == "" {
		resp.Diagnostics.AddError("Missing API key",
			"Set api_key in the provider block or the CRONTINEL_API_KEY environment variable. Create a key under AI connections with monitor:read and monitor:write.")
		return
	}
	client := NewClient(url, key, p.version)
	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *crontinelProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewCronScheduleResource, NewHeartbeatResource, NewOutcomeRuleResource, NewMaintenanceWindowResource, NewEscalationPolicyResource}
}

func (p *crontinelProvider) DataSources(_ context.Context) []func() datasource.DataSource { return nil }

func (p *crontinelProvider) Functions(_ context.Context) []func() function.Function { return nil }
