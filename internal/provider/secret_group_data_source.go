// Copyright (c) Vestmark
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vestmark-infra/tf-provider-northflank/internal/client"
)



// Compile-time interface assertion.
var _ datasource.DataSourceWithConfigure = &SecretGroupDataSource{}

// NewSecretGroupDataSource returns a data source factory function for northflank_secret_group.
func NewSecretGroupDataSource() datasource.DataSource {
	return &SecretGroupDataSource{}
}

// SecretGroupDataSource reads an existing Northflank project secret group.
type SecretGroupDataSource struct {
	client *client.Client
}

// SecretGroupDataSourceModel is the Terraform state model for the northflank_secret_group data source.
type SecretGroupDataSourceModel struct {
	// Required lookup keys
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`

	// Read-only output attributes
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	SecretType  types.String `tfsdk:"secret_type"`
	Type        types.String `tfsdk:"type"`
	Priority    types.Int64  `tfsdk:"priority"`
	StageID            types.String `tfsdk:"stage_id"`
	Restrictions       types.Object `tfsdk:"restrictions"`
	Tags               types.List   `tfsdk:"tags"`
	Variables          types.Map    `tfsdk:"variables"`
	Files              types.Map  `tfsdk:"files"`
	DockerSecretMounts types.Map  `tfsdk:"docker_secret_mounts"`
}

func (d *SecretGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret_group"
}

func (d *SecretGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an existing Northflank project secret group. Useful for consuming secrets managed outside of this Terraform configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the secret group (the derived slug of its name).",
			},
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the project that the secret group belongs to.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Name of the secret group.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description of the secret group.",
			},
			"secret_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Injection scope. One of: `environment` (Runtime), `arguments` (Build), or `environment-arguments` (Build and Runtime).",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Hierarchy type: `secret` or `config`.",
			},
			"priority": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Merge priority (0–100).",
			},
			"stage_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Environment (stage) this secret group is associated with, if set.",
			},
			"restrictions": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Access restriction settings for this secret group.",
				Attributes: map[string]schema.Attribute{
					"restricted": schema.BoolAttribute{
						Computed:            true,
						MarkdownDescription: "Whether access is restricted to specific resources.",
					},
					"tags": schema.SetAttribute{
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Tags that resources must bear to access this secret group.",
					},
					"tag_match_condition": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Whether all tags must match (`and`) or any tag is sufficient (`or`).",
					},
					"services": schema.SetAttribute{
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "IDs of services that may access this secret group.",
					},
					"jobs": schema.SetAttribute{
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "IDs of jobs that may access this secret group.",
					},
				},
			},
			"tags": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Tags attached to the secret group.",
			},
			"variables": schema.MapAttribute{
				Computed:            true,
				Sensitive:           true,
				ElementType:         types.StringType,
				MarkdownDescription: "Decrypted environment variable key/value pairs.",
			},
			"files": schema.MapNestedAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Decrypted secret files. Keys are absolute file paths.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"data": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "File contents, base64-encoded.",
						},
						"encoding": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Encoding of the file contents (e.g. `utf-8`).",
						},
					},
				},
			},
			"docker_secret_mounts": schema.MapNestedAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Decrypted Docker secret mount contents. Keys are mount identifiers.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"data": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Mount contents, base64-encoded.",
						},
						"encoding": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Encoding of the mount contents (e.g. `utf-8`).",
						},
					},
				},
			},
		},
	}
}

func (d *SecretGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData),
		)
		return
	}
	d.client = c
}

// Read fetches the secret group and populates state.
func (d *SecretGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SecretGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sg, err := d.client.GetProjectSecret(ctx, data.ProjectID.ValueString(), data.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.Diagnostics.AddError(
				"Secret group not found",
				fmt.Sprintf("No secret group %q in project %q.", data.ID.ValueString(), data.ProjectID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError("Error reading secret group", err.Error())
		return
	}

	data.ID = types.StringValue(sg.ID)
	data.ProjectID = types.StringValue(sg.ProjectID)
	data.Name = types.StringValue(sg.Name)
	data.Description = types.StringValue(sg.Description)
	data.SecretType = types.StringValue(sg.SecretType)
	data.Type = types.StringValue(sg.Type)
	data.Priority = types.Int64Value(int64(sg.Priority))
	data.StageID = types.StringValue(sg.StageID)
	data.Restrictions = restrictionsFromClient(ctx, sg.Restrictions, &resp.Diagnostics)

	tags, _ := types.ListValueFrom(ctx, types.StringType, sg.Tags)
	data.Tags = tags

	vars, _ := types.MapValueFrom(ctx, types.StringType, sg.Variables)
	data.Variables = vars

	files, filesDiag := types.MapValueFrom(ctx, secretFileAttrType, sg.Files)
	resp.Diagnostics.Append(filesDiag...)
	data.Files = files

	mounts, mountsDiag := types.MapValueFrom(ctx, secretFileAttrType, sg.DockerSecretMounts)
	resp.Diagnostics.Append(mountsDiag...)
	data.DockerSecretMounts = mounts

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
