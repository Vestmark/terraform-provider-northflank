// Copyright (c) Vestmark
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vestmark-infra/tf-provider-northflank/internal/client"
)

// Ensure NorthflankProvider satisfies the provider.Provider interface.
var _ provider.Provider = &NorthflankProvider{}

// NorthflankProvider is the top-level provider implementation.
type NorthflankProvider struct {
	// version is set by the main package from goreleaser build metadata.
	version string
}

// NorthflankProviderModel maps the provider schema attributes to Go types.
type NorthflankProviderModel struct {
	APIToken types.String `tfsdk:"api_token"`
	BaseURL  types.String `tfsdk:"base_url"`
}

// New constructs a provider factory function, called once per Terraform execution.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &NorthflankProvider{version: version}
	}
}

func (p *NorthflankProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "northflank"
	resp.Version = p.version
}

func (p *NorthflankProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Northflank provider manages Northflank resources via the Northflank REST API.",
		Attributes: map[string]schema.Attribute{
			"api_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Northflank API token. May also be sourced from `NORTHFLANK_API_TOKEN`.",
			},
			"base_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Northflank API base URL. Defaults to `https://api.northflank.com`. May also be sourced from `NORTHFLANK_BASE_URL`.",
			},
		},
	}
}

// Configure builds the API client from provider configuration and stores it so
// that resources and data sources can retrieve it via req.ProviderData.
func (p *NorthflankProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg NorthflankProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If any attribute is unknown (e.g. sourced from a data source not yet evaluated
	// during plan), skip client construction.  Terraform calls Configure again during
	// apply with all values resolved.  Resources already handle a nil ProviderData.
	if cfg.APIToken.IsUnknown() || cfg.BaseURL.IsUnknown() {
		return
	}

	// Resolve token: explicit config > environment variable.
	token := os.Getenv("NORTHFLANK_API_TOKEN")
	if !cfg.APIToken.IsNull() {
		token = cfg.APIToken.ValueString()
	}
	baseURL := os.Getenv("NORTHFLANK_BASE_URL")
	if !cfg.BaseURL.IsNull() {
		baseURL = cfg.BaseURL.ValueString()
	}

	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Missing Northflank API Token",
			"The provider requires a Northflank API token. Set api_token in the provider block or export NORTHFLANK_API_TOKEN.",
		)
		return
	}

	c, err := client.New(baseURL, token)
	if err != nil {
		resp.Diagnostics.AddError("Failed to initialize Northflank client", err.Error())
		return
	}

	// Pass the client to both resources and data sources.
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *NorthflankProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewSecretGroupResource,
	}
}

func (p *NorthflankProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewTeamDataSource,
		NewProjectDataSource,
		NewSecretGroupDataSource,
	}
}
