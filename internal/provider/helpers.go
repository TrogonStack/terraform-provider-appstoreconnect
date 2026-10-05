package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

func stringOrNull(v string) types.String {
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}
