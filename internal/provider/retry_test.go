package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
)

func TestRetryPolicy_429_Retries(t *testing.T) {
	resp := &http.Response{StatusCode: 429}
	retry, err := retryPolicy(context.Background(), resp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retry {
		t.Error("expected retry on 429")
	}
}

func TestRetryPolicy_500_Retries(t *testing.T) {
	resp := &http.Response{StatusCode: 500}
	retry, err := retryPolicy(context.Background(), resp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retry {
		t.Error("expected retry on 500")
	}
}

func TestRetryPolicy_502_Retries(t *testing.T) {
	resp := &http.Response{StatusCode: 502}
	retry, err := retryPolicy(context.Background(), resp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retry {
		t.Error("expected retry on 502")
	}
}

func TestRetryPolicy_501_DoesNotRetry(t *testing.T) {
	resp := &http.Response{StatusCode: 501}
	retry, err := retryPolicy(context.Background(), resp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retry {
		t.Error("expected no retry on 501")
	}
}

func TestRetryPolicy_200_DoesNotRetry(t *testing.T) {
	resp := &http.Response{StatusCode: 200}
	retry, err := retryPolicy(context.Background(), resp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retry {
		t.Error("expected no retry on 200")
	}
}

func TestRetryPolicy_404_DoesNotRetry(t *testing.T) {
	resp := &http.Response{StatusCode: 404}
	retry, err := retryPolicy(context.Background(), resp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retry {
		t.Error("expected no retry on 404")
	}
}

func TestRetryPolicy_ConnectionError_Retries(t *testing.T) {
	retry, err := retryPolicy(context.Background(), nil, io.ErrUnexpectedEOF)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retry {
		t.Error("expected retry on connection error")
	}
}

func TestRetryPolicy_CancelledContext_DoesNotRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	retry, err := retryPolicy(ctx, nil, nil)
	if err == nil {
		t.Fatal("expected context error")
	}
	if retry {
		t.Error("expected no retry on cancelled context")
	}
}

func TestRetryPolicy_PostNeverRetries(t *testing.T) {
	ctx := context.WithValue(context.Background(), requestMethodKey{}, http.MethodPost)
	for name, tc := range map[string]struct {
		resp *http.Response
		err  error
	}{
		"server error":    {resp: &http.Response{StatusCode: 500}},
		"rate limited":    {resp: &http.Response{StatusCode: 429}},
		"transport error": {err: io.ErrUnexpectedEOF},
	} {
		t.Run(name, func(t *testing.T) {
			retry, err := retryPolicy(ctx, tc.resp, tc.err)
			if err != nil || retry {
				t.Fatalf("expected POST to never retry, got retry=%v err=%v", retry, err)
			}
		})
	}
}

func TestRetryableClient_BoundsEachAttempt(t *testing.T) {
	tagging, ok := newRetryableClient(&countingTokens{}).Transport.(methodTaggingTransport)
	if !ok {
		t.Fatalf("expected the method tagging transport, got %T", newRetryableClient(&countingTokens{}).Transport)
	}
	transport, ok := tagging.next.(*retryablehttp.RoundTripper)
	if !ok {
		t.Fatalf("expected the retryable round tripper, got %T", tagging.next)
	}
	if got := transport.Client.HTTPClient.Timeout; got != requestAttemptTimeout {
		t.Fatalf("expected a %s per-attempt timeout, got %s", requestAttemptTimeout, got)
	}
}

type countingTokens struct {
	mu    sync.Mutex
	count int
}

func (c *countingTokens) Token() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
	return fmt.Sprintf("token-%d", c.count), nil
}

type recordedRequest struct {
	method        string
	authorization string
}

func newRetryingTestClient(t *testing.T, handler func(attempt int, w http.ResponseWriter)) (*apiClient, func() []recordedRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, recordedRequest{method: r.Method, authorization: r.Header.Get("Authorization")})
		attempt := len(seen)
		mu.Unlock()
		handler(attempt, w)
	}))
	t.Cleanup(server.Close)
	tokens := &countingTokens{}
	return newAPIClient(server.URL, newRetryableClient(tokens), tokens), func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedRequest(nil), seen...)
	}
}

func unavailable(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "0")
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{"errors": []apiErrorDetail{{
		Status: "503",
		Code:   "SERVICE_UNAVAILABLE",
		Title:  "The service is temporarily unavailable.",
	}}})
}

func TestRetryableClient_DoesNotRetryPost(t *testing.T) {
	client, requests := newRetryingTestClient(t, func(_ int, w http.ResponseWriter) { unavailable(w) })

	err := client.post(t.Context(), "/v1/items", map[string]string{"name": "Example"}, nil)
	if !hasStatus(err, http.StatusServiceUnavailable) {
		t.Fatalf("expected the 503 to surface as an apiError, got %v", err)
	}
	if got := len(requests()); got != 1 {
		t.Fatalf("expected exactly one POST attempt, got %d", got)
	}
}

func TestRetryableClient_RetriesIdempotentMethodsWithFreshTokens(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			client, requests := newRetryingTestClient(t, func(attempt int, w http.ResponseWriter) {
				if attempt == 1 {
					unavailable(w)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})

			if err := client.send(t.Context(), method, client.endpoint("/v1/items/ITEM", nil), nil, nil); err != nil {
				t.Fatalf("expected the retry to succeed, got %v", err)
			}
			seen := requests()
			if len(seen) != 2 {
				t.Fatalf("expected a retry, got %d attempts", len(seen))
			}
			if seen[0].authorization == seen[1].authorization {
				t.Fatalf("expected each attempt to ask the token source again, both used %q", seen[0].authorization)
			}
		})
	}
}

func TestRetryableClient_ExhaustedRetriesKeepAppleErrors(t *testing.T) {
	client, requests := newRetryingTestClient(t, func(_ int, w http.ResponseWriter) { unavailable(w) })

	err := client.get(t.Context(), "/v1/items", nil, nil)
	var apiErr *apiError
	if !errors.As(err, &apiErr) || len(apiErr.Errors) != 1 || apiErr.Errors[0].Code != "SERVICE_UNAVAILABLE" {
		t.Fatalf("expected the final response to decode into an apiError, got %v", err)
	}
	if got := len(requests()); got != 6 {
		t.Fatalf("expected the initial attempt plus 5 retries, got %d", got)
	}
}
