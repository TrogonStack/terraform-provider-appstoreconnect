package provider

import (
	"context"
	"net/url"
)

type betaGroupAttributes struct {
	Name                                 string `json:"name"`
	CreatedDate                          string `json:"createdDate"`
	IsInternalGroup                      bool   `json:"isInternalGroup"`
	HasAccessToAllBuilds                 bool   `json:"hasAccessToAllBuilds"`
	PublicLinkEnabled                    bool   `json:"publicLinkEnabled"`
	PublicLinkID                         string `json:"publicLinkId"`
	PublicLinkLimitEnabled               bool   `json:"publicLinkLimitEnabled"`
	PublicLinkLimit                      int64  `json:"publicLinkLimit"`
	PublicLink                           string `json:"publicLink"`
	FeedbackEnabled                      bool   `json:"feedbackEnabled"`
	IosBuildsAvailableForAppleSiliconMac bool   `json:"iosBuildsAvailableForAppleSiliconMac"`
	IosBuildsAvailableForAppleVision     bool   `json:"iosBuildsAvailableForAppleVision"`
}

type betaGroupRelationships struct {
	App toOneRelationship `json:"app"`
}

type betaGroupResource struct {
	Type          resourceType           `json:"type"`
	ID            string                 `json:"id"`
	Attributes    betaGroupAttributes    `json:"attributes"`
	Relationships betaGroupRelationships `json:"relationships"`
}

type betaGroupCreateAttributes struct {
	Name                   string `json:"name"`
	IsInternalGroup        *bool  `json:"isInternalGroup,omitempty"`
	HasAccessToAllBuilds   *bool  `json:"hasAccessToAllBuilds,omitempty"`
	PublicLinkEnabled      *bool  `json:"publicLinkEnabled,omitempty"`
	PublicLinkLimitEnabled *bool  `json:"publicLinkLimitEnabled,omitempty"`
	PublicLinkLimit        *int64 `json:"publicLinkLimit,omitempty"`
	FeedbackEnabled        *bool  `json:"feedbackEnabled,omitempty"`
}

type betaGroupCreate struct {
	Type          resourceType              `json:"type"`
	Attributes    betaGroupCreateAttributes `json:"attributes"`
	Relationships betaGroupRelationships    `json:"relationships"`
}

type betaGroupUpdateAttributes struct {
	Name                                 *string `json:"name,omitempty"`
	PublicLinkEnabled                    *bool   `json:"publicLinkEnabled,omitempty"`
	PublicLinkLimitEnabled               *bool   `json:"publicLinkLimitEnabled,omitempty"`
	PublicLinkLimit                      *int64  `json:"publicLinkLimit,omitempty"`
	FeedbackEnabled                      *bool   `json:"feedbackEnabled,omitempty"`
	IosBuildsAvailableForAppleSiliconMac *bool   `json:"iosBuildsAvailableForAppleSiliconMac,omitempty"`
	IosBuildsAvailableForAppleVision     *bool   `json:"iosBuildsAvailableForAppleVision,omitempty"`
}

type betaGroupUpdate struct {
	Type       resourceType              `json:"type"`
	ID         string                    `json:"id"`
	Attributes betaGroupUpdateAttributes `json:"attributes"`
}

func (c *apiClient) createBetaGroup(ctx context.Context, appID string, attributes betaGroupCreateAttributes) (*betaGroupResource, error) {
	out := &document[betaGroupResource]{}
	in := document[betaGroupCreate]{Data: betaGroupCreate{
		Type:       resourceTypeBetaGroups,
		Attributes: attributes,
		Relationships: betaGroupRelationships{
			App: toOneRelationship{Data: &resourceIdentifier{Type: resourceTypeApps, ID: appID}},
		},
	}}
	if err := c.post(ctx, "/v1/betaGroups", in, out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func (c *apiClient) getBetaGroup(ctx context.Context, id string) (*betaGroupResource, error) {
	out := &document[betaGroupResource]{}
	if err := c.get(ctx, "/v1/betaGroups/"+url.PathEscape(id), url.Values{"include": {"app"}}, out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func (c *apiClient) updateBetaGroup(ctx context.Context, id string, attributes betaGroupUpdateAttributes) (*betaGroupResource, error) {
	out := &document[betaGroupResource]{}
	in := document[betaGroupUpdate]{Data: betaGroupUpdate{Type: resourceTypeBetaGroups, ID: id, Attributes: attributes}}
	if err := c.patch(ctx, "/v1/betaGroups/"+url.PathEscape(id), in, out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func (c *apiClient) deleteBetaGroup(ctx context.Context, id string) error {
	return c.delete(ctx, "/v1/betaGroups/"+url.PathEscape(id), nil)
}
