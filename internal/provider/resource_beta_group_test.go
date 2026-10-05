package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func betaGroupConfig(appID, name string, feedback bool) string {
	return testProviderConfig + fmt.Sprintf(`
resource "appstoreconnect_beta_group" "test" {
  app_id                    = %q
  name                      = %q
  public_link_enabled       = true
  public_link_limit_enabled = true
  public_link_limit         = 100
  feedback_enabled          = %t
}
`, appID, name, feedback)
}

func TestAccBetaGroup_Lifecycle(t *testing.T) {
	fake := setupFake(t)
	appID := fake.addApp(appAttributes{Name: "Example", BundleID: "com.example.app"})
	otherAppID := fake.addApp(appAttributes{Name: "Example Pro", BundleID: "com.example.app.pro"})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(_ *terraform.State) error {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			if len(fake.betaGroups) != 0 {
				return fmt.Errorf("expected every beta group to be deleted, %d remain", len(fake.betaGroups))
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: betaGroupConfig(appID, "Example Beta", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("appstoreconnect_beta_group.test", "id"),
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "app_id", appID),
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "name", "Example Beta"),
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "is_internal_group", "false"),
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "has_access_to_all_builds", "false"),
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "public_link_enabled", "true"),
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "public_link_limit_enabled", "true"),
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "public_link_limit", "100"),
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "feedback_enabled", "true"),
					resource.TestCheckResourceAttrSet("appstoreconnect_beta_group.test", "public_link"),
					resource.TestCheckResourceAttrSet("appstoreconnect_beta_group.test", "public_link_id"),
					resource.TestCheckResourceAttrSet("appstoreconnect_beta_group.test", "created_date"),
				),
			},
			{
				Config: betaGroupConfig(appID, "Example Beta Renamed", false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("appstoreconnect_beta_group.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "name", "Example Beta Renamed"),
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "feedback_enabled", "false"),
					func(_ *terraform.State) error {
						fake.mu.Lock()
						defer fake.mu.Unlock()
						sent := fake.lastBetaGroupUpdate
						if sent.Name == nil || sent.FeedbackEnabled == nil || sent.PublicLinkEnabled != nil || sent.PublicLinkLimitEnabled != nil || sent.PublicLinkLimit != nil {
							return fmt.Errorf("expected only the changed name and feedback_enabled to be sent, got %+v", sent)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "appstoreconnect_beta_group.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: betaGroupConfig(otherAppID, "Example Beta Renamed", false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("appstoreconnect_beta_group.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "app_id", otherAppID),
			},
		},
	})
}

func TestAccBetaGroup_DisablePublicLink(t *testing.T) {
	fake := setupFake(t)
	appID := fake.addApp(appAttributes{Name: "Example", BundleID: "com.example.app"})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: betaGroupConfig(appID, "Example Beta", true),
				Check:  resource.TestCheckResourceAttrSet("appstoreconnect_beta_group.test", "public_link"),
			},
			{
				Config: testProviderConfig + fmt.Sprintf(`
resource "appstoreconnect_beta_group" "test" {
  app_id              = %q
  name                = "Example Beta"
  public_link_enabled = false
}
`, appID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "public_link_enabled", "false"),
					resource.TestCheckNoResourceAttr("appstoreconnect_beta_group.test", "public_link"),
					resource.TestCheckNoResourceAttr("appstoreconnect_beta_group.test", "public_link_id"),
				),
			},
		},
	})
}

func TestAccBetaGroup_InternalReplaces(t *testing.T) {
	fake := setupFake(t)
	appID := fake.addApp(appAttributes{Name: "Example", BundleID: "com.example.app"})

	internalConfig := func(internal bool) string {
		return testProviderConfig + fmt.Sprintf(`
resource "appstoreconnect_beta_group" "test" {
  app_id                   = %q
  name                     = "Example Beta"
  is_internal_group        = %t
  has_access_to_all_builds = %t
}
`, appID, internal, internal)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: internalConfig(false),
				Check:  resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "is_internal_group", "false"),
			},
			{
				Config: internalConfig(true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("appstoreconnect_beta_group.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "is_internal_group", "true"),
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "has_access_to_all_builds", "true"),
				),
			},
		},
	})
}

func TestAccBetaGroup_UpdateOnlyAttributesOnCreate(t *testing.T) {
	fake := setupFake(t)
	appID := fake.addApp(appAttributes{Name: "Example", BundleID: "com.example.app"})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig + fmt.Sprintf(`
resource "appstoreconnect_beta_group" "test" {
  app_id                                     = %q
  name                                       = "Example Beta"
  ios_builds_available_for_apple_silicon_mac = true
  ios_builds_available_for_apple_vision      = true
}
`, appID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "ios_builds_available_for_apple_silicon_mac", "true"),
					resource.TestCheckResourceAttr("appstoreconnect_beta_group.test", "ios_builds_available_for_apple_vision", "true"),
				),
			},
		},
	})
}

func TestAccBetaGroup_DeletedOutOfBand(t *testing.T) {
	fake := setupFake(t)
	appID := fake.addApp(appAttributes{Name: "Example", BundleID: "com.example.app"})
	config := betaGroupConfig(appID, "Example Beta", true)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				PreConfig: func() {
					fake.mu.Lock()
					defer fake.mu.Unlock()
					clear(fake.betaGroups)
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				Check:  resource.TestCheckResourceAttrSet("appstoreconnect_beta_group.test", "id"),
			},
		},
	})
}

func TestAccBetaGroup_DeleteIsIdempotent(t *testing.T) {
	fake := setupFake(t)
	appID := fake.addApp(appAttributes{Name: "Example", BundleID: "com.example.app"})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(_ *terraform.State) error {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			if len(fake.betaGroups) != 0 {
				return fmt.Errorf("expected every beta group to be deleted, %d remain", len(fake.betaGroups))
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: betaGroupConfig(appID, "Example Beta", true),
				Check: func(_ *terraform.State) error {
					fake.mu.Lock()
					defer fake.mu.Unlock()
					fake.deleteReportsNotFound = true
					return nil
				},
			},
		},
	})
}
