package httpclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBearerTransport_InjectsHeader(t *testing.T) {
	const token = "test-token"
	var gotHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	transport := &BearerTransport{Token: token, Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}
	_, _ = client.Get(srv.URL)

	want := "Bearer " + token
	if gotHeader != want {
		t.Errorf("got Authorization %q, want %q", gotHeader, want)
	}
}

func TestBearerTransport_SkipsHeaderWhenNoToken(t *testing.T) {
	var gotHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	transport := &BearerTransport{Token: "", Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}
	_, _ = client.Get(srv.URL)

	if gotHeader != "" {
		t.Errorf("expected no Authorization header, got %q", gotHeader)
	}
}

func TestBearerTransport_DoesNotMutateOriginalRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	original := req.Header.Clone()

	transport := &BearerTransport{Token: "secret", Base: http.DefaultTransport}
	_, _ = transport.RoundTrip(req)

	for k := range req.Header {
		if strings.EqualFold(k, "Authorization") {
			t.Errorf("original request header was mutated: Authorization header was added")
		}
	}
	_ = original
}
