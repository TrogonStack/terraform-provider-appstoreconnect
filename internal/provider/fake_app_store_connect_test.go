package provider

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const fakePageSize = 2

type fakeAppStoreConnect struct {
	mu        sync.Mutex
	publicKey *ecdsa.PublicKey
	keyID     string
	nextID    int

	apps       map[string]appAttributes
	betaGroups map[string]*fakeBetaGroup

	deleteReportsNotFound bool
	lastBetaGroupUpdate   betaGroupUpdateAttributes
	betaGroupReads        int
}

type fakeBetaGroup struct {
	appID      string
	attributes betaGroupAttributes
}

func newFakeAppStoreConnect(credentials apiCredentials) *fakeAppStoreConnect {
	return &fakeAppStoreConnect{
		publicKey:  &credentials.privateKey.PublicKey,
		keyID:      credentials.keyID,
		apps:       map[string]appAttributes{},
		betaGroups: map[string]*fakeBetaGroup{},
	}
}

func setupFake(t *testing.T) *fakeAppStoreConnect {
	t.Helper()
	credentials, _ := newTestCredentials(t)
	fake := newFakeAppStoreConnect(credentials)
	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)
	testAPIClient = newAPIClient(server.URL, server.Client(), newTokenSource(credentials))
	t.Cleanup(func() { testAPIClient = nil })
	return fake
}

func (f *fakeAppStoreConnect) addApp(attributes appAttributes) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.newID("APP")
	f.apps[id] = attributes
	return id
}

func (f *fakeAppStoreConnect) newID(prefix string) string {
	f.nextID++
	return fmt.Sprintf("%s%08d", prefix, f.nextID)
}

func (f *fakeAppStoreConnect) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/apps", f.listApps)

	mux.HandleFunc("POST /v1/betaGroups", f.createBetaGroup)
	mux.HandleFunc("GET /v1/betaGroups/{id}", f.getBetaGroup)
	mux.HandleFunc("PATCH /v1/betaGroups/{id}", f.updateBetaGroup)
	mux.HandleFunc("DELETE /v1/betaGroups/{id}", f.deleteBetaGroup)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("no route for %s %s", r.Method, r.URL.Path))
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !f.authorized(r) {
			writeError(w, http.StatusUnauthorized, "NOT_AUTHORIZED", "missing or invalid bearer token")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (f *fakeAppStoreConnect) authorized(r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	return verifyTestToken(token, f.publicKey, f.keyID) == nil
}

func writeError(w http.ResponseWriter, status int, code, detail string) {
	writeJSON(w, status, map[string]any{"errors": []apiErrorDetail{{
		Status: strconv.Itoa(status),
		Code:   code,
		Title:  http.StatusText(status),
		Detail: detail,
	}}})
}

func writeAttributeConflict(w http.ResponseWriter, attribute, detail string) {
	writeJSON(w, http.StatusConflict, map[string]any{"errors": []apiErrorDetail{{
		Status: strconv.Itoa(http.StatusConflict),
		Code:   "ENTITY_ERROR.ATTRIBUTE.INVALID",
		Title:  "An attribute value is invalid.",
		Detail: detail,
		Source: &apiErrorSource{Pointer: "/data/attributes/" + attribute},
	}}})
}

func (f *fakeAppStoreConnect) betaGroupNameTaken(appID, name, exceptID string) bool {
	for id, group := range f.betaGroups {
		if id != exceptID && group.appID == appID && group.attributes.Name == name {
			return true
		}
	}
	return false
}

func writeNotFound(w http.ResponseWriter, kind, id string) {
	writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("There is no resource of type '%s' with id '%s'", kind, id))
}

func readBody(w http.ResponseWriter, r *http.Request, out any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		writeError(w, http.StatusBadRequest, "PARAMETER_ERROR.INVALID", err.Error())
		return false
	}
	return true
}

func writePage[T any](w http.ResponseWriter, r *http.Request, items []T) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
	end := min(offset+fakePageSize, len(items))
	links := map[string]string{"self": "http://" + r.Host + r.URL.RequestURI()}
	if end < len(items) {
		query := r.URL.Query()
		query.Set("cursor", strconv.Itoa(end))
		next := url.URL{Scheme: "http", Host: r.Host, Path: r.URL.Path, RawQuery: query.Encode()}
		links["next"] = next.String()
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items[min(offset, len(items)):end], "links": links})
}

func (f *fakeAppStoreConnect) listApps(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	filter := r.URL.Query().Get("filter[bundleId]")
	out := []appResource{}
	for _, id := range slices.Sorted(maps.Keys(f.apps)) {
		app := f.apps[id]
		if filter == "" || strings.HasPrefix(string(app.BundleID), filter) {
			out = append(out, appResource{ID: id, Attributes: app})
		}
	}
	writePage(w, r, out)
}

func (f *fakeAppStoreConnect) betaGroupResource(id string, includeApp bool) betaGroupResource {
	group := f.betaGroups[id]
	out := betaGroupResource{Type: resourceTypeBetaGroups, ID: id, Attributes: group.attributes}
	if includeApp {
		out.Relationships.App.Data = &resourceIdentifier{Type: resourceTypeApps, ID: group.appID}
	}
	return out
}

func (f *fakeAppStoreConnect) createBetaGroup(w http.ResponseWriter, r *http.Request) {
	var in document[betaGroupCreate]
	if !readBody(w, r, &in) {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if in.Data.Type != resourceTypeBetaGroups || in.Data.Relationships.App.Data == nil {
		writeError(w, http.StatusUnprocessableEntity, "ENTITY_ERROR.RELATIONSHIP.REQUIRED", "type betaGroups and an app relationship are required")
		return
	}
	appID := in.Data.Relationships.App.Data.ID
	if _, ok := f.apps[appID]; !ok {
		writeNotFound(w, "apps", appID)
		return
	}
	a := in.Data.Attributes
	if f.betaGroupNameTaken(appID, a.Name, "") {
		writeAttributeConflict(w, "name", "A beta group with this name already exists for the app.")
		return
	}
	attributes := betaGroupAttributes{Name: a.Name, CreatedDate: "2026-01-01T00:00:00Z", FeedbackEnabled: true}
	setIfPresent(&attributes.IsInternalGroup, a.IsInternalGroup)
	setIfPresent(&attributes.HasAccessToAllBuilds, a.HasAccessToAllBuilds)
	setIfPresent(&attributes.PublicLinkEnabled, a.PublicLinkEnabled)
	setIfPresent(&attributes.PublicLinkLimitEnabled, a.PublicLinkLimitEnabled)
	setIfPresent(&attributes.PublicLinkLimit, a.PublicLinkLimit)
	setIfPresent(&attributes.FeedbackEnabled, a.FeedbackEnabled)
	refreshFakePublicLink(&attributes)
	id := f.newID("BG")
	f.betaGroups[id] = &fakeBetaGroup{appID: appID, attributes: attributes}
	writeJSON(w, http.StatusCreated, document[betaGroupResource]{Data: f.betaGroupResource(id, false)})
}

func (f *fakeAppStoreConnect) getBetaGroup(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.betaGroupReads++
	id := r.PathValue("id")
	if _, ok := f.betaGroups[id]; !ok {
		writeNotFound(w, "betaGroups", id)
		return
	}
	writeJSON(w, http.StatusOK, document[betaGroupResource]{Data: f.betaGroupResource(id, r.URL.Query().Get("include") == "app")})
}

func (f *fakeAppStoreConnect) updateBetaGroup(w http.ResponseWriter, r *http.Request) {
	var in document[betaGroupUpdate]
	if !readBody(w, r, &in) {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	id := r.PathValue("id")
	group, ok := f.betaGroups[id]
	if !ok {
		writeNotFound(w, "betaGroups", id)
		return
	}
	if in.Data.ID != id || in.Data.Type != resourceTypeBetaGroups {
		writeError(w, http.StatusConflict, "ENTITY_ERROR.ATTRIBUTE.INVALID", "the body does not identify this beta group")
		return
	}
	a := in.Data.Attributes
	if a.Name != nil && f.betaGroupNameTaken(group.appID, *a.Name, id) {
		writeAttributeConflict(w, "name", "A beta group with this name already exists for the app.")
		return
	}
	f.lastBetaGroupUpdate = a
	setIfPresent(&group.attributes.Name, a.Name)
	setIfPresent(&group.attributes.PublicLinkEnabled, a.PublicLinkEnabled)
	setIfPresent(&group.attributes.PublicLinkLimitEnabled, a.PublicLinkLimitEnabled)
	setIfPresent(&group.attributes.PublicLinkLimit, a.PublicLinkLimit)
	setIfPresent(&group.attributes.FeedbackEnabled, a.FeedbackEnabled)
	setIfPresent(&group.attributes.IosBuildsAvailableForAppleSiliconMac, a.IosBuildsAvailableForAppleSiliconMac)
	setIfPresent(&group.attributes.IosBuildsAvailableForAppleVision, a.IosBuildsAvailableForAppleVision)
	refreshFakePublicLink(&group.attributes)
	writeJSON(w, http.StatusOK, document[betaGroupResource]{Data: f.betaGroupResource(id, false)})
}

func (f *fakeAppStoreConnect) deleteBetaGroup(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := r.PathValue("id")
	if _, ok := f.betaGroups[id]; !ok {
		writeNotFound(w, "betaGroups", id)
		return
	}
	delete(f.betaGroups, id)
	if f.deleteReportsNotFound {
		writeNotFound(w, "betaGroups", id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func setIfPresent[T any](target *T, value *T) {
	if value != nil {
		*target = *value
	}
}

func refreshFakePublicLink(attributes *betaGroupAttributes) {
	if attributes.PublicLinkEnabled {
		attributes.PublicLinkID = "abcdEFGH"
		attributes.PublicLink = "https://testflight.example.com/join/abcdEFGH"
		return
	}
	attributes.PublicLinkID = ""
	attributes.PublicLink = ""
}
