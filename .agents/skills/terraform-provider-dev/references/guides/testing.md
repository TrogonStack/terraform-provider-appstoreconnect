# Testing

## Overview

This provider's tests never reach real App Store Connect, except the `TestLive_*` tests gated behind `TF_ACC`. There are three layers today:

1. **Provider configuration tests** (`provider_test.go`): exercise `appStoreConnectProvider.Configure` directly, without building a Terraform config string or running `resource.Test`
2. **Client tests** (`client_test.go`): point a real `*apiClient` at an `httptest.Server`, verifying the JSON:API request/response handling, pagination, and JWT bearer-token signing
3. **Retry policy tests** (`retry_test.go`): plain unit tests of the `retryPolicy` function, no server involved

No resource or data source exists yet, so there is no `resource.Test`-based acceptance test harness. The final section below describes the pattern a future resource's tests would follow.

## Test Infrastructure

`helpers_test.go` has the shared building blocks:

```go
const (
    testIssuerID = "00000000-0000-0000-0000-000000000000"
    testKeyID    = "TESTKEY123"
)

func newTestCredentials(t *testing.T) (apiCredentials, string) {
    // Generates a fresh P-256 key, PEM-encodes it, and loads it through
    // newAPICredentials(testIssuerID, testKeyID, privateKeyPEM).
    // Returns both the parsed credentials and the PEM string.
}

func verifyTestToken(token string, publicKey *ecdsa.PublicKey, keyID string) error {
    // Splits the JWT into header.claims.signature, verifies the ECDSA
    // signature against publicKey, and checks alg/kid/typ and aud.
}
```

`writeJSON` is a small helper for writing a JSON body with a status code in test HTTP handlers.

## Provider Configuration Tests

`provider_test.go` builds a provider, drives `Configure` with a hand-built `tftypes.Value`, and asserts on the response, entirely without Terraform's test driver:

```go
func configureProvider(t *testing.T, config map[string]string) *provider.ConfigureResponse {
    t.Helper()
    ctx := context.Background()
    p := New("test")()

    schemaResp := &provider.SchemaResponse{}
    p.Schema(ctx, provider.SchemaRequest{}, schemaResp)

    values := map[string]tftypes.Value{}
    for _, name := range []string{"issuer_id", "key_id", "private_key"} {
        if v, ok := config[name]; ok {
            values[name] = tftypes.NewValue(tftypes.String, v)
        } else {
            values[name] = tftypes.NewValue(tftypes.String, nil)
        }
    }
    raw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), values)

    resp := &provider.ConfigureResponse{}
    p.Configure(ctx, provider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
    return resp
}

func clearCredentialEnv(t *testing.T) {
    t.Setenv("APP_STORE_CONNECT_ISSUER_ID", "")
    t.Setenv("APP_STORE_CONNECT_KEY_ID", "")
    t.Setenv("APP_STORE_CONNECT_PRIVATE_KEY", "")
}
```

Four tests build on this:

- `TestProviderConfigure_FromAttributes`: issuer_id/key_id/private_key set in config, asserts `resp.ResourceData` is an `*apiClient` pointed at `defaultBaseURL` and shared with `resp.DataSourceData`
- `TestProviderConfigure_FromEnvironment`: same, but via `APP_STORE_CONNECT_*` env vars and no config
- `TestProviderConfigure_Errors`: table-driven, one case per missing/invalid attribute, asserting the diagnostic detail contains the expected substring (`"issuer_id must be set"`, `"key_id must be set"`, `"private_key must be set"`, `"private key is not PEM encoded"`)
- `TestProviderConfigure_UsesTestClient`: sets the package-level `testAPIClient` and asserts Configure returns it unchanged, bypassing credential resolution entirely

## Client Tests

`client_test.go`'s `newTestServer` is the shared fixture: it spins up an `httptest.Server` that rejects requests without a validly signed bearer token, then delegates to the test's own handler:

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

Each test calls `newTestServer` with its own handler and gets back a ready-to-use `*apiClient`:

- `TestClient_SendsJSONWithSignedToken`: drives `get`/`post`/`patch`/`delete` against a handler that records method, path, content type, and body, asserting each verb round-trips correctly (query params encoded, JSON body sent, `DELETE` sends no body and no content type)
- `TestClient_PaginatesThroughNextLinks`: a handler that pages through a fixed set of items via a `cursor` query parameter, asserting `listAll[T]` collects every page
- `TestClient_RefusesNextLinkToAnotherHost`: a handler returning a `links.next` pointed at `https://other.example.com`, asserting `listAll[T]` returns an error containing `"another host"` instead of following it
- `TestClient_DecodesErrors`: a 404 JSON:API error body, asserting `isNotFound(err)` is true, `errors.As(err, &apiErr)` succeeds with the decoded detail, the error message carries the HTTP status and detail text, and that a wrapped `*apiError` is still detected by `isNotFound` while a plain, unrelated error is not
- `TestClient_RejectsForeignSignature`: builds a second client whose `tokenSource` holds different credentials than the server expects, asserting the server's own signature check rejects it with `hasStatus(err, http.StatusUnauthorized)`

## Retry Policy Tests

`retry_test.go` tests `retryPolicy` directly, with no server or fake involved, since it's a plain `func(context.Context, *http.Response, error) (bool, error)`:

```go
func TestRetryPolicy_429_Retries(t *testing.T) {
    resp := &http.Response{StatusCode: 429}
    retry, err := retryPolicy(context.Background(), resp, nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !retry {
        t.Error("expected retry on 429")
    }
}
```

The full table: `_429_Retries`, `_500_Retries`, `_502_Retries`, `_501_DoesNotRetry` (501 is not transient), `_200_DoesNotRetry`, `_404_DoesNotRetry`, `_ConnectionError_Retries` (a non-nil `err` with no response), `_CancelledContext_DoesNotRetry`.

## Live Acceptance Tests

`live_test.go` runs against a real App Store Connect team, skipped unless both `TF_ACC` and all three `APP_STORE_CONNECT_*` variables are set:

```go
func requireLiveCredentials(t *testing.T) {
    t.Helper()
    if os.Getenv("TF_ACC") == "" {
        t.Skip("set TF_ACC=1 to run live acceptance tests against a real App Store Connect team")
    }
    for _, name := range []string{"APP_STORE_CONNECT_ISSUER_ID", "APP_STORE_CONNECT_KEY_ID", "APP_STORE_CONNECT_PRIVATE_KEY"} {
        if os.Getenv(name) == "" {
            t.Skipf("set %s to run live acceptance tests against a real App Store Connect team", name)
        }
    }
}

func TestLive_Authenticates(t *testing.T) {
    requireLiveCredentials(t)
    credentials, err := newAPICredentials(
        os.Getenv("APP_STORE_CONNECT_ISSUER_ID"),
        os.Getenv("APP_STORE_CONNECT_KEY_ID"),
        os.Getenv("APP_STORE_CONNECT_PRIVATE_KEY"),
    )
    // ...builds a real client with newAPIClient(defaultBaseURL, newRetryableClient(), newTokenSource(credentials))
    // and asserts a GET /v1/apps round-trips successfully.
}
```

`mise run test` runs `go test -count=1 -cover -skip TestLive ./...`, so these never run by accident; `mise run test:live` runs them explicitly.

## Testing a Future Resource

No resource exists yet, so this is illustrative, following the same `httptest` style already used in `client_test.go` rather than a hand-rolled fake service interface. For a hypothetical `appstoreconnect_bundle_id`:

1. Stand up an `httptest.Server` with handlers for the `bundleIds` paths it calls (`POST /v1/bundleIds`, `GET /v1/bundleIds/{id}`, `PATCH /v1/bundleIds/{id}`, `DELETE /v1/bundleIds/{id}`), backed by an in-memory map, the same shape `newTestServer` already uses for token verification
2. Build a client against it with `newAPIClient(server.URL, server.Client(), newTokenSource(credentials))` and assign it to the package-level `testAPIClient`, bypassing provider `Configure`
3. Add a provider factory map and minimal provider config block, mirroring other Terraform Plugin Framework providers:

```go
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
    "appstoreconnect": providerserver.NewProtocol6WithError(New("test")()),
}

const testProviderConfig = `
provider "appstoreconnect" {
  issuer_id   = "test-issuer"
  key_id      = "test-key"
  private_key = "unused, testAPIClient bypasses this"
}
`
```

4. Drive `resource.Test` the usual way:

```go
func TestAccBundleId_Basic(t *testing.T) {
    // set up the httptest.Server and testAPIClient as described above

    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testProviderConfig + `
resource "appstoreconnect_bundle_id" "test" {
  identifier = "com.example.app"
  name       = "Example App"
  platform   = "IOS"
}
`,
                Check: resource.ComposeAggregateTestCheckFunc(
                    resource.TestCheckResourceAttrSet("appstoreconnect_bundle_id.test", "id"),
                    resource.TestCheckResourceAttr("appstoreconnect_bundle_id.test", "identifier", "com.example.app"),
                ),
            },
        },
    })
}
```

5. For a not-found/drift test, delete the item from the in-memory map between steps and use `PlanOnly` + `ExpectNonEmptyPlan`, the same way `client_test.go`'s fixtures are reset between cases
6. For import, add an `ImportState: true, ImportStateVerify: true` step; if a field cannot be reconstructed from a read (write-only, or not returned by the API), add it to `ImportStateVerifyIgnore`

## Check Functions

| Function                                                       | Use Case                     |
| ------------------------------------------------------------------ | --------------------------------- |
| `resource.TestCheckResourceAttr(name, key, value)`             | Exact attribute match        |
| `resource.TestCheckResourceAttrSet(name, key)`                 | Attribute is set (any value) |
| `resource.TestCheckNoResourceAttr(name, key)`                  | Attribute is NOT set         |
| `resource.TestCheckTypeSetElemAttr(name, key, value)`          | Element present in a Set attribute |
| `resource.TestCheckResourceAttrPair(name1, key1, name2, key2)` | Two attributes match         |
| `resource.ComposeAggregateTestCheckFunc(...)`                  | Combine multiple checks      |

## Running Tests

```bash
go test ./internal/provider/ -v -run TestClient
go test ./internal/provider/ -v -run TestProviderConfigure
go test ./internal/provider/ -v -run TestRetryPolicy

# Full suite, matching `mise run test` (skips TestLive_*)
go test -count=1 -cover -skip TestLive ./...

# Live acceptance tests against a real App Store Connect team
mise run test:live
```

## Test Naming Convention

```
Test<Subject>_<Scenario>
```

Examples from this provider: `TestClient_SendsJSONWithSignedToken`, `TestClient_PaginatesThroughNextLinks`, `TestClient_RefusesNextLinkToAnotherHost`, `TestClient_DecodesErrors`, `TestClient_RejectsForeignSignature`, `TestProviderConfigure_FromAttributes`, `TestProviderConfigure_FromEnvironment`, `TestProviderConfigure_Errors`, `TestProviderConfigure_UsesTestClient`, `TestRetryPolicy_429_Retries`, `TestLive_Authenticates`. A future resource's acceptance tests would follow `TestAcc<Resource>_<Scenario>` (e.g. `TestAccBundleId_Basic`), matching the Plugin Framework's own convention.

## Related Framework References

| File                      | Contents                             |
| --------------------------- | ----------------------------------------- |
| `framework/acctests.mdx`  | Acceptance test setup with framework |
| `framework/debugging.mdx` | Debugging test failures              |
