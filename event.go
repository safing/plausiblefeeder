package plausiblefeeder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

// PlausibleEvent represents an event for plausible and holds all data necessary to report it.
type PlausibleEvent struct {
	// Header Data.
	userAgent string `json:"-"` // Will marshal as "XuserAgent" if not explicitly ignored. WHY?
	remoteIP  net.IP `json:"-"` // Will marshal as "XremoteIP" if not explicitly ignored. WHY?

	// Payload Data.
	Domain     string `json:"domain"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	StatusCode string `json:"plausible-event-statuscode"`
}

func (pef *PlausibleEventFeeder) submitToFeed(r *http.Request, statusCode int) {
	// Get and check host.
	plausibleDomain := r.Host
	if !sliceContainsString(pef.config.Domains, plausibleDomain) {
		if pef.config.ReportAnyHost {
			plausibleDomain = pef.config.Domains[0]
		} else {
			// Ignore request to unlisted domain.
			if pef.config.DebugLogging {
				fmt.Fprintf(os.Stdout, "debug: ignoring request to unconfigured domain %q\n", plausibleDomain)
			}
			return
		}
	}

	// Get remote IP.
	var remoteIP net.IP
	if pef.config.RemoteIPFromHeader != "" {
		headerVal := r.Header.Get(pef.config.RemoteIPFromHeader)
		if headerVal == "" {
			// Ignore request if remote IP header is missing.
			if pef.config.DebugLogging {
				fmt.Fprintf(os.Stdout, "debug: required remote IP header field %q is missing\n", pef.config.RemoteIPFromHeader)
			}
			return
		}
		remoteIP = net.ParseIP(headerVal)
		if remoteIP == nil {
			// Ignore request if remote IP is invalid.
			if pef.config.DebugLogging {
				fmt.Fprintf(os.Stdout, "debug: remote IP from header field is invalid: %q\n", headerVal)
			}
			return
		}
	} else {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			// Ignore request if remote address is invalid.
			if pef.config.DebugLogging {
				fmt.Fprintf(os.Stdout, "debug: failed to parse remote address %q: %s\n", r.RemoteAddr, err)
			}
			return
		}
		remoteIP = net.ParseIP(host)
		if remoteIP == nil {
			// Ignore request if remote IP is invalid.
			if pef.config.DebugLogging {
				fmt.Fprintf(os.Stdout, "debug: remote IP from remote address is invalid: %q\n", host)
			}
			return
		}
	}

	// Create event and submit to queue.
	event := &PlausibleEvent{
		// Header Data.
		userAgent: r.UserAgent(),
		remoteIP:  remoteIP,
		// Payload Data.
		Domain:     plausibleDomain,
		Name:       "pageview",
		URL:        r.RequestURI,
		StatusCode: strconv.Itoa(statusCode),
	}
	select {
	case pef.queue <- event:
	default:
		fmt.Fprintf(os.Stderr, "plausiblefeeder plugin %q failed to submit event: queue full\n", pef.name)
	}
}

func (pef *PlausibleEventFeeder) startWorker(ctx context.Context) {
	for {
		err := pef.plausibleEventFeeder(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "plausiblefeeder plugin %q feed worker failed: %s\n", pef.name, err)
		} else {
			return
		}
	}
}

func (pef *PlausibleEventFeeder) plausibleEventFeeder(ctx context.Context) (err error) {
	defer func() {
		// Eecover from panic.
		panicVal := recover()
		if panicVal != nil {
			err = fmt.Errorf("panic: %v", panicVal)
		}
	}()

	client := &http.Client{
		Timeout: 1 * time.Second,
	}

	for {
		// Wait for event.
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "plausiblefeeder plugin %q feed worker shutting down (canceled)\n", pef.name)
			return nil

		case event := <-pef.queue:
			// Report event to plausible.
			pef.reportEventToPlausible(ctx, client, event)
		}
	}
}

func (pef *PlausibleEventFeeder) reportEventToPlausible(ctx context.Context, client *http.Client, event *PlausibleEvent) {
	// Create body.
	payload, err := json.Marshal(event)
	if err != nil {
		fmt.Fprintf(os.Stderr, "plausiblefeeder plugin %q failed to marshal event: %s\n", pef.name, err)
		return
	}

	// Create request.
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		pef.config.EventEndpoint,
		bytes.NewReader(payload),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "plausiblefeeder plugin %q failed to create http request: %s\n", pef.name, err)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", event.userAgent)
	request.Header.Set("X-Forwarded-For", event.remoteIP.String())

	// Send to plausible.
	resp, err := client.Do(request)
	if err != nil {
		fmt.Fprintf(os.Stderr, "plausiblefeeder plugin %q failed to send http request to plausible endpoint: %s\n", pef.name, err)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusAccepted {
		fmt.Fprintf(os.Stderr, "plausiblefeeder plugin %q got unexpected status code from plausible endpoint: %s\n", pef.name, resp.Status)
		return
	}

	if pef.config.DebugLogging {
		fmt.Fprintf(os.Stderr, "debug: sent event to %s:\n", pef.config.EventEndpoint)
		fmt.Fprintf(os.Stderr, "debug: sent headers: %+v\n", request.Header)
		fmt.Fprintf(os.Stderr, "debug: sent payload: %s\n", string(payload))
	}
}
