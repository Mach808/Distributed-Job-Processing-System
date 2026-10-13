package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardServesEmbeddedPageAndAssets(t *testing.T) {
	handler, err := newDashboardHandler()
	if err != nil {
		t.Fatalf("create dashboard handler: %v", err)
	}

	for _, test := range []struct {
		path        string
		content     string
		contentType string
	}{
		{path: "/", content: "Job queue", contentType: "text/html"},
		{path: "/app.js", content: "refreshEveryMs", contentType: "text/javascript"},
		{path: "/styles.css", content: "--green", contentType: "text/css"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", test.path, nil))

			if response.Code != 200 {
				t.Fatalf("GET %s status = %d, want 200", test.path, response.Code)
			}
			if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, test.contentType) {
				t.Fatalf("GET %s Content-Type = %q, want substring %q", test.path, contentType, test.contentType)
			}
			if !strings.Contains(response.Body.String(), test.content) {
				t.Fatalf("GET %s body does not contain %q", test.path, test.content)
			}
		})
	}
}
