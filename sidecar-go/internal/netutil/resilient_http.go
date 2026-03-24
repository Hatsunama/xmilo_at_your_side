package netutil

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// NewResilientHTTPClient returns an HTTP client that can survive Android/Termux
// environments where the Go DNS resolver is pointed at a dead local stub
// (commonly ::1:53), causing all external HTTPS calls to fail.
//
// Design:
//   - Try the default resolver first (best-case: respects system DNS).
//   - If lookup fails in a way that looks like a local stub resolver failure,
//     fall back to a DNS resolver that queries well-known public DNS servers.
//   - We still pass the original host:port to the Transport so TLS SNI uses the
//     hostname, even if we dial a resolved IP underneath.
func NewResilientHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: resilientDialContext(
				net.DefaultResolver,
				newPublicDNSResolver(),
				&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second},
			),
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          20,
		},
	}
}

func resilientDialContext(defaultResolver *net.Resolver, fallbackResolver *net.Resolver, d *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			// If somehow not host:port, let the dialer handle it.
			return d.DialContext(ctx, network, addr)
		}

		// Fast path for IP literals.
		if net.ParseIP(host) != nil {
			return d.DialContext(ctx, network, addr)
		}

		ips, err := lookupIPAddrs(ctx, defaultResolver, host)
		if err != nil && isLocalStubResolverFailure(err) {
			ips, err = lookupIPAddrs(ctx, fallbackResolver, host)
		}
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("dns: no A/AAAA records for %q", host)
		}

		// Try v4 first (mobile networks vary; v4 is the lowest friction).
		var v4, v6 []net.IPAddr
		for _, ip := range ips {
			if ip.IP.To4() != nil {
				v4 = append(v4, ip)
			} else {
				v6 = append(v6, ip)
			}
		}
		candidates := append(v4, v6...)

		var lastErr error
		for _, ip := range candidates {
			conn, dialErr := d.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = errors.New("dial failed")
		}
		return nil, lastErr
	}
}

func lookupIPAddrs(ctx context.Context, r *net.Resolver, host string) ([]net.IPAddr, error) {
	// Use LookupIPAddr so we keep IPv4/IPv6, and because it doesn't require a network hint.
	addrs, err := r.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]net.IPAddr, 0, len(addrs))
	for _, a := range addrs {
		if a.IP == nil {
			continue
		}
		out = append(out, net.IPAddr{IP: a.IP})
	}
	return out, nil
}

func isLocalStubResolverFailure(err error) bool {
	// net.DNSError.Error() often embeds the resolver address:
	// "lookup relay.xmiloatyourside.com on [::1]:53: read udp ... connection refused"
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		s := dnsErr.Error()
		return looksLikeLoopbackDNS(s)
	}
	// Be defensive for wrapped / platform-specific errors.
	return looksLikeLoopbackDNS(err.Error())
}

func looksLikeLoopbackDNS(s string) bool {
	s = strings.ToLower(s)
	if !strings.Contains(s, "lookup ") {
		return false
	}
	// Common loopback stub resolvers that show up in Go error strings.
	if strings.Contains(s, " on [::1]:53") || strings.Contains(s, " on 127.0.0.1:53") {
		// Most actionable local-stub failures are connection refused / unreachable.
		if strings.Contains(s, "connection refused") || strings.Contains(s, "i/o timeout") || strings.Contains(s, "no such host") {
			return true
		}
		return true
	}
	return false
}

func newPublicDNSResolver() *net.Resolver {
	servers := []string{
		"1.1.1.1:53",                // Cloudflare v4
		"8.8.8.8:53",                // Google v4
		"[2606:4700:4700::1111]:53", // Cloudflare v6
		"[2001:4860:4860::8888]:53", // Google v6
	}

	d := &net.Dialer{Timeout: 4 * time.Second}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			// Prefer UDP; fall back to TCP if UDP is blocked.
			var lastErr error
			for _, server := range servers {
				conn, err := d.DialContext(ctx, "udp", server)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			for _, server := range servers {
				conn, err := d.DialContext(ctx, "tcp", server)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr == nil {
				lastErr = errors.New("dns: no public resolvers configured")
			}
			return nil, lastErr
		},
	}
}
