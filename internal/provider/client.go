package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const defaultBaseURL = "https://api.appstoreconnect.apple.com"

type bearerTokenSource interface {
	Token() (string, error)
}

type apiClient struct {
	baseURL    string
	httpClient *http.Client
	tokens     bearerTokenSource
}

func newAPIClient(baseURL string, httpClient *http.Client, tokens bearerTokenSource) *apiClient {
	return &apiClient{baseURL: strings.TrimSuffix(baseURL, "/"), httpClient: httpClient, tokens: tokens}
}

func (c *apiClient) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.send(ctx, http.MethodGet, c.endpoint(path, query), nil, out)
}

func (c *apiClient) post(ctx context.Context, path string, body, out any) error {
	return c.send(ctx, http.MethodPost, c.endpoint(path, nil), body, out)
}

func (c *apiClient) patch(ctx context.Context, path string, body, out any) error {
	return c.send(ctx, http.MethodPatch, c.endpoint(path, nil), body, out)
}

func (c *apiClient) delete(ctx context.Context, path string, body any) error {
	return c.send(ctx, http.MethodDelete, c.endpoint(path, nil), body, nil)
}

func (c *apiClient) endpoint(path string, query url.Values) string {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	return endpoint
}

func (c *apiClient) send(ctx context.Context, method, endpoint string, body, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding %s %s request: %w", method, endpoint, err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	token, err := c.tokens.Token()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &apiError{StatusCode: resp.StatusCode}
		var envelope struct {
			Errors []apiErrorDetail `json:"errors"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err == nil {
			apiErr.Errors = envelope.Errors
		}
		return apiErr
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding %s %s response: %w", method, endpoint, err)
	}
	return nil
}

type pagedLinks struct {
	Next string `json:"next"`
}

type page[T any] struct {
	Data  []T        `json:"data"`
	Links pagedLinks `json:"links"`
}

func listAll[T any](ctx context.Context, c *apiClient, path string, query url.Values) ([]T, error) {
	var all []T
	endpoint := c.endpoint(path, query)
	for endpoint != "" {
		var current page[T]
		if err := c.send(ctx, http.MethodGet, endpoint, nil, &current); err != nil {
			return nil, err
		}
		all = append(all, current.Data...)
		next, err := c.nextPage(current.Links.Next)
		if err != nil {
			return nil, err
		}
		endpoint = next
	}
	return all, nil
}

func (c *apiClient) nextPage(next string) (string, error) {
	if next == "" {
		return "", nil
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	target, err := base.Parse(next)
	if err != nil {
		return "", fmt.Errorf("parsing next page link %q: %w", next, err)
	}
	if target.Scheme != base.Scheme || target.Host != base.Host {
		return "", fmt.Errorf("refusing to follow next page link to another host: %s", target.Host)
	}
	return target.String(), nil
}
