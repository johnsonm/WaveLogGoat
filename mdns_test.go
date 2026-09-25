package main

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func init() {
	log = logrus.New()
	log.SetLevel(logrus.PanicLevel)
}

// MockMDNSQuerier is a mock implementation of the MDNSQuerier interface.
type MockMDNSQuerier struct {
	mock.Mock
	IPv4Success bool
	IPv6Success bool
}

func (m *MockMDNSQuerier) Query(params *mdns.QueryParam) error {
	// Simulate finding a service by sending an entry to the channel if IPv4Success or IPv6Success is true
	if params.Entries != nil && (m.IPv4Success || m.IPv6Success) {
		if m.IPv4Success {
			params.Entries <- &mdns.ServiceEntry{
				AddrV4: net.ParseIP("192.168.1.1"),
			}
		} else if m.IPv6Success {
			params.Entries <- &mdns.ServiceEntry{
				AddrV6: net.ParseIP("::1"),
			}
		}
	} else if !m.IPv4Success && !m.IPv6Success {
		// Sleep to allow timeout to happen in the caller
		time.Sleep(500 * time.Millisecond)
	}
	return m.Called(params).Error(0)
}

func TestLookupLocalHost(t *testing.T) {
	ctx := context.Background()

	t.Run("successful IPv4 resolution", func(t *testing.T) {
		mockQuerier := new(MockMDNSQuerier)
		mockQuerier.IPv4Success = true
		mockQuerier.On("Query", mock.Anything).Return(nil)

		ip, err := LookupLocalHost(ctx, "test.local", mockQuerier)

		assert.NoError(t, err)
		assert.Equal(t, "192.168.1.1", ip)
		mockQuerier.AssertExpectations(t)
	})

	t.Run("successful IPv6 resolution", func(t *testing.T) {
		mockQuerier := new(MockMDNSQuerier)
		mockQuerier.IPv6Success = true
		mockQuerier.On("Query", mock.Anything).Return(nil)

		ip, err := LookupLocalHost(ctx, "test.local", mockQuerier)

		assert.NoError(t, err)
		assert.Equal(t, "::1", ip)
		mockQuerier.AssertExpectations(t)
	})

	t.Run("resolution timeout", func(t *testing.T) {
		// Create a mock that never sends an entry
		slowQuerier := new(MockMDNSQuerier)
		slowQuerier.IPv4Success = false
		slowQuerier.IPv6Success = false
		slowQuerier.On("Query", mock.Anything).Return(nil)

		// Set a short context timeout
		shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		_, err := LookupLocalHost(shortCtx, "slow.local", slowQuerier)

		assert.Error(t, err)
		if err != nil {
			assert.Contains(t, err.Error(), "mDNS resolution timed out")
		}
	})
}
