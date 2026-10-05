package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &appDataSource{}

func newAppDataSource() datasource.DataSource { return &appDataSource{} }

type appDataSource struct {
	client *apiClient
}

type appDataSourceModel struct {
	Id            types.String `tfsdk:"id"`
	BundleId      types.String `tfsdk:"bundle_id"`
	Name          types.String `tfsdk:"name"`
	Sku           types.String `tfsdk:"sku"`
	PrimaryLocale types.String `tfsdk:"primary_locale"`
}

func (d *appDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func (d *appDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Looks up an app in App Store Connect by its bundle identifier.

The lookup fails unless exactly one app visible to the API key has that bundle identifier.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The App Store Connect ID of the app.",
			},
			"bundle_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The bundle identifier of the app, e.g. `com.example.app`.",
				Validators:          []validator.String{bundleIdentifierValidator{}},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the app.",
			},
			"sku": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The SKU of the app.",
			},
			"primary_locale": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The primary locale of the app, e.g. `en-US`.",
			},
		},
	}
}

func (d *appDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*apiClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *apiClient, got: %T", req.ProviderData))
		return
	}
	d.client = client
}

func (d *appDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config appDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bundleID, err := parseBundleIdentifier(config.BundleId.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("bundle_id"), "Invalid Configuration", err.Error())
		return
	}

	apps, err := d.client.findAppsByBundleID(ctx, bundleID)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to look up app: %s", err))
		return
	}
	if len(apps) == 0 {
		resp.Diagnostics.AddAttributeError(path.Root("bundle_id"), "App Not Found", fmt.Sprintf("No app with bundle identifier %q is visible to this API key.", bundleID))
		return
	}
	if len(apps) > 1 {
		resp.Diagnostics.AddAttributeError(path.Root("bundle_id"), "Multiple Apps Found", fmt.Sprintf("%d apps with bundle identifier %q are visible to this API key.", len(apps), bundleID))
		return
	}

	app := apps[0]
	config.Id = types.StringValue(app.ID)
	config.Name = stringOrNull(app.Attributes.Name)
	config.Sku = stringOrNull(app.Attributes.SKU)
	config.PrimaryLocale = stringOrNull(app.Attributes.PrimaryLocale)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
