package plausiblefeeder

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
)

// Default config values.
const (
	DefaultQueueSize = 1000
	MinQueueSize     = 100
)

// Config is the plugin configuration.
type Config struct { //nolint:maligned
	// EventEndpoint defines the URL of the events API endpoint of plausible.
	EventEndpoint string

	// Domains defines the hosts that should be reported to plausible.
	// Requests to hosts not in the list will be ignored.
	Domains []string

	// ReportExtensions defines an alternative list of file extensions that should be reported. The default set is disabled.
	ReportExtensions []string

	// ReportAllResources defines whether all requests for any resource should be
	// reported to plausible.
	// By default, only requests that are believed to contain content are reported.
	// If enabled, all requests are reported to plausible.
	ReportAllResources bool

	// ReportAnyHost defines whether all hosts should be reported to plausible.
	// By default, only hosts listed in Domains are reported.
	// If enabled non-matching requests will be reported using the first domain from Domains.
	ReportAnyHost bool

	// ReportErrors defines whether error responses should be ignored and not sent to plausible.
	// By default only 2xx and 3xx status codes are reported.
	// If enabled, 4xx and 5xx status codes will be reported too.
	ReportErrors bool

	// RemoteIPFromHeader defines which header to get the remote IP from.
	// If not defined, the remote IP of the network connection is used.
	// If the defined header is missing, the request will be ignored and not reported.
	// If the value found in the header is not a valid IP address, the request will be ignored and not reported.
	RemoteIPFromHeader string

	// QueueSize defines the size of the queue size, ie. the amount events that
	// are waiting to be submitted to plausible.
	// Minimum is 100.
	QueueSize int

	// DebugLogging defines whether debug information should be logged.
	// Warning: It's a lot!
	DebugLogging bool
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{}
}

// PlausibleEventFeeder is a plugin that feeds requests to plausible as pageview events.
type PlausibleEventFeeder struct {
	next   http.Handler
	name   string
	config *Config
	queue  chan *PlausibleEvent
}

// New created a new plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// Check required values.
	if config.EventEndpoint == "" {
		return nil, errors.New("must configure event endpoint for plausiblefeeder")
	}
	if len(config.Domains) == 0 {
		return nil, errors.New("must configure at least one domain for plausiblefeeder")
	}

	// Apply default values.
	if config.QueueSize == 0 {
		if config.DebugLogging {
			fmt.Fprintf(os.Stdout, "debug: using default queue size of %d\n", DefaultQueueSize)
		}

		config.QueueSize = DefaultQueueSize
	}
	if config.QueueSize < MinQueueSize {
		if config.DebugLogging {
			fmt.Fprintf(os.Stdout, "debug: raising queue size to %d (was %d)\n", MinQueueSize, config.QueueSize)
		}

		config.QueueSize = MinQueueSize
	}

	// Make sure that all defined extensions have a leading dot.
	for k, v := range config.ReportExtensions {
		if !strings.HasPrefix(v, ".") && v != "" {
			config.ReportExtensions[k] = "." + v

			if config.DebugLogging {
				fmt.Fprintf(os.Stdout, "debug: converting report extension to %q\n", config.ReportExtensions[k])
			}
		}
	}

	// Log the instantiation of the plugin, including configuration.
	fmt.Fprintf(os.Stdout, "creating plausiblefeeder plugin %q with config: %+v\n", name, config)

	// Create instance and start worker.
	pef := &PlausibleEventFeeder{
		next:   next,
		name:   name,
		config: config,
		queue:  make(chan *PlausibleEvent, config.QueueSize),
	}
	go pef.startWorker(ctx)

	return pef, nil
}

// ServeHTTP handles a http request.
func (pef *PlausibleEventFeeder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if a request to this resource should be reported at all.
	if !pef.resourceIsReportable(r) {
		pef.next.ServeHTTP(w, r)
		return
	}

	// If the resource should be reported, we wrap the response writer and check the status code before reporting.
	wrappedResponseWriter := &ResponseWriter{
		ResponseWriter: w,
		request:        r,
		pef:            pef,
	}

	// Continue with next handler.
	pef.next.ServeHTTP(wrappedResponseWriter, r)
}

func (pef *PlausibleEventFeeder) resourceIsReportable(r *http.Request) (report bool) {
	// Check if reporting is enabled for the requested host.
	switch {
	case pef.config.ReportAnyHost:
		// Enabled for any host, continue.
	case sliceContainsString(pef.config.Domains, r.Host):
		// Hostname is enabled.
	default:
		// Requested host should not be reported.
		if pef.config.DebugLogging {
			fmt.Fprintf(os.Stdout, "debug: not reporting request to host %q\n", r.Host)
		}
		return false
	}

	// Check if all resources should be potentially reported.
	if pef.config.ReportAllResources {
		return true
	}

	// If a custom file extension list is defined, check if the resource matches it. If not, do not report.
	if len(pef.config.ReportExtensions) > 0 {
		pathExt := path.Ext(r.URL.Path)
		for _, suffix := range pef.config.ReportExtensions {
			if suffix == pathExt {
				return true
			}
		}
		if pef.config.DebugLogging {
			fmt.Fprintf(os.Stdout, "debug: not reporting request for path %q - does not end in configured extensions\n", r.URL.Path)
		}
		return false
	}

	// Check if the suffix is regarded to be "content".
	switch path.Ext(r.URL.Path) {
	case ".htm":
	case ".html":
	case ".php":
	case ".rss":
	case ".rtf":
	case ".xml":
	case "":
	default:
		if pef.config.DebugLogging {
			fmt.Fprintf(os.Stdout, "debug: not reporting request for path %q - does not end in default extensions\n", r.URL.Path)
		}
		return false
	}

	return true
}

func (pef *PlausibleEventFeeder) statusIsReportable(statusCode int) (report bool) {
	statusBase := statusCode - statusCode%100

	switch statusBase {
	case 200, 300:
		return true

	case 400, 500:
		// Check if errors should be reported.
		if pef.config.ReportErrors {
			return true
		}
		if pef.config.DebugLogging {
			fmt.Fprintf(os.Stdout, "debug: not reporting %d error\n", statusCode)
		}
		return false

	default:
		if pef.config.DebugLogging {
			fmt.Fprintf(os.Stdout, "debug: not reporting unexpected status code %d\n", statusCode)
		}
		return false
	}
}
