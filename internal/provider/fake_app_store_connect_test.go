package provider

import (
	"crypto/ecdsa"
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

	apps map[string]appAttributes
}

func newFakeAppStoreConnect(credentials apiCredentials) *fakeAppStoreConnect {
	return &fakeAppStoreConnect{
		publicKey: &credentials.privateKey.PublicKey,
		keyID:     credentials.keyID,
		apps:      map[string]appAttributes{},
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
