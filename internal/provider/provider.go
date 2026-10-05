package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &appStoreConnectProvider{}

// testAPIClient is set by tests to bypass authentication and inject a client pointed at the fake API.
var testAPIClient *apiClient

type appStoreConnectProvider struct {
	version string
}

type appStoreConnectProviderModel struct {
	IssuerID   types.String `tfsdk:"issuer_id"`
	KeyID      types.String `tfsdk:"key_id"`
	PrivateKey types.String `tfsdk:"private_key"`
}

func (p *appStoreConnectProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "appstoreconnect"
	resp.Version = p.version
}

func (p *appStoreConnectProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manage App Store Connect with Terraform.

The provider talks to the App Store Connect API, which automates the tasks
that are otherwise done by hand in App Store Connect and the Apple Developer
website.

## Authentication

Every request carries a JSON Web Token signed with ES256 from an App Store
Connect API key, as Apple describes in "Generating Tokens for API Requests".
Create a team key in App Store Connect under Users and Access > Integrations >
App Store Connect API, then pass its issuer ID, key ID, and the contents of the
downloaded ` + "`.p8`" + ` file. The provider signs a token that lives 15 minutes and
signs a new one shortly before it expires. The role assigned to the
key limits what the provider can manage.

## Environment variables

| Attribute     | Environment variable            |
| ------------- | ------------------------------- |
| ` + "`issuer_id`" + `   | ` + "`APP_STORE_CONNECT_ISSUER_ID`" + `   |
| ` + "`key_id`" + `      | ` + "`APP_STORE_CONNECT_KEY_ID`" + `      |
| ` + "`private_key`" + ` | ` + "`APP_STORE_CONNECT_PRIVATE_KEY`" + ` |`,
		Attributes: map[string]schema.Attribute{
			"issuer_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The issuer ID shown above the list of API keys in App Store Connect.",
			},
			"key_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The ID of the App Store Connect API key.",
			},
			"private_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "The PEM contents of the API key's `.p8` file, including the `BEGIN PRIVATE KEY` and `END PRIVATE KEY` lines.",
			},
		},
	}
}

func (p *appStoreConnectProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data appStoreConnectProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unknown := map[string]bool{
		"issuer_id":   data.IssuerID.IsUnknown(),
		"key_id":      data.KeyID.IsUnknown(),
		"private_key": data.PrivateKey.IsUnknown(),
	}
	for attribute, isUnknown := range unknown {
		if isUnknown {
			resp.Diagnostics.AddAttributeError(path.Root(attribute), "Unknown Provider Configuration",
				fmt.Sprintf("`%s` depends on a value that is only known after apply, so the provider cannot authenticate during this plan. Use a value known at plan time, or apply the resources it depends on first with -target.", attribute))
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// In tests, skip authentication and use the injected client.
	if testAPIClient != nil {
		resp.DataSourceData = testAPIClient
		resp.ResourceData = testAPIClient
		return
	}

	issuerID := data.IssuerID.ValueString()
	if issuerID == "" {
		issuerID = os.Getenv("APP_STORE_CONNECT_ISSUER_ID")
	}
	if issuerID == "" {
		resp.Diagnostics.AddError("Configuration Error", "issuer_id must be set, either in the provider configuration or the APP_STORE_CONNECT_ISSUER_ID environment variable")
		return
	}

	keyID := data.KeyID.ValueString()
	if keyID == "" {
		keyID = os.Getenv("APP_STORE_CONNECT_KEY_ID")
	}
	if keyID == "" {
		resp.Diagnostics.AddError("Configuration Error", "key_id must be set, either in the provider configuration or the APP_STORE_CONNECT_KEY_ID environment variable")
		return
	}

	privateKey := data.PrivateKey.ValueString()
	if privateKey == "" {
		privateKey = os.Getenv("APP_STORE_CONNECT_PRIVATE_KEY")
	}
	if privateKey == "" {
		resp.Diagnostics.AddError("Configuration Error", "private_key must be set, either in the provider configuration or the APP_STORE_CONNECT_PRIVATE_KEY environment variable")
		return
	}

	credentials, err := newAPICredentials(issuerID, keyID, privateKey)
	if err != nil {
		resp.Diagnostics.AddError("Configuration Error", "Unable to load the App Store Connect API key: "+err.Error())
		return
	}

	tokens := newTokenSource(credentials)
	client := newAPIClient(defaultBaseURL, newRetryableClient(tokens), tokens)
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *appStoreConnectProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newBetaGroup,
	}
}

func (p *appStoreConnectProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newAppDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &appStoreConnectProvider{
			version: version,
		}
	}
}
