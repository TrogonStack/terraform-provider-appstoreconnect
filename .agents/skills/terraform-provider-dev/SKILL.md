---
name: terraform-provider-dev
description: >
  Use this skill when developing terraform-provider-appstoreconnect: adding
  resources or data sources, designing schemas, implementing CRUD operations,
  plan modification, state upgrades, import, validation, acceptance testing,
  debugging, or any Terraform Plugin Framework work in Go. Also use when the
  user asks about terraform provider patterns, attribute types, or how to
  structure tests. This is the primary development skill for this repository.
---

# Terraform Provider Development (Plugin Framework)

## Mental Model

- Provider = Go server implementing Terraform RPCs (GetProviderSchema, PlanResourceChange, ApplyResourceChange, ReadResource, etc.)
- Resource = struct implementing `resource.Resource` interface: Metadata, Schema, Configure, Create, Read, Update, Delete
- DataSource = struct implementing `datasource.DataSource` interface: Metadata, Schema, Configure, Read
- Schema defines the "shape" of config/plan/state: attributes (leaf values) and blocks (nested structures)
- Plan then Apply: Terraform calls PlanResourceChange (propose changes), then ApplyResourceChange (execute)
- State = Terraform's record of the real world; Plan = expected post-apply state
- Computed attributes: set by the provider from API responses (IDs, timestamps, server-generated values)
- Plugin Framework uses strong Go types: `types.String`, `types.Bool`, `types.Int64`, `types.List`, etc.
- Null vs Unknown: null means the user did not set it; unknown means the value will be known after apply (planned computed)

---

## This Provider: Conventions

This provider ships the provider configuration, authentication and API client today; it has no resources or data sources yet (`Resources()` and `DataSources()` in `provider.go` both return an empty slice). Everything under "Adding a New Resource" and "Adding a Data Source" below is illustrative, built around a hypothetical `appstoreconnect_bundle_id` resource (the App Store Connect `bundleIds` endpoint, `/v1/bundleIds/{id}`).

- **Package**: `internal/provider` (single flat package, all resources will live here)
- **File naming**: `resource_<name>.go`, `resource_<name>_test.go`, `data_source_<name>.go`
- **Provider client**: `*apiClient` in `client.go` is a small hand-written JSON:API client (`get`, `post`, `patch`, `delete`, and `listAll[T]` for pagination). No Go SDK for the App Store Connect API is both maintained and permissively licensed, so none is used
- **Client injection**: Configure method casts `req.ProviderData.(*apiClient)`
- **Auth**: `jwt.go` signs an ES256 JWT (`aud` `appstoreconnect-v1`) from the configured API key, caches it in `tokenSource` for 15 minutes and re-signs it 2 minutes before expiry
- **ID helper**: no `helpers.go` exists yet, since no resource has been added. When the first resource lands, give it a Computed `id` attribute with `stringplanmodifier.UseStateForUnknown()`, following the pattern described in `references/guides/schema-design.md`
- **Registration**: new resources and data sources are added to `Resources()` / `DataSources()` in `provider.go`, both currently empty
- **Not-found handling**: the API reports a missing object as HTTP 404 with a JSON:API `errors` array, which `client.go` decodes into `*apiError`. Detect it with `isNotFound(err)` in `errors.go`, which unwraps with `errors.As` (a wrapped error would otherwise fail a direct type check)
  - **Read**: call `resp.State.RemoveResource(ctx)` and return (resource was deleted externally)
  - **Delete**: return without error (idempotent)
- **Missing operations**: several App Store Connect types have no update or delete operation. Mark attributes that cannot be updated with `RequiresReplace`, and when there is no delete, document what destroy does instead in the resource description
- **Source of truth**: model every resource on Apple's App Store Connect API documentation and its OpenAPI specification. Never add fields or behaviors the specification does not describe
- **Retry**: `retry.go` wraps the client's HTTP transport with automatic retry on 429 and 5xx except 501, honoring `Retry-After`. No configuration attribute
- **Testing**: no resource tests exist yet. `helpers_test.go` generates a P-256 test key (`newTestCredentials`) and checks a bearer token's signature, header and audience (`verifyTestToken`); `client_test.go` points a real `apiClient` at an `httptest.Server` with `newAPIClient(server.URL, server.Client(), newTokenSource(credentials))`. A future resource's tests would inject that client as the package-level `testAPIClient`, bypassing provider configuration, backed by an in-memory `httptest` fake of the endpoints it calls. `TestLive_*` in `live_test.go` runs against a real App Store Connect team, skipped unless `TF_ACC` and the three `APP_STORE_CONNECT_*` variables are set

---

## Adding a New Resource

1. Create `internal/provider/resource_<name>.go`
2. Define model struct(s) with `tfsdk` tags
3. Implement the resource:

```go
var (
    _ resource.Resource                = &fooResource{}
    _ resource.ResourceWithImportState = &fooResource{}
)

func newFoo() resource.Resource { return &fooResource{} }

type fooResource struct {
    client *apiClient
}

type fooResourceModel struct {
    Id   types.String `tfsdk:"id"`
    Name types.String `tfsdk:"name"`
}

func (r *fooResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_foo"
}

func (r *fooResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "id": schema.StringAttribute{
                Computed: true,
                PlanModifiers: []planmodifier.String{
                    stringplanmodifier.UseStateForUnknown(),
                },
            },
            "name": schema.StringAttribute{Required: true},
        },
    }
}

func (r *fooResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
```

4. Implement Create, Read, Update, Delete (see guide: `references/guides/resource-lifecycle.md`)
5. Implement ImportState
6. Register in `provider.go`: add `newFoo` to `Resources()` return slice
7. Create `internal/provider/resource_foo_test.go` (see guide: `references/guides/testing.md`)

---

## Adding a Data Source

The provider does not define any data sources yet (`DataSources()` returns an empty slice). The shape below is illustrative, built around the hypothetical `appstoreconnect_bundle_id` resource, assuming a direct get-by-id endpoint (`GET /v1/bundleIds/{id}`):

```go
var _ datasource.DataSource = &bundleIdDataSource{}

func newBundleIdDataSource() datasource.DataSource { return &bundleIdDataSource{} }

type bundleIdDataSource struct {
    client *apiClient
}

type bundleIdDataSourceModel struct {
    Id         types.String `tfsdk:"id"`
    Identifier types.String `tfsdk:"identifier"`
    Name       types.String `tfsdk:"name"`
    Platform   types.String `tfsdk:"platform"`
}

func (d *bundleIdDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_bundle_id"
}

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

func (d *bundleIdDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
    if req.ProviderData == nil {
        return
    }
    client, ok := req.ProviderData.(*apiClient)
    if !ok {
        resp.Diagnostics.AddError("Unexpected DataSource Configure Type", fmt.Sprintf("Expected *apiClient, got: %T", req.ProviderData))
        return
    }
    d.client = client
}

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
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read bundle ID: %s", err))
        return
    }

    data.Identifier = types.StringValue(out.Data.Attributes.Identifier)
    data.Name = types.StringValue(out.Data.Attributes.Name)
    data.Platform = types.StringValue(out.Data.Attributes.Platform)
    resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
```

Register: add `newBundleIdDataSource` to `DataSources()` in `provider.go`.

---

## Schema Design Quick-Reference

| Schema Type                                                                                    | Go Model Type            | When to Use                    |
| ---------------------------------------------------------------------------------------------- | ------------------------ | ------------------------------- |
| `schema.StringAttribute{Required: true}`                                                       | `types.String`           | User must provide              |
| `schema.StringAttribute{Optional: true}`                                                       | `types.String`           | User may provide               |
| `schema.StringAttribute{Computed: true}`                                                       | `types.String`           | Server-generated only          |
| `schema.StringAttribute{Optional: true, Computed: true}`                                       | `types.String`           | User provides OR server fills  |
| `schema.StringAttribute{Sensitive: true}`                                                      | `types.String`           | Credential value (provider's `private_key`) |

The table above reflects this provider's real `private_key` attribute; the rest is illustrative until a resource exists. A hypothetical `appstoreconnect_bundle_id` would likely mark `identifier` and `platform` Required (the API has no rename or platform-change operation) and `name` Optional, since bundle name changes are typically allowed without recreating the bundle ID.

### Plan Modifiers

| Modifier                                  | Use Case                                   |
| ------------------------------------------ | ------------------------------------------ |
| `stringplanmodifier.UseStateForUnknown()` | Computed value stable across updates (an `id` attribute) |
| `stringplanmodifier.RequiresReplace()`    | Changing this forces resource recreation (immutable fields, e.g. a hypothetical `identifier` or `platform` on `appstoreconnect_bundle_id`) |

Full details: `references/guides/schema-design.md`

---

## Testing Patterns

### Test Infrastructure

No acceptance-test harness (`resource.Test`, a provider factory map) exists yet, since there are no resources to exercise that way. What exists today are direct unit tests of the provider and client:

```go
func configureProvider(t *testing.T, config map[string]string) *provider.ConfigureResponse {
    t.Helper()
    ctx := context.Background()
    p := New("test")()

    schemaResp := &provider.SchemaResponse{}
    p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
    // build a tftypes.Value from config and call p.Configure(...)
}
```

`TestProviderConfigure_FromAttributes`, `TestProviderConfigure_FromEnvironment`, `TestProviderConfigure_Errors`, and `TestProviderConfigure_UsesTestClient` in `provider_test.go` exercise this directly, without ever building a Terraform config string.

### Test Structure (what a future resource test will look like)

```go
func newTestServer(t *testing.T, handler http.HandlerFunc) (*apiClient, apiCredentials) {
    t.Helper()
    credentials, _ := newTestCredentials(t)
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
        if !ok || verifyTestToken(token, &credentials.privateKey.PublicKey, credentials.keyID) != nil {
            writeJSON(w, http.StatusUnauthorized, map[string]any{"errors": []apiErrorDetail{{Status: "401", Code: "NOT_AUTHORIZED"}}})
            return
        }
        handler(w, r)
    }))
    t.Cleanup(server.Close)
    return newAPIClient(server.URL, server.Client(), newTokenSource(credentials)), credentials
}
```

This is the real `newTestServer` in `client_test.go`. A resource test would inject the resulting client as `testAPIClient` (bypassing provider `Configure` entirely) and drive it through `resource.Test` once the first resource exists.

### Running Tests

```bash
go test ./internal/provider/ -v -run TestClient
go test ./internal/provider/ -v -run TestProviderConfigure
mise run test:live   # TestLive_* against a real App Store Connect team
```

Full details: `references/guides/testing.md`

---

## State Upgrade

No resource exists yet, so none has needed a state upgrade. If a future resource's schema needs a breaking change after it ships (e.g. splitting a flat attribute into a nested object):

1. Increment `Version` in the schema
2. Implement `resource.ResourceWithUpgradeState`
3. Parse raw JSON state and write to current model

```go
func (r *fooResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {
            StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
                var raw map[string]json.RawMessage
                if err := json.Unmarshal(req.RawState.JSON, &raw); err != nil {
                    resp.Diagnostics.AddError("State Upgrade Error", fmt.Sprintf("Unable to parse raw state: %s", err))
                    return
                }
                // Parse old format, build new model, set state
                resp.Diagnostics.Append(resp.State.Set(ctx, &newModel)...)
            },
        },
    }
}
```

Full details: `references/guides/state-management.md`

---

## Reference Docs

### Topic Guides (synthesized, task-oriented)

| Guide                                         | Contents                                              |
| ---------------------------------------------- | ------------------------------------------------------ |
| `references/guides/resource-lifecycle.md`     | CRUD methods, interface contracts, registration       |
| `references/guides/data-source-lifecycle.md`  | Data source pattern, Read method                      |
| `references/guides/schema-design.md`          | Attributes, blocks, types, nested models              |
| `references/guides/plan-modification.md`      | UseStateForUnknown, RequiresReplace, custom modifiers |
| `references/guides/state-management.md`       | Import, state upgrade, private state                  |
| `references/guides/validation.md`             | Attribute validators, resource-level validation       |
| `references/guides/testing.md`                | Unit tests, httptest fakes, live acceptance tests     |
| `references/guides/provider-configuration.md` | Provider setup, client injection, auth                |
| `references/guides/functions.md`              | Provider-defined functions (Terraform 1.8+)           |

### Framework Reference (verbatim, upstream HashiCorp docs)

Key entry points in `references/framework/`:

| File                                 | Contents                         |
| ------------------------------------- | --------------------------------- |
| `resources/index.mdx`                | Resource interface, registration |
| `resources/create.mdx`               | Create method contract           |
| `resources/read.mdx`                 | Read method, refresh state       |
| `resources/update.mdx`               | Update method, in-place changes  |
| `resources/delete.mdx`               | Delete method                    |
| `resources/configure.mdx`            | Client injection into resources  |
| `resources/import.mdx`               | Import state support             |
| `resources/plan-modification.mdx`    | Plan modifiers                   |
| `resources/state-upgrade.mdx`        | State upgrade for schema changes |
| `data-sources/index.mdx`             | Data source interface            |
| `handling-data/schemas.mdx`          | Schema definition                |
| `handling-data/accessing-values.mdx` | Reading config/plan/state        |
| `handling-data/writing-state.mdx`    | Writing to response state        |
| `handling-data/attributes/index.mdx` | All attribute types              |
| `handling-data/blocks/index.mdx`     | All block types                  |
| `handling-data/types/index.mdx`      | Type system (Go value types)     |
| `validation.mdx`                     | Validation patterns              |
| `diagnostics.mdx`                    | Error/warning diagnostics        |
| `acctests.mdx`                       | Acceptance testing setup         |
| `debugging.mdx`                      | Debugging providers              |
| `providers/index.mdx`                | Provider interface               |
| `provider-servers.mdx`               | Provider server (main.go)        |
| `functions/implementation.mdx`       | Provider functions               |
| `migrating/index.mdx`                | SDKv2 migration overview         |
</content_truncated_for_brevity>
