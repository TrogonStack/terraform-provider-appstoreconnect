# Provider Configuration

## Overview

The provider is the top-level component that:

1. Defines its own configuration schema (auth credentials)
2. Creates the API client during Configure
3. Passes client data to resources and data sources
4. Registers all available resources and data sources

## Provider Interface

```go
type Provider interface {
    Metadata(context.Context, MetadataRequest, *MetadataResponse)
    Schema(context.Context, SchemaRequest, *SchemaResponse)
    Configure(context.Context, ConfigureRequest, *ConfigureResponse)
    Resources(context.Context) []func() resource.Resource
    DataSources(context.Context) []func() datasource.DataSource
}
```

## This Provider's Structure

### Metadata

```go
func (p *appStoreConnectProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
    resp.TypeName = "appstoreconnect"
    resp.Version = p.version
}
```

`TypeName` becomes the prefix for all resource names (e.g. a hypothetical `appstoreconnect_bundle_id`, since no resource exists yet).

### Schema

Provider schema defines what goes in the `provider "appstoreconnect" {}` block:

```go
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
| ------------- | -------------------------------- |
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
```

### Configure

Configure creates the API client and makes it available to resources:

```go
func (p *appStoreConnectProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
    var data appStoreConnectProviderModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
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

    // keyID and privateKey are resolved the same way, from key_id/APP_STORE_CONNECT_KEY_ID
    // and private_key/APP_STORE_CONNECT_PRIVATE_KEY...

    credentials, err := newAPICredentials(issuerID, keyID, privateKey)
    if err != nil {
        resp.Diagnostics.AddError("Configuration Error", "Unable to load the App Store Connect API key: "+err.Error())
        return
    }

    client := newAPIClient(defaultBaseURL, newRetryableClient(), newTokenSource(credentials))
    resp.DataSourceData = client
    resp.ResourceData = client
}
```

### Resource/DataSource Registration

```go
func (p *appStoreConnectProvider) Resources(ctx context.Context) []func() resource.Resource {
    return []func() resource.Resource{}
}

func (p *appStoreConnectProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
    return []func() datasource.DataSource{}
}
```

Both are empty today, since this provider has not shipped a resource or data source yet. Adding one means appending its constructor function here (see `references/guides/resource-lifecycle.md` and `references/guides/data-source-lifecycle.md`).

## Client Data Flow

```
provider.Configure()
    -> resp.ResourceData = client
    -> resp.DataSourceData = client

resource.Configure()
    -> req.ProviderData == client (same pointer)
    -> r.client = req.ProviderData.(*apiClient)

resource.Create/Read/Update/Delete()
    -> r.client.post(ctx, "/v1/bundleIds", body, &out)
    -> r.client.get(ctx, "/v1/bundleIds/"+id, nil, &out)
    -> r.client.patch(ctx, "/v1/bundleIds/"+id, body, &out)
    -> r.client.delete(ctx, "/v1/bundleIds/"+id, nil)
```

The last four calls are illustrative (no resource exists yet); `*apiClient` itself is real, in `client.go`:

```go
type apiClient struct {
    baseURL    string
    httpClient *http.Client
    tokens     bearerTokenSource
}

func newAPIClient(baseURL string, httpClient *http.Client, tokens bearerTokenSource) *apiClient {
    return &apiClient{baseURL: strings.TrimSuffix(baseURL, "/"), httpClient: httpClient, tokens: tokens}
}
```

It is a small hand-written JSON:API client, not a generated SDK: `get`, `post`, `patch`, and `delete` all funnel through `send`, which attaches a bearer token from `tokens.Token()`, decodes a non-2xx response's JSON:API `errors` array into an `*apiError` (see `errors.go`), and otherwise JSON-decodes the response body into `out`. `listAll[T]` wraps repeated `get` calls to follow `links.next` for paginated list endpoints, refusing to follow a link to any host other than the configured base URL.

## Environment Variable Fallbacks

Provider config values can fall back to environment variables:

```go
issuerID := data.IssuerID.ValueString()
if issuerID == "" {
    issuerID = os.Getenv("APP_STORE_CONNECT_ISSUER_ID")
}
```

This provider supports:

| Attribute     | Environment variable            |
| ------------- | -------------------------------- |
| `issuer_id`   | `APP_STORE_CONNECT_ISSUER_ID`   |
| `key_id`      | `APP_STORE_CONNECT_KEY_ID`      |
| `private_key` | `APP_STORE_CONNECT_PRIVATE_KEY` |

The provider adds an error diagnostic worded `"<attribute> must be set, either in the provider configuration or the <ENV> environment variable"` if a value is missing from both configuration and environment.

## Authentication

`jwt.go` turns `issuer_id`/`key_id`/`private_key` into a signed bearer token. `newAPICredentials` parses the PEM `private_key` with `parsePrivateKey` (PKCS#8, requiring a P-256 ECDSA key); `newTokenSource` wraps the resulting `apiCredentials` in a `tokenSource` that signs and caches a token:

```go
const (
    tokenAudience      = "appstoreconnect-v1"
    tokenLifetime      = 15 * time.Minute
    tokenRefreshMargin = 2 * time.Minute
)

func (s *tokenSource) Token() (string, error) {
    // Reuses the cached token until it is within tokenRefreshMargin of expiring,
    // otherwise signs a new one with signToken and caches it.
}
```

`signToken` builds a JWT by hand: header `{alg: "ES256", kid, typ: "JWT"}`, claims `{iss, iat, exp, aud: "appstoreconnect-v1"}`, both base64url-encoded and signed over their concatenation with `ecdsa.Sign`, producing a raw `r || s` 64-byte signature (no external JWT library).

## Retry

`retry.go`'s `newRetryableClient` wraps go-retryablehttp's client with a custom `retryPolicy`: retry on connection errors, HTTP 429, and 5xx except 501 (Not Implemented, which retrying cannot fix); everything else, and a cancelled context, stop immediately. The provider wires this in unconditionally; there is no provider-level attribute to configure it.

## Test Bypass

Tests inject a client via the package-level `testAPIClient` variable, declared in `provider.go`:

```go
var testAPIClient *apiClient

func (p *appStoreConnectProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
    // ...
    if testAPIClient != nil {
        resp.DataSourceData = testAPIClient
        resp.ResourceData = testAPIClient
        return
    }
    // ... real authentication logic
}
```

`newTestServer` in `client_test.go` is what builds a client pointed at a fake: it stands up an `httptest.Server` that checks the bearer token's signature before delegating to the test's handler, and returns `newAPIClient(server.URL, server.Client(), newTokenSource(credentials))`. A future resource test would assign that client to `testAPIClient`, bypassing provider configuration and real JWT verification entirely (see `references/guides/testing.md`).

## Provider Server (main.go)

The entry point wraps the provider into a gRPC server:

```go
package main

import (
    "context"
    "log"

    "github.com/TrogonStack/terraform-provider-appstoreconnect/internal/provider"
    "github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version string = "dev"

func main() {
    err := providerserver.Serve(
        context.Background(),
        provider.New(version),
        providerserver.ServeOpts{
            Address: "registry.terraform.io/trogonstack/appstoreconnect",
        },
    )
    if err != nil {
        log.Fatal(err)
    }
}
```

## Related Framework References

| File                                             | Contents                                    |
| ---------------------------------------------------- | ---------------------------------------------- |
| `framework/providers/index.mdx`                  | Provider interface, metadata, schema        |
| `framework/providers/validate-configuration.mdx` | Provider-level validation                   |
| `framework/provider-servers.mdx`                 | Server setup, protocol versions, debug mode |
| `framework/resources/configure.mdx`              | How resources receive provider data         |
| `framework/data-sources/configure.mdx`           | How data sources receive provider data      |
