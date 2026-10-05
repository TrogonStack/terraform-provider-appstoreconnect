# terraform-provider-appstoreconnect

Always use `/claude-md-improver` when updating this file.

Terraform provider for the App Store Connect API. It currently ships the provider configuration, authentication and API client; resources are added one at a time as they are needed.

- **Module**: `github.com/TrogonStack/terraform-provider-appstoreconnect`
- **Package**: `internal/provider/` (single flat package, all resources here)

## Commands

```bash
mise run test          # go test -count=1 -cover -skip TestLive ./...
mise run test:live     # TestLive_* against a real App Store Connect team
mise run lint          # golangci-lint run --fix ./...
mise run build         # full CI pipeline (download, tidy, lint, test, docs, diff)
mise run docs          # regenerate docs/ from schema descriptions
```

Single test:

```bash
go test ./internal/provider/ -v -run TestClient
```

Runtime credentials the provider itself needs (not required to run the unit tests, which never call a real API):

- `APP_STORE_CONNECT_ISSUER_ID`: the issuer ID of the team's API keys
- `APP_STORE_CONNECT_KEY_ID`: the API key ID, sent as the token's `kid`
- `APP_STORE_CONNECT_PRIVATE_KEY`: the PEM contents of the key's `.p8` file

## Skills

Always load `terraform-provider-dev` when working on a resource or data source.

## Architecture

- File naming: `resource_<name>.go`, `resource_<name>_test.go`, `data_source_<name>.go`
- Provider client: `*apiClient` in `client.go` is a small hand-written JSON:API client (`get`, `post`, `patch`, `delete`, and `listAll[T]` for pagination). No Go SDK for this API is both maintained and permissively licensed, so none is used
- Auth: `jwt.go` signs an ES256 token (`aud` `appstoreconnect-v1`) with the API key, caches it in `tokenSource` for 15 minutes and re-signs it 2 minutes before expiry
- New resources must be registered in `provider.go` `Resources()` / `DataSources()`

## Conventions

### Source of truth

Model every resource on Apple's App Store Connect API documentation and its OpenAPI specification. Never add fields or behaviors the specification does not describe; when it is ambiguous, say so in the resource description.

### Resource naming

Use the same terminology as the API. A resource for `bundleIds` is `appstoreconnect_bundle_id`, and attribute names are the snake_case form of the API's attribute names.

### Not-found handling

The API reports a missing object as HTTP 404 with a JSON:API `errors` array, which `client.go` decodes into `*apiError`. Detect it with the shared `isNotFound(err)` helper in `errors.go`, which unwraps with `errors.As`, since a wrapped error would otherwise fail a direct type check.

- **Read**: call `resp.State.RemoveResource(ctx)` and return (resource was deleted externally)
- **Delete**: return without error (idempotent)

### Missing operations

Several types have no update or delete operation. Mark attributes that cannot be updated with `RequiresReplace`, and when there is no delete, document what destroy does instead in the resource description.

### Pagination

List endpoints return `links.next`. Use `listAll[T]`, which follows it and refuses a link to any host other than the configured base URL.

### Testing

Tests never reach App Store Connect:

- `helpers_test.go`: `newTestCredentials` generates a P-256 key per test; `verifyTestToken` checks a bearer token's signature, header and audience
- Point a real `apiClient` at an `httptest.Server` with `newAPIClient(server.URL, server.Client(), newTokenSource(credentials))`
- Resource tests inject that client as `testAPIClient`, bypassing provider configuration entirely, and back it with an in-memory fake of the endpoints the resource calls
- `TestLive_*` tests skip unless `TF_ACC` and the three `APP_STORE_CONNECT_*` variables are set

### Context propagation

Every `apiClient` method takes `ctx` as its first argument and builds its request with `http.NewRequestWithContext`, so passing the CRUD method's own `ctx` is enough for Terraform cancellation to reach in-flight requests.

### Retry

Automatic retry on 429 and 5xx except 501, honoring `Retry-After`. No configuration attribute: the transport in `retry.go` is fixed.

## CI

- PR: lint + test + build (GitHub Actions)
- Release: release-please + goreleaser on push to main
