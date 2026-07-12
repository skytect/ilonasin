package provider

import (
	"net/http"
	"time"
)

const (
	outboundMaxIdleConns        = 64
	outboundMaxIdleConnsPerHost = 16
	outboundMaxConnsPerHost     = 32
	outboundIdleConnTimeout     = 90 * time.Second
	outboundTLSHandshakeTimeout = 10 * time.Second
	outboundExpectContinueTime  = time.Second
	outboundResponseHeaderTime  = 30 * time.Second
)

var sharedOutboundTransport http.RoundTripper = newSharedOutboundTransport()

func newSharedOutboundTransport() http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	transport := base.Clone()
	transport.MaxIdleConns = outboundMaxIdleConns
	transport.MaxIdleConnsPerHost = outboundMaxIdleConnsPerHost
	transport.MaxConnsPerHost = outboundMaxConnsPerHost
	transport.IdleConnTimeout = outboundIdleConnTimeout
	transport.TLSHandshakeTimeout = outboundTLSHandshakeTimeout
	transport.ExpectContinueTimeout = outboundExpectContinueTime
	transport.ResponseHeaderTimeout = outboundResponseHeaderTime
	return transport
}

func NewOutboundHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: sharedOutboundTransport,
	}
}

func outboundHTTPClientWithDefaults(client *http.Client, timeout time.Duration) *http.Client {
	if client == nil {
		return NewOutboundHTTPClient(timeout)
	}
	clone := *client
	if clone.Timeout == 0 && timeout > 0 {
		clone.Timeout = timeout
	}
	if clone.Transport == nil {
		clone.Transport = sharedOutboundTransport
	}
	return &clone
}
