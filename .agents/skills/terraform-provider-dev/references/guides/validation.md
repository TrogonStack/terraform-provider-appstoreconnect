# Validation

## Overview

Validation runs during `terraform validate`, `terraform plan`, and `terraform apply`. It returns diagnostics (warnings/errors) before any API calls happen. Validation occurs at two levels:

1. **Attribute-level validators**: validate individual attribute values
2. **Resource/data-source-level validation**: cross-attribute validation logic

Important: configuration values may be unknown during validation (references to other resources). Validators must handle this by returning early without diagnostics.

This provider has no resources yet, so no attribute validator exists in real code today. Everything below is illustrative, built around a hypothetical `appstoreconnect_bundle_id` resource.

## Attribute Validators

Add validators to any attribute's `Validators` field. All validators in the slice always run (no short-circuit).

```go
import (
    "github.com/hashicorp/terraform-plugin-framework/schema/validator"
    "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
)

schema.StringAttribute{
    Required:            true,
    MarkdownDescription: "The platform this bundle ID is for. One of `IOS`, `MAC_OS`, or `UNIVERSAL`.",
    Validators: []validator.String{
        stringvalidator.OneOf("IOS", "MAC_OS", "UNIVERSAL"),
    },
}
```

The `stringvalidator.OneOf` constraint mirrors whatever App Store Connect's OpenAPI specification says the `platform` enum accepts; the validator gives the practitioner that error earlier, at plan time, instead of waiting for a 4xx response.

## Common Validators (terraform-plugin-framework-validators)

### String

| Validator                                     | Description           |
| ------------------------------------------------- | ------------------------ |
| `stringvalidator.LengthBetween(min, max)`     | String length range   |
| `stringvalidator.LengthAtLeast(min)`          | Minimum length        |
| `stringvalidator.LengthAtMost(max)`           | Maximum length        |
| `stringvalidator.RegexMatches(re, msg)`       | Regex pattern match   |
| `stringvalidator.OneOf("a", "b", "c")`        | Enum values           |
| `stringvalidator.NoneOf("x", "y")`            | Excluded values       |
| `stringvalidator.UTF8LengthBetween(min, max)` | UTF-8 character count |
| `stringvalidator.IsURLWithHTTPS()`            | Valid HTTPS URL       |

### Int64

| Validator                          | Description       |
| -------------------------------------- | -------------------- |
| `int64validator.Between(min, max)` | Range (inclusive) |
| `int64validator.AtLeast(min)`      | Minimum           |
| `int64validator.AtMost(max)`       | Maximum           |
| `int64validator.OneOf(1, 2, 3)`    | Enum values       |

### Bool

| Validator                    | Description  |
| --------------------------------- | -------------- |
| `boolvalidator.Equals(true)` | Must be true |

### List/Set/Map

| Validator                             | Description           |
| ------------------------------------------ | ------------------------ |
| `listvalidator.SizeAtLeast(min)`      | Minimum element count |
| `listvalidator.SizeAtMost(max)`       | Maximum element count |
| `listvalidator.SizeBetween(min, max)` | Element count range   |
| `listvalidator.UniqueValues()`        | No duplicate elements |

`setvalidator` has the same shapes, for a future Set-typed attribute.

## Conflict/Dependency Validators

Express relationships between attributes:

```go
// Exactly one of these must be set
schema.StringAttribute{
    Optional: true,
    Validators: []validator.String{
        stringvalidator.ExactlyOneOf(
            path.MatchRoot("field_a"),
            path.MatchRoot("field_b"),
        ),
    },
}

// At least one of these must be set
schema.StringAttribute{
    Optional: true,
    Validators: []validator.String{
        stringvalidator.AtLeastOneOf(
            path.MatchRoot("field_a"),
            path.MatchRoot("field_b"),
        ),
    },
}

// These conflict (cannot both be set)
schema.StringAttribute{
    Optional: true,
    Validators: []validator.String{
        stringvalidator.ConflictsWith(
            path.MatchRoot("other_field"),
        ),
    },
}

// Required together (if one is set, all must be set)
schema.StringAttribute{
    Optional: true,
    Validators: []validator.String{
        stringvalidator.AlsoRequires(
            path.MatchRoot("other_field"),
        ),
    },
}
```

Not used anywhere in this provider today.

## Custom Validators

Implement the `validator.<Type>` interface. Not used anywhere in this provider today; the shape below is illustrative, for a bundle identifier that must look like a reverse-DNS string:

```go
type bundleIdentifierValidator struct{}

func (v bundleIdentifierValidator) Description(_ context.Context) string {
    return "value must be a dotted reverse-DNS identifier, e.g. com.example.app"
}

func (v bundleIdentifierValidator) MarkdownDescription(_ context.Context) string {
    return "value must be a dotted reverse-DNS identifier, e.g. `com.example.app`"
}

func (v bundleIdentifierValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
    if req.ConfigValue.IsUnknown() || req.ConfigValue.IsNull() {
        return
    }

    value := req.ConfigValue.ValueString()
    if !strings.Contains(value, ".") {
        resp.Diagnostics.AddAttributeError(
            req.Path,
            "Invalid Bundle Identifier",
            fmt.Sprintf("%q is not a valid reverse-DNS bundle identifier", value),
        )
    }
}

// Usage:
schema.StringAttribute{
    Required:   true,
    Validators: []validator.String{bundleIdentifierValidator{}},
}
```

## Resource-Level Validation

For cross-attribute validation that requires access to multiple fields. Not used anywhere in this provider today; the shape below is illustrative:

```go
var _ resource.ResourceWithValidateConfig = &bundleIdResource{}

func (r *bundleIdResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
    var data bundleIdResourceModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
    if resp.Diagnostics.HasError() {
        return
    }

    // Skip validation if values are unknown (references to other resources)
    if data.Platform.IsUnknown() || data.Name.IsUnknown() {
        return
    }

    if data.Platform.ValueString() == "MAC_OS" && data.Name.ValueString() == "" {
        resp.Diagnostics.AddAttributeError(
            path.Root("name"),
            "Missing Required Field",
            "name is required when platform is 'MAC_OS'",
        )
    }
}
```

## Diagnostics

### Error vs Warning

```go
// Error: blocks apply
resp.Diagnostics.AddError("Title", "Detail message")

// Warning: allows apply but notifies user
resp.Diagnostics.AddWarning("Title", "Detail message")

// Attribute-specific error (shows path in output)
resp.Diagnostics.AddAttributeError(path.Root("name"), "Title", "Detail")

// Check for errors before continuing
if resp.Diagnostics.HasError() {
    return
}
```

The provider's own `Configure` is the only real code using this today, via `resp.Diagnostics.AddError("Configuration Error", ...)` for a missing or invalid credential (see `references/guides/provider-configuration.md`). A future resource's CRUD methods would use `resp.Diagnostics.AddError("API Error", ...)` the same way, as shown in `references/guides/resource-lifecycle.md`.

## Related Framework References

| File                                                | Contents                      |
| -------------------------------------------------------- | -------------------------------- |
| `framework/validation.mdx`                          | Full validation documentation |
| `framework/diagnostics.mdx`                         | Diagnostics (errors/warnings) |
| `framework/resources/validate-configuration.mdx`    | Resource-level validation     |
| `framework/data-sources/validate-configuration.mdx` | Data source validation        |
| `framework/providers/validate-configuration.mdx`    | Provider-level validation     |
