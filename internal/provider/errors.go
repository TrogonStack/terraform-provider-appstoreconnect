package provider

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type apiErrorDetail struct {
	Status string `json:"status"`
	Code   string `json:"code"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

type apiError struct {
	StatusCode int
	Errors     []apiErrorDetail
}

func (e *apiError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("App Store Connect API returned HTTP %d", e.StatusCode)
	}
	messages := make([]string, 0, len(e.Errors))
	for _, d := range e.Errors {
		messages = append(messages, fmt.Sprintf("%s: %s", d.Code, d.Detail))
	}
	return fmt.Sprintf("App Store Connect API returned HTTP %d: %s", e.StatusCode, strings.Join(messages, "; "))
}

func hasStatus(err error, status int) bool {
	var apiErr *apiError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

func isNotFound(err error) bool {
	return hasStatus(err, http.StatusNotFound)
}
