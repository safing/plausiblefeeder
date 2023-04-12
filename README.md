# Plausible Feeder Traefik Plugin

Traefik plugin that feeds HTTP requests to plausible as pageview events.

### Config

```
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
```
