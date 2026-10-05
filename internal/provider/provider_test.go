package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"appstoreconnect": providerserver.NewProtocol6WithError(New("test")()),
}

const testProviderConfig = `
provider "appstoreconnect" {}
`

func configureProvider(t *testing.T, config map[string]string) *provider.ConfigureResponse {
	t.Helper()
	ctx := context.Background()
	p := New("test")()

	schemaResp := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema has errors: %v", schemaResp.Diagnostics)
	}

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

func TestProviderConfigure_FromAttributes(t *testing.T) {
	clearCredentialEnv(t)
	_, privateKeyPEM := newTestCredentials(t)

	resp := configureProvider(t, map[string]string{
		"issuer_id":   testIssuerID,
		"key_id":      testKeyID,
		"private_key": privateKeyPEM,
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected configure to succeed, got %v", resp.Diagnostics)
	}
	client, ok := resp.ResourceData.(*apiClient)
	if !ok || client.baseURL != defaultBaseURL {
		t.Fatalf("expected an API client for %s, got %#v", defaultBaseURL, resp.ResourceData)
	}
	if resp.DataSourceData != resp.ResourceData {
		t.Fatal("expected data sources and resources to share the client")
	}
}

func TestProviderConfigure_FromEnvironment(t *testing.T) {
	_, privateKeyPEM := newTestCredentials(t)
	t.Setenv("APP_STORE_CONNECT_ISSUER_ID", testIssuerID)
	t.Setenv("APP_STORE_CONNECT_KEY_ID", testKeyID)
	t.Setenv("APP_STORE_CONNECT_PRIVATE_KEY", privateKeyPEM)

	resp := configureProvider(t, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected configure to succeed from the environment, got %v", resp.Diagnostics)
	}
	if _, ok := resp.ResourceData.(*apiClient); !ok {
		t.Fatalf("expected an API client, got %#v", resp.ResourceData)
	}
}

func TestProviderConfigure_Errors(t *testing.T) {
	_, privateKeyPEM := newTestCredentials(t)

	cases := []struct {
		name   string
		config map[string]string
		want   string
	}{
		{"missing issuer", map[string]string{"key_id": testKeyID, "private_key": privateKeyPEM}, "issuer_id must be set"},
		{"missing key ID", map[string]string{"issuer_id": testIssuerID, "private_key": privateKeyPEM}, "key_id must be set"},
		{"missing private key", map[string]string{"issuer_id": testIssuerID, "key_id": testKeyID}, "private_key must be set"},
		{"invalid private key", map[string]string{"issuer_id": testIssuerID, "key_id": testKeyID, "private_key": "not a key"}, "private key is not PEM encoded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearCredentialEnv(t)
			resp := configureProvider(t, tc.config)
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected configure to fail")
			}
			got := resp.Diagnostics.Errors()[0].Detail()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected an error containing %q, got %q", tc.want, got)
			}
		})
	}
}

func TestProviderConfigure_UsesTestClient(t *testing.T) {
	clearCredentialEnv(t)
	credentials, _ := newTestCredentials(t)
	testAPIClient = newAPIClient("http://127.0.0.1", nil, newTokenSource(credentials))
	t.Cleanup(func() { testAPIClient = nil })

	resp := configureProvider(t, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected the injected client to bypass credentials, got %v", resp.Diagnostics)
	}
	if resp.ResourceData != testAPIClient {
		t.Fatal("expected the injected test client to be used")
	}
}
