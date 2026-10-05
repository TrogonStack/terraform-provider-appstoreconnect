data "appstoreconnect_app" "example" {
  bundle_id = "com.example.app"
}

resource "appstoreconnect_beta_group" "external" {
  app_id                    = data.appstoreconnect_app.example.id
  name                      = "Example Beta"
  public_link_enabled       = true
  public_link_limit_enabled = true
  public_link_limit         = 500
  feedback_enabled          = true
}
