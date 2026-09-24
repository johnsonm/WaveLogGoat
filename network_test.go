package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultProviders(t *testing.T) {
	// Structural check: Ensure defaultConnectionProvider exists and
	// has a Dial method, and implements ConnectionProvider.
	connProvider := &defaultConnectionProvider{}
	assert.Implements(t, (*ConnectionProvider)(nil), connProvider)

	// Structural check: Ensure defaultTransportProvider exists and
	// has a GetTransport method, and implements TransportProvider.
	transportProvider := &defaultTransportProvider{}
	assert.Implements(t, (*TransportProvider)(nil), transportProvider)
}
