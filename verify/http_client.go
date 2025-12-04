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
			configureProxy(transport, options)
			client.Transport = transport
		} else {
			// If transport exists, try to configure proxy if it's a *http.Transport
			if transport, ok := client.Transport.(*http.Transport); ok {
				// Clone to avoid modifying the original
				clonedTransport := transport.Clone()
				configureProxy(clonedTransport, options)
				client.Transport = clonedTransport
			}
		}
		return &client
	}

	// Create a new client with timeout and proxy support
	transport := http.DefaultTransport.(*http.Transport).Clone()
	configureProxy(transport, options)
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

// configureProxy configures the proxy settings for the transport
// It prioritizes ProxyURL from options, then falls back to environment variables
func configureProxy(transport *http.Transport, options *VerifyOptions) {
	if options.ProxyURL != nil {
		// Use explicit proxy URL if provided
		transport.Proxy = http.ProxyURL(options.ProxyURL)
	} else {
		// Otherwise, use proxy from environment variables (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)
		transport.Proxy = http.ProxyFromEnvironment
	}
}
