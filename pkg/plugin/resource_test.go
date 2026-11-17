package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

func TestNormalizeResourcePath(t *testing.T) {
	cases := map[string]string{
		"projects":   "/projects",
		"/projects":  "/projects",
		"projects/":  "/projects",
		"//projects": "/projects",
		"regions":    "/regions",
	}
	for in, want := range cases {
		if got := normalizeResourcePath(in); got != want {
			t.Errorf("normalizeResourcePath(%q) = %q, want %q", in, got, want)
		}
	}
}

type captureSender struct{ resp *backend.CallResourceResponse }

func (c *captureSender) Send(r *backend.CallResourceResponse) error { c.resp = r; return nil }

// Grafana passes req.Path without a leading slash; it must not hit the "unknown path" branch.
func TestCallResourceRoutesPathWithoutLeadingSlash(t *testing.T) {
	// Fake UCloud OpenAPI endpoint for GetMetricDataAggregationMethod.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"RetCode":0,"Action":"GetMetricDataAggregationMethodResponse","List":[{"Value":"avg","Label":"平均值","IsDefault":true}]}`))
	}))
	defer srv.Close()
	d := &Datasource{uclient: NewUCloudClient("pub", "priv", srv.URL, "")}
	for _, p := range []string{"aggregations", "/aggregations"} {
		s := &captureSender{}
		req := &backend.CallResourceRequest{Path: p, URL: p, Method: http.MethodGet}
		if err := d.CallResource(context.Background(), req, s); err != nil {
			t.Fatalf("path %q: %v", p, err)
		}
		if s.resp.Status != http.StatusOK {
			t.Fatalf("path %q: status %d body %s", p, s.resp.Status, s.resp.Body)
		}
		var opts []Option
		if err := json.Unmarshal(s.resp.Body, &opts); err != nil || len(opts) == 0 {
			t.Fatalf("path %q: bad body %s", p, s.resp.Body)
		}
	}
	s := &captureSender{}
	_ = d.CallResource(context.Background(), &backend.CallResourceRequest{Path: "nope", URL: "nope"}, s)
	if s.resp.Status != http.StatusNotFound {
		t.Fatalf("unknown path should 404, got %d", s.resp.Status)
	}
}
