# terraform-provider-appstoreconnect

**A Terraform provider for the App Store Connect API.** It authenticates with an App Store Connect API key and is the foundation for managing App Store Connect and Apple Developer configuration as code.

**The provider turns console clicks into reviewed configuration.** Bundle IDs, capabilities, registered devices, TestFlight groups and team invitations are usually set up by hand in App Store Connect and the Apple Developer website, which leaves no record a code review or a rollout pipeline can rely on. Expressing them as Terraform configuration puts those changes under review and makes them reproducible across apps and teams.

**Resources are added one at a time, as they are needed.** This release ships the provider configuration, authentication and API client; no resources or data sources are exposed yet.

## Provider configuration

```hcl
provider "appstoreconnect" {
  issuer_id   = "00000000-0000-0000-0000-000000000000"
  key_id      = "ABC123DEFG"
  private_key = file("AuthKey_ABC123DEFG.p8")
}
```

| Attribute     | Environment variable            | Description                                                                  |
| ------------- | ------------------------------- | ---------------------------------------------------------------------------- |
| `issuer_id`   | `APP_STORE_CONNECT_ISSUER_ID`   | The issuer ID shown above the list of API keys in App Store Connect. Required. |
| `key_id`      | `APP_STORE_CONNECT_KEY_ID`      | The ID of the App Store Connect API key. Required.                            |
| `private_key` | `APP_STORE_CONNECT_PRIVATE_KEY` | The PEM contents of the key's `.p8` file. Required, sensitive.                |

Every attribute falls back to its environment variable when unset in configuration, and the provider fails with a diagnostic if a value is missing from both places.

Create the key in App Store Connect under Users and Access > Integrations > App Store Connect API. The provider signs every request with a short-lived ES256 token derived from the key and signs a new one shortly before it expires. The role assigned to the key limits what the provider can manage.

## Resources

None yet.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup, test workflow, and release process.
