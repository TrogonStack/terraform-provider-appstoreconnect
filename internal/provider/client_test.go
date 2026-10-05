package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

type testItem struct {
	ID string `json:"id"`
}

func newTestServer(t *testing.T, handler http.HandlerFunc) (*apiClient, apiCredentials) {
	t.Helper()
	credentials, _ := newTestCredentials(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || verifyTestToken(token, &credentials.privateKey.PublicKey, credentials.keyID) != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"errors": []apiErrorDetail{{Status: "401", Code: "NOT_AUTHORIZED", Title: "Unauthorized", Detail: "invalid bearer token"}}})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return newAPIClient(server.URL, server.Client(), newTokenSource(credentials)), credentials
}

func TestClient_SendsJSONWithSignedToken(t *testing.T) {
	var gotMethod, gotPath, gotContentType string
	var gotBody map[string]any
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotContentType = r.Method, r.URL.RequestURI(), r.Header.Get("Content-Type")
		gotBody = nil
		if r.Body != nil {
			raw, _ := io.ReadAll(r.Body)
			if len(raw) > 0 {
				_ = json.Unmarshal(raw, &gotBody)
			}
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": "ITEM1"}})
	})
	ctx := t.Context()

	var out struct {
		Data testItem `json:"data"`
	}
	if err := client.get(ctx, "/v1/items/ITEM1", map[string][]string{"include": {"owner"}}, &out); err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/v1/items/ITEM1?include=owner" || out.Data.ID != "ITEM1" {
		t.Fatalf("unexpected get round trip: %s %s -> %+v", gotMethod, gotPath, out)
	}

	body := map[string]any{"data": map[string]string{"type": "items"}}
	if err := client.post(ctx, "/v1/items", body, &out); err != nil {
		t.Fatalf("post failed: %v", err)
	}
	if gotMethod != http.MethodPost || gotContentType != "application/json" || gotBody["data"] == nil {
		t.Fatalf("unexpected post round trip: %s %s %v", gotMethod, gotContentType, gotBody)
	}

	if err := client.patch(ctx, "/v1/items/ITEM1", body, nil); err != nil {
		t.Fatalf("patch failed: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("expected PATCH, got %s", gotMethod)
	}

	if err := client.delete(ctx, "/v1/items/ITEM1", nil); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if gotMethod != http.MethodDelete || gotBody != nil {
		t.Fatalf("expected a DELETE without body, got %s %v", gotMethod, gotBody)
	}
}

func TestClient_PaginatesThroughNextLinks(t *testing.T) {
	const total = 5
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
		end := min(offset+2, total)
		data := []testItem{}
		for i := offset; i < end; i++ {
			data = append(data, testItem{ID: fmt.Sprintf("ITEM%d", i)})
		}
		links := map[string]string{}
		if end < total {
			links["next"] = fmt.Sprintf("http://%s%s?cursor=%d&limit=2", r.Host, r.URL.Path, end)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": data, "links": links})
	})

	items, err := listAll[testItem](t.Context(), client, "/v1/items", map[string][]string{"limit": {"2"}})
	if err != nil {
		t.Fatalf("listAll failed: %v", err)
	}
	if len(items) != total || items[total-1].ID != "ITEM4" {
		t.Fatalf("expected %d items across pages, got %+v", total, items)
	}
}

func TestClient_RefusesNextLinkToAnotherHost(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"data": []testItem{}, "links": map[string]string{"next": "https://other.example.com/v1/items"}})
	})

	_, err := listAll[testItem](t.Context(), client, "/v1/items", nil)
	if err == nil || !strings.Contains(err.Error(), "another host") {
		t.Fatalf("expected a refusal to follow a foreign next link, got %v", err)
	}
}

func TestClient_DecodesErrors(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]any{"errors": []apiErrorDetail{{
			Status: "404",
			Code:   "NOT_FOUND",
			Title:  "The specified resource does not exist",
			Detail: "There is no resource of type 'items' with id 'MISSING'",
		}}})
	})

	err := client.get(t.Context(), "/v1/items/MISSING", nil, nil)
	if !isNotFound(err) {
		t.Fatalf("expected a not-found error, got %v", err)
	}
	var apiErr *apiError
	if !errors.As(err, &apiErr) || len(apiErr.Errors) != 1 || apiErr.Errors[0].Code != "NOT_FOUND" {
		t.Fatalf("expected a decoded JSON:API error, got %#v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 404") || !strings.Contains(err.Error(), "MISSING") {
		t.Fatalf("expected the message to carry status and detail, got %q", err.Error())
	}
	if isNotFound(fmt.Errorf("wrapped: %w", errors.New("plain"))) {
		t.Fatal("expected a non-API error not to count as not found")
	}
	if !isNotFound(fmt.Errorf("wrapped: %w", err)) {
		t.Fatal("expected a wrapped API error to still count as not found")
	}
}

func TestClient_RejectsForeignSignature(t *testing.T) {
	_, credentials := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	other, _ := newTestCredentials(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if verifyTestToken(token, &credentials.privateKey.PublicKey, credentials.keyID) != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newAPIClient(server.URL, server.Client(), newTokenSource(other))
	if err := client.get(t.Context(), "/v1/items", nil, nil); !hasStatus(err, http.StatusUnauthorized) {
		t.Fatalf("expected a token signed by another key to be rejected, got %v", err)
	}
}
