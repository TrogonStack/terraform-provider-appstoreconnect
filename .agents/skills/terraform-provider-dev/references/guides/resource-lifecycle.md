# Resource Lifecycle

This provider has no resources yet (`Resources()` in `provider.go` returns an empty slice). Everything below is illustrative, built around a hypothetical `appstoreconnect_bundle_id` resource (the App Store Connect `bundleIds` endpoint, `/v1/bundleIds/{id}`), grounded in the real `apiClient`, `errors.go`, and `jwt.go`/`retry.go` plumbing that already ships.

## Interface

A resource must implement `resource.Resource`:

```go
type Resource interface {
    Metadata(context.Context, MetadataRequest, *MetadataResponse)
    Schema(context.Context, SchemaRequest, *SchemaResponse)
    Create(context.Context, CreateRequest, *CreateResponse)
    Read(context.Context, ReadRequest, *ReadResponse)
    Update(context.Context, UpdateRequest, *UpdateResponse)
    Delete(context.Context, DeleteRequest, *DeleteResponse)
}
```

Optional interfaces:

- `resource.ResourceWithConfigure`: receive provider client
- `resource.ResourceWithImportState`: support `terraform import`
- `resource.ResourceWithUpgradeState`: handle schema migrations
- `resource.ResourceWithModifyPlan`: resource-level plan modification
- `resource.ResourceWithValidateConfig`: resource-level validation

## Registration

Add a constructor function to the provider's `Resources()` method:

```go
func newBundleId() resource.Resource { return &bundleIdResource{} }

// In provider.go:
func (p *appStoreConnectProvider) Resources(ctx context.Context) []func() resource.Resource {
    return []func() resource.Resource{
        newBundleId,
    }
}
```

## Metadata

Sets the resource type name as it appears in Terraform configurations:

```go
func (r *bundleIdResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_bundle_id"
}
```

This produces `appstoreconnect_bundle_id` as the resource type (`req.ProviderTypeName` is `"appstoreconnect"`, set in the provider's own `Metadata`).

## Configure

Receive the provider-configured API client:

```go
func (r *bundleIdResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
    if req.ProviderData == nil {
        return
    }
    client, ok := req.ProviderData.(*apiClient)
    if !ok {
        resp.Diagnostics.AddError("Unexpected Resource Configure Type",
            fmt.Sprintf("Expected *apiClient, got: %T", req.ProviderData))
        return
    }
    r.client = client
}
```

The `nil` check is required: Configure is called during validation when provider data is not yet available.

## Create

Contract:

- Read plan data from `req.Plan`
- Perform the API creation call
- Set ALL attribute values (including computed) in `resp.State`
- Unknown values in plan MUST become known in state (error otherwise)
- On error, the resource is marked tainted for recreation on next plan

```go
func (r *bundleIdResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
    var plan bundleIdResourceModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() {
        return
    }

    body := map[string]any{
        "data": map[string]any{
            "type": "bundleIds",
            "attributes": map[string]any{
                "identifier": plan.Identifier.ValueString(),
                "name":       plan.Name.ValueString(),
                "platform":   plan.Platform.ValueString(),
            },
        },
    }

    var out bundleIdDocument
    if err := r.client.post(ctx, "/v1/bundleIds", body, &out); err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to create bundle ID: %s", err))
        return
    }

    plan.Id = types.StringValue(out.Data.ID)
    resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
```

## Read

Contract:

- Read prior state from `req.State`
- Perform the API read call
- If the resource no longer exists: call `resp.State.RemoveResource(ctx)` and return
- Otherwise, update all state values to reflect current API state

A direct get-by-id call, and the real `isNotFound` helper from `errors.go` to detect drift:

```go
func (r *bundleIdResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
    var state bundleIdResourceModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() {
        return
    }

    var out bundleIdDocument
    if err := r.client.get(ctx, "/v1/bundleIds/"+state.Id.ValueString(), nil, &out); err != nil {
        if isNotFound(err) {
            resp.State.RemoveResource(ctx)
            return
        }
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read bundle ID: %s", err))
        return
    }

    state.Identifier = types.StringValue(out.Data.Attributes.Identifier)
    state.Name = types.StringValue(out.Data.Attributes.Name)
    state.Platform = types.StringValue(out.Data.Attributes.Platform)
    resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
```

For an endpoint that has no get-by-id (list-only), Read would call `listAll[T]` and filter client-side by ID instead, collapsing "a 404 from the API" and "present in state but absent from the list" into the same `RemoveResource` branch.

## Update

Contract:

- Read plan data from `req.Plan` (the desired new state)
- Perform the API update call
- Set state to reflect the actual post-update values
- All values in state MUST match plan values (or Terraform produces an "inconsistent result" error)

```go
func (r *bundleIdResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
    var plan bundleIdResourceModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() {
        return
    }

    body := map[string]any{
        "data": map[string]any{
            "type":       "bundleIds",
            "id":         plan.Id.ValueString(),
            "attributes": map[string]any{"name": plan.Name.ValueString()},
        },
    }

    var out bundleIdDocument
    if err := r.client.patch(ctx, "/v1/bundleIds/"+plan.Id.ValueString(), body, &out); err != nil {
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to update bundle ID: %s", err))
        return
    }

    plan.Name = types.StringValue(out.Data.Attributes.Name)
    resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
```

`identifier` and `platform` are not sent here: both would be marked `RequiresReplace` in the schema (see `references/guides/plan-modification.md`), since the App Store Connect API has no operation to change either after creation.

## Delete

Contract:

- Read prior state from `req.State`
- Perform the API deletion
- If already deleted: return without error (idempotent)
- No need to modify state: framework removes it automatically on success

```go
func (r *bundleIdResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
    var state bundleIdResourceModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() {
        return
    }

    if err := r.client.delete(ctx, "/v1/bundleIds/"+state.Id.ValueString(), nil); err != nil {
        if isNotFound(err) {
            return
        }
        resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to delete bundle ID: %s", err))
        return
    }
}
```

Some App Store Connect types have no delete operation at all. For those, Delete should return an error explaining that the resource must be removed by hand (or whatever the API does support), and the resource's schema description should say so up front.

## Decoding JSON:API Errors

Unlike a hand-rolled `Success bool` / `Message string` pair, every non-2xx response is already turned into a Go `error` inside `apiClient.send` (`client.go`): it decodes the JSON:API `errors` array into `*apiError{StatusCode, Errors []apiErrorDetail}` (`errors.go`). CRUD methods never need to check a separate success flag, only the returned `error`:

```go
type apiErrorDetail struct {
    Status string `json:"status"`
    Code   string `json:"code"`
    Title  string `json:"title"`
    Detail string `json:"detail"`
}

func hasStatus(err error, status int) bool {
    var apiErr *apiError
    return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

func isNotFound(err error) bool {
    return hasStatus(err, http.StatusNotFound)
}
```

`errors.As` is required (not a direct type assertion) because an error returned from a CRUD method may be wrapped, e.g. with `fmt.Errorf("...: %w", err)`.

## Related Framework References

| File                                | Contents                                  |
| ------------------------------------ | -------------------------------------------- |
| `framework/resources/index.mdx`     | Resource type definition, full interface  |
| `framework/resources/create.mdx`    | Create method details and caveats         |
| `framework/resources/read.mdx`      | Read method and state refresh             |
| `framework/resources/update.mdx`    | Update method and plan consistency        |
| `framework/resources/delete.mdx`    | Delete method                             |
| `framework/resources/configure.mdx` | Configure method, provider data injection |
