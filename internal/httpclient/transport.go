package httpclient

import "net/http"

// BearerTransport injects an Authorization header when Token is set.
type BearerTransport struct {
	Token string
	Base  http.RoundTripper
}

func (t *BearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.Token != "" {
		// Clone with the request's own context (set by NewRequestWithContext upstream).
		// RoundTripper contract forbids mutating the original request; Clone preserves
		// context cancellation without introducing a new context.
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+t.Token)
	}
	return t.Base.RoundTrip(r)
}
