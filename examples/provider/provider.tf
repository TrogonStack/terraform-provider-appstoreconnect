terraform {
  required_providers {
    appstoreconnect = {
      source = "trogonstack/appstoreconnect"
    }
  }
}

provider "appstoreconnect" {
  issuer_id   = "00000000-0000-0000-0000-000000000000"
  key_id      = "ABC123DEFG"
  private_key = file("AuthKey_ABC123DEFG.p8")
}
