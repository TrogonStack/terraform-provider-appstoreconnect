package provider

import (
	"context"
	"net/url"
)

type appAttributes struct {
	Name          string           `json:"name"`
	BundleID      bundleIdentifier `json:"bundleId"`
	SKU           string           `json:"sku"`
	PrimaryLocale string           `json:"primaryLocale"`
}

type appResource struct {
	ID         string        `json:"id"`
	Attributes appAttributes `json:"attributes"`
}

func (c *apiClient) findAppsByBundleID(ctx context.Context, bundleID bundleIdentifier) ([]appResource, error) {
	candidates, err := listAll[appResource](ctx, c, "/v1/apps", url.Values{
		"filter[bundleId]": {string(bundleID)},
		"fields[apps]":     {"name,bundleId,sku,primaryLocale"},
		"limit":            {"200"},
	})
	if err != nil {
		return nil, err
	}
	matches := []appResource{}
	for _, app := range candidates {
		if app.Attributes.BundleID == bundleID {
			matches = append(matches, app)
		}
	}
	return matches, nil
}
