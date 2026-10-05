package provider

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type apiErrorSource struct {
	Pointer   string `json:"pointer,omitempty"`
	Parameter string `json:"parameter,omitempty"`
}

type apiErrorDetail struct {
	Status string          `json:"status"`
	Code   string          `json:"code"`
	Title  string          `json:"title"`
	Detail string          `json:"detail"`
	Source *apiErrorSource `json:"source,omitempty"`
}

func (d apiErrorDetail) hasCode(prefix string) bool {
	return d.Code == prefix || strings.HasPrefix(d.Code, prefix+".")
}

func (d apiErrorDetail) message() string {
	if d.Detail != "" {
		return d.Detail
	}
	return d.Title
}

const maxErrorBodyRunes = 512

type errorBody string

func newErrorBody(raw []byte) errorBody {
	runes := []rune(strings.TrimSpace(string(raw)))
	if len(runes) > maxErrorBodyRunes {
		return errorBody(string(runes[:maxErrorBodyRunes]) + "...")
	}
	return errorBody(runes)
}

type apiError struct {
	StatusCode int
	Errors     []apiErrorDetail
	Body       errorBody
}

func (e *apiError) Error() string {
	if len(e.Errors) == 0 {
		if e.Body != "" {
			return fmt.Sprintf("App Store Connect API returned HTTP %d: %s", e.StatusCode, e.Body)
		}
		return fmt.Sprintf("App Store Connect API returned HTTP %d", e.StatusCode)
	}
	messages := make([]string, 0, len(e.Errors))
	for _, d := range e.Errors {
		messages = append(messages, fmt.Sprintf("%s: %s", d.Code, d.message()))
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

func entityErrors(err error) []apiErrorDetail {
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		return nil
	}
	var details []apiErrorDetail
	for _, d := range apiErr.Errors {
		if d.hasCode("ENTITY_ERROR") {
			details = append(details, d)
		}
	}
	return details
}
