package plausiblefeeder

import "net/http"

// ResponseWriter is used to wrap given response writers.
type ResponseWriter struct {
	http.ResponseWriter

	request *http.Request
	pef     *PlausibleEventFeeder
}

// WriteHeader adds custom handling to the wrapped WriterHeader method.
func (rw *ResponseWriter) WriteHeader(code int) {
	if rw.pef.statusIsReportable(code) {
		rw.pef.submitToFeed(rw.request, code)
	}

	// Continue with the original method.
	rw.ResponseWriter.WriteHeader(code)
}
