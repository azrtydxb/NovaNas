package metrics

import (
	"net/http"
	"net/http/httptest"
)

func scrapeRequest() *http.Request {
	return httptest.NewRequest("GET", "/metrics", nil)
}

func newScrapeRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}
