package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func appConfig(bundleID string) string {
	return testProviderConfig + `
data "appstoreconnect_app" "test" {
  bundle_id = "` + bundleID + `"
}
`
}

func TestAccAppDataSource(t *testing.T) {
	fake := setupFake(t)
	fake.addApp(appAttributes{Name: "Example Lite", BundleID: "com.example.app.lite", SKU: "EXAMPLE-LITE", PrimaryLocale: "en-US"})
	fake.addApp(appAttributes{Name: "Example Pro", BundleID: "com.example.app.pro", SKU: "EXAMPLE-PRO", PrimaryLocale: "en-US"})
	appID := fake.addApp(appAttributes{Name: "Example", BundleID: "com.example.app", SKU: "EXAMPLE", PrimaryLocale: "en-GB"})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: appConfig("com.example.app"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.appstoreconnect_app.test", "id", appID),
					resource.TestCheckResourceAttr("data.appstoreconnect_app.test", "bundle_id", "com.example.app"),
					resource.TestCheckResourceAttr("data.appstoreconnect_app.test", "name", "Example"),
					resource.TestCheckResourceAttr("data.appstoreconnect_app.test", "sku", "EXAMPLE"),
					resource.TestCheckResourceAttr("data.appstoreconnect_app.test", "primary_locale", "en-GB"),
				),
			},
		},
	})
}

func TestAccAppDataSource_NotFound(t *testing.T) {
	fake := setupFake(t)
	fake.addApp(appAttributes{Name: "Example Pro", BundleID: "com.example.app.pro"})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      appConfig("com.example.app"),
				ExpectError: regexp.MustCompile(`App Not Found`),
			},
		},
	})
}

func TestAccAppDataSource_MultipleMatches(t *testing.T) {
	fake := setupFake(t)
	fake.addApp(appAttributes{Name: "Example", BundleID: "com.example.app"})
	fake.addApp(appAttributes{Name: "Example Copy", BundleID: "com.example.app"})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      appConfig("com.example.app"),
				ExpectError: regexp.MustCompile(`Multiple Apps Found`),
			},
		},
	})
}

func TestAccAppDataSource_InvalidBundleID(t *testing.T) {
	setupFake(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      appConfig("com.example.*"),
				ExpectError: regexp.MustCompile(`Invalid Bundle Identifier`),
			},
		},
	})
}

func TestParseBundleIdentifier(t *testing.T) {
	for _, valid := range []string{"com.example.app", "com.example.app-beta", "Example"} {
		if _, err := parseBundleIdentifier(valid); err != nil {
			t.Errorf("expected %q to be valid, got %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "com.example.*", "com..example", ".com.example", "com.example.", "com.example app"} {
		if _, err := parseBundleIdentifier(invalid); err == nil {
			t.Errorf("expected %q to be rejected", invalid)
		}
	}
}
