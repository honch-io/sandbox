package health

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPStatusReportsUpForSuccessfulHealthEndpoint(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	})}

	status := httpStatus(context.Background(), client, "http://capture.test/health", time.Second)
	if status != "up" {
		t.Fatalf("status = %q, want up", status)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHTTPStatusReportsDownWithReasonForRefusedEndpoint(t *testing.T) {
	status := HTTPStatus(context.Background(), "http://127.0.0.1:1", 50*time.Millisecond)
	if !strings.HasPrefix(status, "down") {
		t.Fatalf("status = %q, want down reason", status)
	}
}
