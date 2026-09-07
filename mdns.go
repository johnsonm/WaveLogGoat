package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
)

var mDNSTTL time.Duration = 5 * time.Minute

// LookupLocalHost resolves a .local hostname to an IP address using pure Go mDNS.
func LookupLocalHost(ctx context.Context, hostname string) (string, error) {
	if !strings.HasSuffix(hostname, ".") {
		hostname = hostname + "."
	}

	entriesCh := make(chan *mdns.ServiceEntry, 4)

	// Create a localized context timeout for the mDNS multicast query
	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	go func() {
		defer close(entriesCh)
		params := mdns.DefaultParams(hostname)
		params.DisableIPv6 = false
		if err := mdns.Query(params); err != nil {
			log.Printf("mDNS query failed: %v", err)
		}
	}()

	select {
	case entry := <-entriesCh:
		if entry != nil {
			if entry.AddrV4 != nil {
				return entry.AddrV4.String(), nil
			}
			if entry.AddrV6 != nil {
				return entry.AddrV6.String(), nil
			}
		}
		return "", fmt.Errorf("host %s found but had no valid IP addresses", hostname)
	case <-queryCtx.Done():
		return "", fmt.Errorf("mDNS resolution timed out for %s: %w", hostname, queryCtx.Err())
	}
}

// CustomDialer intercepts network connection requests. If the address contains a
// ".local" hostname, it resolves it using pure Go mDNS before establishing a raw TCP connection.
func CustomDialer(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	if strings.Contains(host, ".local") {
		resolvedIP, err := LookupLocalHost(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve .local host via mDNS: %w", err)
		}
		addr = net.JoinHostPort(resolvedIP, port)
	}

	dialer := net.Dialer{Timeout: mDNSTTL}
	return dialer.DialContext(ctx, network, addr)
}
