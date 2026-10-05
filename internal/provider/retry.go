package provider

import (
	"context"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

const requestAttemptTimeout = 90 * time.Second

type requestMethodKey struct{}

type methodTaggingTransport struct {
	next http.RoundTripper
}

func (t methodTaggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.next.RoundTrip(req.WithContext(context.WithValue(req.Context(), requestMethodKey{}, req.Method)))
}

func newRetryableClient(tokens bearerTokenSource) *http.Client {
	client := retryablehttp.NewClient()
	client.HTTPClient.Timeout = requestAttemptTimeout
	client.RetryMax = 5
	client.CheckRetry = retryPolicy
	client.ErrorHandler = retryablehttp.PassthroughErrorHandler
	client.PrepareRetry = func(req *http.Request) error {
		token, err := tokens.Token()
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
	client.Logger = nil
	return &http.Client{Transport: methodTaggingTransport{next: &retryablehttp.RoundTripper{Client: client}}}
}

func retryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if method, _ := ctx.Value(requestMethodKey{}).(string); method == http.MethodPost {
		return false, nil
	}
	if err != nil {
		return true, nil
	}
	if resp.StatusCode == 429 {
		return true, nil
	}
	if resp.StatusCode >= 500 && resp.StatusCode != 501 {
		return true, nil
	}
	return false, nil
}
