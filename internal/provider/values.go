package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type bundleIdentifier string

var bundleIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*$`)

func parseBundleIdentifier(raw string) (bundleIdentifier, error) {
	if !bundleIdentifierPattern.MatchString(raw) {
		return "", fmt.Errorf("%q is not a bundle identifier: use reverse-DNS segments of letters, digits and hyphens separated by periods", raw)
	}
	return bundleIdentifier(raw), nil
}

type bundleIdentifierValidator struct{}

func (bundleIdentifierValidator) Description(context.Context) string {
	return "must be a reverse-DNS bundle identifier of letters, digits, hyphens and periods"
}

func (v bundleIdentifierValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (bundleIdentifierValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := parseBundleIdentifier(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Bundle Identifier", err.Error())
	}
}
