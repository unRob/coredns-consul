// Copyright © 2022 Roberto Hidalgo <coredns-consul@un.rob.mx>
// SPDX-License-Identifier: Apache-2.0
package catalog_test

import (
	"net"
	"testing"

	"github.com/miekg/dns"
	. "github.com/unRob/coredns-consul"
)

func TestProxiedAddressesByProximity(t *testing.T) {
	proxy := &Service{
		Name: "proxy",
		Addresses: []net.IP{
			net.ParseIP("192.168.1.6"),
			net.ParseIP("192.168.1.7"),
			net.ParseIP("192.168.1.8"),
			net.ParseIP("192.168.1.9"),
			net.ParseIP("192.168.1.10"),
		},
	}

	tests := []struct {
		Name     string
		Source   net.IP
		Target   *Service
		Proxy    *Service
		Expected []net.IP
	}{
		{
			Name:   "prioritizes incoming address above else",
			Source: net.ParseIP("192.168.1.10"),
			Proxy:  proxy,
			Target: &Service{
				Name:   "test",
				Target: "proxy",
				Addresses: []net.IP{
					net.ParseIP("192.168.1.6"),
					net.ParseIP("192.168.1.10"),
				},
			},
			Expected: []net.IP{
				net.ParseIP("192.168.1.10"),
				net.ParseIP("192.168.1.6"),
				net.ParseIP("192.168.1.7"),
				net.ParseIP("192.168.1.8"),
				net.ParseIP("192.168.1.9"),
			},
		},
		{
			Name:   "ignores target addresses not in proxy",
			Source: net.ParseIP("127.0.0.1"),
			Proxy:  proxy,
			Target: &Service{
				Name:   "test",
				Target: "proxy",
				Addresses: []net.IP{
					net.ParseIP("192.168.1.7"),
					net.ParseIP("127.0.0.1"),
				},
			},
			Expected: []net.IP{
				net.ParseIP("192.168.1.7"),
				net.ParseIP("192.168.1.6"),
				net.ParseIP("192.168.1.8"),
				net.ParseIP("192.168.1.9"),
				net.ParseIP("192.168.1.10"),
			},
		},
		{
			Name:   "prioritizes target addresses",
			Source: net.ParseIP("127.0.0.1"),
			Proxy:  proxy,
			Target: &Service{
				Name:   "test",
				Target: "proxy",
				Addresses: []net.IP{
					net.ParseIP("192.168.1.6"),
					net.ParseIP("192.168.1.10"),
				},
			},
			Expected: []net.IP{
				net.ParseIP("192.168.1.6"),
				net.ParseIP("192.168.1.10"),
				net.ParseIP("192.168.1.7"),
				net.ParseIP("192.168.1.8"),
				net.ParseIP("192.168.1.9"),
			},
		},
		{
			Name:   "prioritizes target addresses over proxy address",
			Source: net.ParseIP("192.168.1.9"),
			Proxy:  proxy,
			Target: &Service{
				Name:   "test",
				Target: "proxy",
				Addresses: []net.IP{
					net.ParseIP("192.168.1.6"),
					net.ParseIP("192.168.1.10"),
				},
			},
			Expected: []net.IP{
				net.ParseIP("192.168.1.6"),
				net.ParseIP("192.168.1.10"),
				net.ParseIP("192.168.1.7"),
				net.ParseIP("192.168.1.8"),
				net.ParseIP("192.168.1.9"),
			},
		},
	}

	for _, tst := range tests {
		t.Run(tst.Name, func(t *testing.T) {
			header := dns.RR_Header{}
			res := ProxiedAddressesByProximity(tst.Source, tst.Target, tst.Proxy, header)

			for idx, record := range res {
				got := record.(*dns.A).A
				wanted := tst.Expected[idx]
				if !got.Equal(wanted) {
					t.Fatalf("Expected %d: %s, got %s", idx, wanted, got)
				}
			}
		})
	}
}
