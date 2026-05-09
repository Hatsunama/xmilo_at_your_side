package netutil

import (
	"fmt"
	"net"
	"testing"
)

func TestLocalStubResolverFailureDetection(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "android_ipv6_loopback_stub",
			err:  &net.DNSError{Err: "read udp [::1]:53: connection refused", Name: "api.openai.com", Server: "[::1]:53"},
			want: true,
		},
		{
			name: "android_ipv4_loopback_stub",
			err:  fmt.Errorf("lookup api.openai.com on 127.0.0.1:53: read udp 127.0.0.1: connection refused"),
			want: true,
		},
		{
			name: "real_nxdomain",
			err:  &net.DNSError{Err: "no such host", Name: "not-a-real-provider.invalid", Server: "8.8.8.8:53"},
			want: false,
		},
		{
			name: "non_dns_network_error",
			err:  fmt.Errorf("dial tcp 203.0.113.10:443: i/o timeout"),
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isLocalStubResolverFailure(test.err); got != test.want {
				t.Fatalf("isLocalStubResolverFailure() = %v, want %v", got, test.want)
			}
		})
	}
}
