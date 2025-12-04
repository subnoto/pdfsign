package verify

import (
	"net/http"
	"time"
)

// getHTTPClient returns an HTTP client configured with the correct timeout and proxy settings
func getHTTPClient(options *VerifyOptions) *http.Client {
	timeout := options.HTTPTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	// If a custom HTTP client is provided, clone it with the correct timeout
	if options.HTTPClient != nil {
		client := *options.HTTPClient
		client.Timeout = timeout
		// If the custom client has no transport, ensure proxy support
		if client.Transport == nil {
			transport := http.DefaultTransport.(*http.Transport).Clone()
			client.Transport = transport
		}
		return &client
	}

	// Create a new client with timeout and proxy support
	transport := http.DefaultTransport.(*http.Transport).Clone()
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
