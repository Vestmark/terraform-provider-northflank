// Copyright (c) Vestmark
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/vestmark-infra/tf-provider-northflank/internal/client"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = &SecretGroupResource{}
	_ resource.ResourceWithConfigure   = &SecretGroupResource{}
	_ resource.ResourceWithImportState = &SecretGroupResource{}
)

// NewSecretGroupResource returns a resource factory function for northflank_secret_group.
func NewSecretGroupResource() resource.Resource {
	return &SecretGroupResource{}
}

// SecretGroupResource manages a Northflank project secret group.
type SecretGroupResource struct {
	client *client.Client
}

// SecretGroupResourceModel is the Terraform state model for northflank_secret_group.
type SecretGroupResourceModel struct {
	// Computed identifiers
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`

	// Required config
	Name       types.String `tfsdk:"name"`
	SecretType types.String `tfsdk:"secret_type"`
	Priority   types.Int64  `tfsdk:"priority"`

	// Optional config
	Description types.String `tfsdk:"description"`
	Type        types.String `tfsdk:"type"`
	StageID     types.String `tfsdk:"stage_id"`
	Tags        types.List   `tfsdk:"tags"`

	// Access restrictions.
	Restrictions types.Object `tfsdk:"restrictions"`

	// Secret payload (all sensitive — masked in plan/apply output).
	Variables          types.Map `tfsdk:"variables"`
	Files              types.Map `tfsdk:"files"`
	DockerSecretMounts types.Map `tfsdk:"docker_secret_mounts"`
}

func (r *SecretGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret_group"
}

func (r *SecretGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Northflank project secret group. Secret groups hold environment variables, files, and docker secret mounts that are injected into project services and jobs.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-generated identifier (derived slug of `name`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the project that this secret group belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the secret group (3–100 chars; alphanumeric with hyphens or spaces between words).",
			},
			"secret_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Injection scope. One of: `environment` (Runtime), `arguments` (Build), or `environment-arguments` (Build and Runtime).",
			},
			"priority": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Merge priority (0–100). Higher value wins when multiple groups define the same key.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Human-readable description of the secret group.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Hierarchy type. One of `secret` (default) or `config` (plaintext config values).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"stage_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Environment (stage) this secret group is associated with. The Northflank GET endpoint does not return this field, so Terraform preserves the configured value across refreshes.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tags": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Tags attached to the secret group.",
			},
			"restrictions": schema.SingleNestedAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Controls which services and jobs can access this secret group.",
				Attributes: map[string]schema.Attribute{
					"restricted": schema.BoolAttribute{
						Required:            true,
						MarkdownDescription: "When `true`, only resources matching `tags` or `nf_objects` have access.",
					},
					"tags": schema.SetAttribute{
						Optional:            true,
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Restrict access to resources bearing these tags.",
					},
					"tag_match_condition": schema.StringAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Whether all tags must match (`and`) or any tag is sufficient (`or`).",
					},
					"services": schema.SetAttribute{
						Optional:            true,
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "IDs of services that may access this secret group.",
					},
					"jobs": schema.SetAttribute{
						Optional:            true,
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "IDs of jobs that may access this secret group.",
					},
				},
			},
			"variables": schema.MapAttribute{
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				ElementType:         types.StringType,
				MarkdownDescription: "Environment variable key/value pairs. Values are encrypted at rest. Keys may only contain letters, numbers, hyphens, forward slashes, and dots.",
			},
			"files": schema.MapNestedAttribute{
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Secret files, encrypted at rest. Keys must be absolute file paths (e.g. `/etc/app/config.json`).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"data": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "File contents, base64-encoded.",
						},
						"encoding": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Encoding of the file contents (e.g. `utf-8`).",
						},
					},
				},
			},
			"docker_secret_mounts": schema.MapNestedAttribute{
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Docker secret mount contents, encrypted at rest. Keys must be valid Docker secret mount identifiers.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"data": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Mount contents, base64-encoded.",
						},
						"encoding": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Encoding of the mount contents (e.g. `utf-8`).",
						},
					},
				},
			},
		},
	}
}

func (r *SecretGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return // happens during early schema validation
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData),
		)
		return
	}
	r.client = c
}

// Create provisions a new secret group and saves state.
func (r *SecretGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SecretGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := client.CreateSecretInput{
		ProjectID:  plan.ProjectID.ValueString(),
		Name:       plan.Name.ValueString(),
		SecretType: plan.SecretType.ValueString(),
		Priority:   int(plan.Priority.ValueInt64()),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		in.Description = plan.Description.ValueString()
	}
	if !plan.Type.IsNull() && !plan.Type.IsUnknown() {
		in.Type = plan.Type.ValueString()
	}
	if !plan.StageID.IsNull() && !plan.StageID.IsUnknown() {
		in.StageID = plan.StageID.ValueString()
	}
	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &in.Tags, false)...)
	}
	in.Restrictions = restrictionsToClient(ctx, plan.Restrictions, &resp.Diagnostics)
	if !plan.Variables.IsNull() && !plan.Variables.IsUnknown() {
		vars := make(map[string]string)
		resp.Diagnostics.Append(plan.Variables.ElementsAs(ctx, &vars, false)...)
		in.Variables = vars
	}
	if !plan.Files.IsNull() && !plan.Files.IsUnknown() {
		files := make(map[string]client.SecretFile)
		resp.Diagnostics.Append(plan.Files.ElementsAs(ctx, &files, false)...)
		in.Files = files
	}
	if !plan.DockerSecretMounts.IsNull() && !plan.DockerSecretMounts.IsUnknown() {
		mounts := make(map[string]client.SecretFile)
		resp.Diagnostics.Append(plan.DockerSecretMounts.ElementsAs(ctx, &mounts, false)...)
		in.DockerSecretMounts = mounts
	}
	if resp.Diagnostics.HasError() {
		return
	}

	sg, err := r.client.CreateProjectSecret(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating secret group", err.Error())
		return
	}

	secretGroupToState(ctx, sg, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes state from the API.
func (r *SecretGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SecretGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sg, err := r.client.GetProjectSecret(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx) // drift: resource deleted outside Terraform
			return
		}
		resp.Diagnostics.AddError("Error reading secret group", err.Error())
		return
	}

	secretGroupToState(ctx, sg, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies changes via PATCH.
func (r *SecretGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SecretGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := client.UpdateSecretInput{
		ProjectID: state.ProjectID.ValueString(),
		SecretID:  state.ID.ValueString(),
	}

	// Only send fields that changed.
	if !plan.Description.Equal(state.Description) {
		v := plan.Description.ValueString()
		in.Description = &v
	}
	if !plan.SecretType.Equal(state.SecretType) {
		v := plan.SecretType.ValueString()
		in.SecretType = &v
	}
	if !plan.Type.Equal(state.Type) {
		v := plan.Type.ValueString()
		in.Type = &v
	}
	if !plan.StageID.Equal(state.StageID) {
		v := plan.StageID.ValueString()
		in.StageID = &v
	}
	if !plan.Priority.Equal(state.Priority) {
		v := int(plan.Priority.ValueInt64())
		in.Priority = &v
	}
	if !plan.Tags.Equal(state.Tags) {
		var tags []string
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		in.Tags = &tags
	}
	if !plan.Restrictions.Equal(state.Restrictions) {
		in.Restrictions = restrictionsToClient(ctx, plan.Restrictions, &resp.Diagnostics)
	}
	if !plan.Variables.Equal(state.Variables) {
		vars := make(map[string]string)
		resp.Diagnostics.Append(plan.Variables.ElementsAs(ctx, &vars, false)...)
		in.Variables = &vars
	}
	if !plan.Files.Equal(state.Files) {
		files := make(map[string]client.SecretFile)
		resp.Diagnostics.Append(plan.Files.ElementsAs(ctx, &files, false)...)
		in.Files = &files
	}
	if !plan.DockerSecretMounts.Equal(state.DockerSecretMounts) {
		mounts := make(map[string]client.SecretFile)
		resp.Diagnostics.Append(plan.DockerSecretMounts.ElementsAs(ctx, &mounts, false)...)
		in.DockerSecretMounts = &mounts
	}
	if resp.Diagnostics.HasError() {
		return
	}

	sg, err := r.client.UpdateProjectSecret(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Error updating secret group", err.Error())
		return
	}

	secretGroupToState(ctx, sg, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the secret group.
func (r *SecretGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SecretGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteProjectSecret(ctx, state.ProjectID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting secret group", err.Error())
	}
}

// ImportState handles `terraform import northflank_secret_group.x project-id/secret-group-id`.
func (r *SecretGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Import ID must be in the format <project_id>/<secret_group_id>. Got: %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// secretFileAttrType is the object type for a single file/mount entry.
var secretFileAttrType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"data":     types.StringType,
		"encoding": types.StringType,
	},
}

// restrictionsAttrType is the object type for the restrictions block.
var restrictionsAttrType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"restricted":          types.BoolType,
		"tags":                types.SetType{ElemType: types.StringType},
		"tag_match_condition": types.StringType,
		"services":            types.SetType{ElemType: types.StringType},
		"jobs":                types.SetType{ElemType: types.StringType},
	},
}

// restrictionsModel is the Go struct that maps to the restrictions block.
type restrictionsModel struct {
	Restricted        types.Bool   `tfsdk:"restricted"`
	Tags              types.Set    `tfsdk:"tags"`
	TagMatchCondition types.String `tfsdk:"tag_match_condition"`
	Services          types.Set    `tfsdk:"services"`
	Jobs              types.Set    `tfsdk:"jobs"`
}

// restrictionsToClient converts a restrictionsModel to client.SecretRestrictions.
func restrictionsToClient(ctx context.Context, obj types.Object, diagnostics *diag.Diagnostics) *client.SecretRestrictions {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m restrictionsModel
	diagnostics.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if diagnostics.HasError() {
		return nil
	}
	r := &client.SecretRestrictions{
		Restricted:        m.Restricted.ValueBool(),
		TagMatchCondition: m.TagMatchCondition.ValueString(),
	}
	if !m.Tags.IsNull() && !m.Tags.IsUnknown() {
		diagnostics.Append(m.Tags.ElementsAs(ctx, &r.Tags, false)...)
	}
	if !m.Services.IsNull() && !m.Services.IsUnknown() {
		diagnostics.Append(m.Services.ElementsAs(ctx, &r.Services, false)...)
	}
	if !m.Jobs.IsNull() && !m.Jobs.IsUnknown() {
		diagnostics.Append(m.Jobs.ElementsAs(ctx, &r.Jobs, false)...)
	}
	return r
}

// restrictionsFromClient converts client.SecretRestrictions to a types.Object.
func restrictionsFromClient(ctx context.Context, r *client.SecretRestrictions, diagnostics *diag.Diagnostics) types.Object {
	if r == nil {
		return types.ObjectNull(restrictionsAttrType.AttrTypes)
	}
	tags, d := types.SetValueFrom(ctx, types.StringType, r.Tags)
	diagnostics.Append(d...)
	services, d := types.SetValueFrom(ctx, types.StringType, r.Services)
	diagnostics.Append(d...)
	jobs, d := types.SetValueFrom(ctx, types.StringType, r.Jobs)
	diagnostics.Append(d...)

	obj, d := types.ObjectValue(restrictionsAttrType.AttrTypes, map[string]attr.Value{
		"restricted":          types.BoolValue(r.Restricted),
		"tags":                tags,
		"tag_match_condition": types.StringValue(r.TagMatchCondition),
		"services":            services,
		"jobs":                jobs,
	})
	diagnostics.Append(d...)
	return obj
}

// secretGroupToState populates a SecretGroupResourceModel from a client.SecretGroup.
func secretGroupToState(ctx context.Context, sg *client.SecretGroup, m *SecretGroupResourceModel, diagnostics *diag.Diagnostics) {
	m.ID = types.StringValue(sg.ID)
	m.ProjectID = types.StringValue(sg.ProjectID)
	m.Name = types.StringValue(sg.Name)
	m.SecretType = types.StringValue(sg.SecretType)
	m.Priority = types.Int64Value(int64(sg.Priority))
	m.Description = types.StringValue(sg.Description)
	m.Type = types.StringValue(sg.Type)
	if sg.StageID != "" {
		m.StageID = types.StringValue(sg.StageID)
	}
	// If the API didn't return StageID (GET omits it), leave m.StageID untouched
	// so the prior state value is preserved.

	tags, tagsDiag := types.ListValueFrom(ctx, types.StringType, sg.Tags)
	diagnostics.Append(tagsDiag...)
	m.Tags = tags

	vars, varsDiag := types.MapValueFrom(ctx, types.StringType, sg.Variables)
	diagnostics.Append(varsDiag...)
	m.Variables = vars

	files, filesDiag := types.MapValueFrom(ctx, secretFileAttrType, sg.Files)
	diagnostics.Append(filesDiag...)
	m.Files = files

	mounts, mountsDiag := types.MapValueFrom(ctx, secretFileAttrType, sg.DockerSecretMounts)
	diagnostics.Append(mountsDiag...)
	m.DockerSecretMounts = mounts

	m.Restrictions = restrictionsFromClient(ctx, sg.Restrictions, diagnostics)
}
