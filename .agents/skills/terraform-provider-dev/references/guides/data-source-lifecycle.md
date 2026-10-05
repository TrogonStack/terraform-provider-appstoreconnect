# Data Source Lifecycle

The provider does not define any data sources today (`DataSources()` in `provider.go` returns an empty slice). Everything below is the pattern to follow when one is added, built around a hypothetical `appstoreconnect_bundle_id` data source (the App Store Connect `bundleIds` endpoint, `GET /v1/bundleIds/{id}`).

## Interface

A data source must implement `datasource.DataSource`:

```go
type DataSource interface {
    Metadata(context.Context, MetadataRequest, *MetadataResponse)
    Schema(context.Context, SchemaRequest, *SchemaResponse)
    Read(context.Context, ReadRequest, *ReadResponse)
}
```

Optional interfaces:

- `datasource.DataSourceWithConfigure`: receive provider client
- `datasource.DataSourceWithValidateConfig`: configuration validation

## Registration

```go
func newBundleIdDataSource() datasource.DataSource { return &bundleIdDataSource{} }

// In provider.go:
func (p *appStoreConnectProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
    return []func() datasource.DataSource{
        newBundleIdDataSource,
    }
}
```

## Metadata

```go
func (d *bundleIdDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_bundle_id"
}
```

## Schema

Data source schemas use `datasource/schema` package (not `resource/schema`):

```go
import "github.com/hashicorp/terraform-plugin-framework/datasource/schema"

func (d *bundleIdDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "id":         schema.StringAttribute{Required: true},
            "identifier": schema.StringAttribute{Computed: true},
            "name":       schema.StringAttribute{Computed: true},
            "platform":   schema.StringAttribute{Computed: true},
        },
    }
}
```

Key differences from resource schemas:

- No plan modifiers (no plan phase for data sources)
- No defaults (no apply phase)
- Attributes are either Required (lookup key) or Computed (returned value)
- Optional attributes serve as optional filter criteria

## Configure

Same pattern as resources:

```go
func (d *bundleIdDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
    if req.ProviderData == nil {
        return
    }
    client, ok := req.ProviderData.(*apiClient)
    if !ok {
        resp.Diagnostics.AddError("Unexpected DataSource Configure Type",
            fmt.Sprintf("Expected *apiClient, got: %T", req.ProviderData))
        return
    }
    d.client = client
}
```

## Read

Contract:

- Read configuration from `req.Config` (the user-provided lookup criteria)
- Perform the API call to find the data
- If not found: add an error diagnostic (data sources must find their target)
- Set all attribute values in `resp.State`

`bundleIds` has a direct get-by-id endpoint, so a data source would call `client.get` once, rather than filtering a list client-side:

```go
func (d *bundleIdDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
    var data bundleIdDataSourceModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
    if resp.Diagnostics.HasError() {
        return
    }

    var out struct {
        Data struct {
            Attributes struct {
                Identifier string `json:"identifier"`
                Name       string `json:"name"`
                Platform   string `json:"platform"`
            } `json:"attributes"`
        } `json:"data"`
    }
    if err := d.client.get(ctx, "/v1/bundleIds/"+data.Id.ValueString(), nil, &out); err != nil {
        if isNotFound(err) {
            resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Bundle ID %q not found", data.Id.ValueString()))
            return
        }
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read bundle ID: %s", err))
        return
    }

    data.Identifier = types.StringValue(out.Data.Attributes.Identifier)
    data.Name = types.StringValue(out.Data.Attributes.Name)
    data.Platform = types.StringValue(out.Data.Attributes.Platform)
    resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
```

A future data source backed by a list-only endpoint (no get-by-id) would instead call `listAll[T]` and filter client-side, treating "not present in the list" the same way: an error diagnostic, not a state removal.

## Data Sources vs Resources

| Aspect           | Resource                     | Data Source          |
| ------------------ | ------------------------------- | ----------------------- |
| Purpose          | Manage lifecycle (CRUD)      | Read-only lookup     |
| Methods          | Create, Read, Update, Delete | Read only            |
| Import           | Supported                    | N/A                  |
| Plan modifiers   | Yes                          | No                   |
| Defaults         | Yes                          | No                   |
| State management | Full lifecycle               | Refreshed every plan |
| Not found        | RemoveResource (drift)       | Error diagnostic     |

## Related Framework References

| File                                                | Contents                            |
| ------------------------------------------------------ | -------------------------------------- |
| `framework/data-sources/index.mdx`                  | Data source interface, registration |
| `framework/data-sources/configure.mdx`              | Configure method                    |
| `framework/data-sources/validate-configuration.mdx` | Validation                          |
| `framework/data-sources/timeouts.mdx`               | Timeout support                     |
