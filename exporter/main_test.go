package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCommandVerb(t *testing.T) {
	cases := map[string]string{
		"get pods -n prod":            "get",
		"-n prod delete deploy web":   "delete", // leading flags skipped
		"exec pod -- sh":              "exec",
		"":                            "unknown",
		"--all-namespaces get events": "get",
	}
	for in, want := range cases {
		if got := commandVerb(in); got != want {
			t.Errorf("commandVerb(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHandleAuditCountsByLabels(t *testing.T) {
	decisions.Reset()
	body := `{"time":"t","actor":"holmesgpt","context":"prod","command":"get secret db -o yaml","outcome":"blocked","reason":"protected-resource"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/audit", strings.NewReader(body))
	handleAudit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got := testutil.ToFloat64(decisions.WithLabelValues("blocked", "get", "holmesgpt", "prod", "protected-resource"))
	if got != 1 {
		t.Errorf("counter for the blocked secret read = %v, want 1", got)
	}
}

func TestHandleAuditMalformedIs200(t *testing.T) {
	// A malformed event must not error (the guard's webhook is best-effort and must
	// never retry or block the command it audits).
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/audit", bytes.NewReader([]byte("not json")))
	handleAudit(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("malformed event status = %d, want 200", rr.Code)
	}
}
