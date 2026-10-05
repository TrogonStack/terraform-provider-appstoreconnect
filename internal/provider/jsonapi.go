package provider

type resourceType string

const (
	resourceTypeApps       resourceType = "apps"
	resourceTypeBetaGroups resourceType = "betaGroups"
)

type resourceIdentifier struct {
	Type resourceType `json:"type"`
	ID   string       `json:"id"`
}

type toOneRelationship struct {
	Data *resourceIdentifier `json:"data,omitempty"`
}

type document[T any] struct {
	Data T `json:"data"`
}
