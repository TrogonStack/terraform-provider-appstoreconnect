package provider

import (
	"fmt"
	"net/http"
	"testing"
)

func TestEntityErrors(t *testing.T) {
	conflict := &apiError{StatusCode: http.StatusConflict, Errors: []apiErrorDetail{
		{Code: "ENTITY_ERROR.ATTRIBUTE.INVALID", Detail: "invalid", Source: &apiErrorSource{Pointer: "/data/attributes/name"}},
		{Code: "ENTITY_ERRORS", Detail: "not an entity error"},
		{Code: "ENTITY_ERROR", Detail: "generic"},
	}}

	got := entityErrors(fmt.Errorf("wrapped: %w", conflict))
	if len(got) != 2 || got[0].Detail != "invalid" || got[1].Detail != "generic" {
		t.Fatalf("expected the two ENTITY_ERROR details, got %+v", got)
	}

	unprocessable := &apiError{StatusCode: http.StatusUnprocessableEntity, Errors: conflict.Errors}
	if got := entityErrors(unprocessable); got != nil {
		t.Fatalf("expected no entity errors outside HTTP 409, got %+v", got)
	}
}
