package main

import (
	"context"
	"net"
	"net/http"
)

// TransportProvider defines the interface for creating http.RoundTripper, enabling mocking in tests.
type TransportProvider interface {
	GetTransport() http.RoundTripper
}

// defaultTransportProvider uses the CustomDialer to create http.RoundTripper.
type defaultTransportProvider struct{}

func (d *defaultTransportProvider) GetTransport() http.RoundTripper {
	return &http.Transport{
		DialContext: CustomDialer,
	}
}

// ConnectionProvider defines the interface for creating network connections, enabling mocking in tests.
type ConnectionProvider interface {
	Dial(ctx context.Context, network, address string) (net.Conn, error)
}

// defaultConnectionProvider uses the CustomDialer to create network connections.
type defaultConnectionProvider struct{}

func (d *defaultConnectionProvider) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	return CustomDialer(ctx, network, address)
}
