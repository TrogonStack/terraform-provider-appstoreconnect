package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &betaGroupResourceImpl{}
	_ resource.ResourceWithImportState    = &betaGroupResourceImpl{}
	_ resource.ResourceWithValidateConfig = &betaGroupResourceImpl{}
)

func newBetaGroup() resource.Resource { return &betaGroupResourceImpl{} }

type betaGroupResourceImpl struct {
	client *apiClient
}

type betaGroupResourceModel struct {
	Id                                   types.String `tfsdk:"id"`
	AppId                                types.String `tfsdk:"app_id"`
	Name                                 types.String `tfsdk:"name"`
	IsInternalGroup                      types.Bool   `tfsdk:"is_internal_group"`
	HasAccessToAllBuilds                 types.Bool   `tfsdk:"has_access_to_all_builds"`
	PublicLinkEnabled                    types.Bool   `tfsdk:"public_link_enabled"`
	PublicLinkLimitEnabled               types.Bool   `tfsdk:"public_link_limit_enabled"`
	PublicLinkLimit                      types.Int64  `tfsdk:"public_link_limit"`
	FeedbackEnabled                      types.Bool   `tfsdk:"feedback_enabled"`
	IosBuildsAvailableForAppleSiliconMac types.Bool   `tfsdk:"ios_builds_available_for_apple_silicon_mac"`
	IosBuildsAvailableForAppleVision     types.Bool   `tfsdk:"ios_builds_available_for_apple_vision"`
	PublicLink                           types.String `tfsdk:"public_link"`
	PublicLinkId                         types.String `tfsdk:"public_link_id"`
	CreatedDate                          types.String `tfsdk:"created_date"`
}

func (r *betaGroupResourceImpl) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_beta_group"
}

func (r *betaGroupResourceImpl) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepBool := []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}
	replaceBool := []planmodifier.Bool{boolplanmodifier.UseStateForUnknown(), boolplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manages a TestFlight beta group for an app.

Changing ` + "`app_id`" + `, ` + "`is_internal_group`" + ` or ` + "`has_access_to_all_builds`" + ` replaces the group, because App Store Connect only accepts them when the group is created. Optional settings left unset keep whatever App Store Connect assigns.

Destroying or replacing a group is destructive for its testers. Deleting the group removes every tester's membership in it, and a replacement group gets a new public link, so every join link already shared stops working. Consider ` + "`lifecycle { prevent_destroy = true }`" + ` on groups whose link has been shared.

Internal groups cannot have a public link, so ` + "`public_link_enabled`" + `, ` + "`public_link_limit_enabled`" + ` and ` + "`public_link_limit`" + ` are rejected at plan time when ` + "`is_internal_group`" + ` is true.`,
		Attributes: map[string]schema.Attribute{
			"id": rsId(),
			"app_id": schema.StringAttribute{
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				MarkdownDescription: "The App Store Connect ID of the app the group belongs to.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
				MarkdownDescription: "The name of the group.",
			},
			"is_internal_group": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       replaceBool,
				MarkdownDescription: "Whether the group is for internal testers (members of the team) rather than external testers.",
			},
			"has_access_to_all_builds": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       replaceBool,
				MarkdownDescription: "Whether the group automatically gets access to every build.",
			},
			"public_link_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepBool,
				MarkdownDescription: "Whether testers can join the group through a public link.",
			},
			"public_link_limit_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepBool,
				MarkdownDescription: "Whether `public_link_limit` caps the testers who join through the public link.",
			},
			"public_link_limit": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
				MarkdownDescription: "The maximum number of testers who can join through the public link. Requires `public_link_limit_enabled = true`. A limit of 0 does not close the link and App Store Connect rejects it; set `public_link_enabled = false` to close the link instead.",
			},
			"feedback_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepBool,
				MarkdownDescription: "Whether testers in the group can send feedback.",
			},
			"ios_builds_available_for_apple_silicon_mac": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepBool,
				MarkdownDescription: "Whether iOS builds are available to testers on Mac computers with Apple silicon. App Store Connect only accepts it on update, so the provider sets it right after creating the group.",
			},
			"ios_builds_available_for_apple_vision": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keepBool,
				MarkdownDescription: "Whether iOS builds are available to testers on Apple Vision Pro. App Store Connect only accepts it on update, so the provider sets it right after creating the group.",
			},
			"public_link": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The public TestFlight link, when enabled.",
			},
			"public_link_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the public TestFlight link, when enabled.",
			},
			"created_date": schema.StringAttribute{
				Computed:            true,
				PlanModifiers:       keepString,
				MarkdownDescription: "When the group was created, in RFC 3339 format.",
			},
		},
	}
}

func (r *betaGroupResourceImpl) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*apiClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *apiClient, got: %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *betaGroupResourceImpl) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan betaGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.createBetaGroup(ctx, plan.AppId.ValueString(), betaGroupCreateAttributes{
		Name:                   plan.Name.ValueString(),
		IsInternalGroup:        optionalBool(plan.IsInternalGroup),
		HasAccessToAllBuilds:   optionalBool(plan.HasAccessToAllBuilds),
		PublicLinkEnabled:      optionalBool(plan.PublicLinkEnabled),
		PublicLinkLimitEnabled: optionalBool(plan.PublicLinkLimitEnabled),
		PublicLinkLimit:        optionalInt64(plan.PublicLinkLimit),
		FeedbackEnabled:        optionalBool(plan.FeedbackEnabled),
	})
	if err != nil {
		appendBetaGroupError(&resp.Diagnostics, "Unable to create beta group", err)
		return
	}
	plan.Id = types.StringValue(created.ID)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), plan.Id)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group := created
	updateOnly := betaGroupUpdateAttributes{
		IosBuildsAvailableForAppleSiliconMac: optionalBool(plan.IosBuildsAvailableForAppleSiliconMac),
		IosBuildsAvailableForAppleVision:     optionalBool(plan.IosBuildsAvailableForAppleVision),
	}
	if updateOnly != (betaGroupUpdateAttributes{}) {
		group, err = r.client.updateBetaGroup(ctx, created.ID, updateOnly)
		if err != nil {
			appendBetaGroupError(&resp.Diagnostics, "Unable to configure beta group after create", err)
			return
		}
	}

	applyBetaGroup(&plan, group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *betaGroupResourceImpl) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state betaGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := r.client.getBetaGroup(ctx, state.Id.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read beta group: %s", err))
		return
	}

	applyBetaGroup(&state, group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *betaGroupResourceImpl) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state betaGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	changes := betaGroupUpdateAttributes{
		PublicLinkEnabled:                    changedBool(plan.PublicLinkEnabled, state.PublicLinkEnabled),
		PublicLinkLimitEnabled:               changedBool(plan.PublicLinkLimitEnabled, state.PublicLinkLimitEnabled),
		PublicLinkLimit:                      changedInt64(plan.PublicLinkLimit, state.PublicLinkLimit),
		FeedbackEnabled:                      changedBool(plan.FeedbackEnabled, state.FeedbackEnabled),
		IosBuildsAvailableForAppleSiliconMac: changedBool(plan.IosBuildsAvailableForAppleSiliconMac, state.IosBuildsAvailableForAppleSiliconMac),
		IosBuildsAvailableForAppleVision:     changedBool(plan.IosBuildsAvailableForAppleVision, state.IosBuildsAvailableForAppleVision),
	}
	if !plan.Name.Equal(state.Name) {
		name := plan.Name.ValueString()
		changes.Name = &name
	}
	group, err := r.client.updateBetaGroup(ctx, plan.Id.ValueString(), changes)
	if err != nil {
		appendBetaGroupError(&resp.Diagnostics, "Unable to update beta group", err)
		return
	}

	applyBetaGroup(&plan, group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *betaGroupResourceImpl) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state betaGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.deleteBetaGroup(ctx, state.Id.ValueString()); err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to delete beta group: %s", err))
	}
}

func (r *betaGroupResourceImpl) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *betaGroupResourceImpl) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config betaGroupResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.IsInternalGroup.ValueBool() {
		publicLinkSettings := map[string]bool{
			"public_link_enabled":       !config.PublicLinkEnabled.IsNull(),
			"public_link_limit_enabled": !config.PublicLinkLimitEnabled.IsNull(),
			"public_link_limit":         !config.PublicLinkLimit.IsNull(),
		}
		for attribute, set := range publicLinkSettings {
			if set {
				resp.Diagnostics.AddAttributeError(path.Root(attribute), "Public Link On Internal Group",
					fmt.Sprintf("Internal beta groups cannot have a public link. Remove `%s`, or set `is_internal_group = false` for an external group.", attribute))
			}
		}
	}

	if !config.PublicLinkLimit.IsNull() && !config.PublicLinkLimitEnabled.IsUnknown() && !config.PublicLinkLimitEnabled.ValueBool() {
		resp.Diagnostics.AddAttributeError(path.Root("public_link_limit"), "Public Link Limit Not Enabled",
			"`public_link_limit` only takes effect with `public_link_limit_enabled = true`. Enable the limit, or remove `public_link_limit`.")
	}
}

var betaGroupAttributePointers = map[string]string{
	"/data/attributes/name":                                 "name",
	"/data/attributes/isInternalGroup":                      "is_internal_group",
	"/data/attributes/hasAccessToAllBuilds":                 "has_access_to_all_builds",
	"/data/attributes/publicLinkEnabled":                    "public_link_enabled",
	"/data/attributes/publicLinkLimitEnabled":               "public_link_limit_enabled",
	"/data/attributes/publicLinkLimit":                      "public_link_limit",
	"/data/attributes/feedbackEnabled":                      "feedback_enabled",
	"/data/attributes/iosBuildsAvailableForAppleSiliconMac": "ios_builds_available_for_apple_silicon_mac",
	"/data/attributes/iosBuildsAvailableForAppleVision":     "ios_builds_available_for_apple_vision",
	"/data/relationships/app":                               "app_id",
}

func appendBetaGroupError(diags *diag.Diagnostics, summary string, err error) {
	rejected := entityErrors(err)
	if len(rejected) == 0 {
		diags.AddError("API Error", fmt.Sprintf("%s: %s", summary, err))
		return
	}
	for _, d := range rejected {
		detail := fmt.Sprintf("%s: App Store Connect rejected the value (%s): %s", summary, d.Code, d.Detail)
		if d.Source != nil {
			if attribute, ok := betaGroupAttributePointers[d.Source.Pointer]; ok {
				diags.AddAttributeError(path.Root(attribute), "Beta Group Rejected", detail)
				continue
			}
		}
		diags.AddError("Beta Group Rejected", detail)
	}
}

func applyBetaGroup(model *betaGroupResourceModel, group *betaGroupResource) {
	a := group.Attributes
	model.Id = types.StringValue(group.ID)
	if app := group.Relationships.App.Data; app != nil {
		model.AppId = types.StringValue(app.ID)
	}
	model.Name = types.StringValue(a.Name)
	model.IsInternalGroup = types.BoolValue(a.IsInternalGroup)
	model.HasAccessToAllBuilds = types.BoolValue(a.HasAccessToAllBuilds)
	model.PublicLinkEnabled = types.BoolValue(a.PublicLinkEnabled)
	model.PublicLinkLimitEnabled = types.BoolValue(a.PublicLinkLimitEnabled)
	if a.PublicLinkLimit > 0 {
		model.PublicLinkLimit = types.Int64Value(a.PublicLinkLimit)
	} else {
		model.PublicLinkLimit = types.Int64Null()
	}
	model.FeedbackEnabled = types.BoolValue(a.FeedbackEnabled)
	model.IosBuildsAvailableForAppleSiliconMac = types.BoolValue(a.IosBuildsAvailableForAppleSiliconMac)
	model.IosBuildsAvailableForAppleVision = types.BoolValue(a.IosBuildsAvailableForAppleVision)
	model.PublicLink = stringOrNull(a.PublicLink)
	model.PublicLinkId = stringOrNull(a.PublicLinkID)
	model.CreatedDate = stringOrNull(a.CreatedDate)
}
