// Copyright © 2022 Roberto Hidalgo <coredns-consul@un.rob.mx>
// SPDX-License-Identifier: Apache-2.0
package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	. "github.com/unRob/coredns-consul"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/hashicorp/consul/api"
	"github.com/miekg/dns"
)

func staticServices() []byte {
	b, err := json.Marshal(map[string]StaticEntry{
		"domain": {
			Target: "traefik",
			ACL:    []string{"allow private"},
		},
		"sub.domain": {
			Target: "@service_proxy",
			ACL:    []string{"allow private"},
		},
		"*.star": {
			Target: "@service_proxy",
			ACL:    []string{"allow private"},
		},
		"alias": {
			Target:  "@service_proxy",
			ACL:     []string{"allow private"},
			Aliases: []string{"*.alias"},
		},
	})

	if err != nil {
		panic(err)
	}

	return b
}

func TestServeDNS(t *testing.T) {
	t.Parallel()
	allHosts := map[string][]string{
		"nomad.service.consul.":   {"192.168.100.1"},
		"traefik.service.consul.": {"192.168.100.2"},
		"git.service.consul.":     {"192.168.100.3", "192.168.100.4"},
	}

	src := NewWatch(&WatchKVPath{Key: "static/path"})

	c, _, kv := NewTestCatalog(true, src)
	tkv := kv.(*testKVClient)
	tkv.Keys = map[string]*api.KVPair{
		"static/path": {
			Key:   "static/path",
			Value: staticServices(),
		},
	}
	tkv.keysIndex++

	if err := c.ReloadAll(); err != nil {
		t.Fatal(err)
	}

	for !c.Ready() {
		time.Sleep(1 * time.Second)
	}

	DefaultLookup = func(ctx context.Context, req request.Request, target string) (*dns.Msg, error) {
		res := new(dns.Msg)
		res.Answer = []dns.RR{}
		ips, exists := allHosts[target]
		if !exists {
			res.SetRcode(req.Req, dns.RcodeNameError)
		} else {
			header := dns.RR_Header{Name: req.QName(), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}
			for _, ip := range ips {
				res.Answer = append(res.Answer, &dns.A{
					Hdr: header,
					A:   net.ParseIP(ip),
				})
			}
		}
		return res, nil
	}

	tests := []struct {
		qname         string
		qtype         uint16
		expectedCode  int
		expectedReply []string
		expectedErr   error
		from          string
	}{
		{
			qname:         "nomad.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"192.168.100.2"},
			expectedErr:   nil,
			from:          "192.168.100.42",
		},
		{
			qname:         "traefik.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"192.168.100.2"},
			expectedErr:   nil,
			from:          "192.168.100.42",
		},
		{
			qname:         "git.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"192.168.100.3", "192.168.100.4"},
			expectedErr:   nil,
			from:          "192.168.100.42",
		},
		{
			qname:         "nomad.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeServerFailure,
			expectedReply: []string{},
			expectedErr:   plugin.Error("consul_catalog", fmt.Errorf("no next plugin found")),
			from:          "192.168.1.1",
		},
		{
			qname:         "git.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeServerFailure,
			expectedReply: []string{},
			expectedErr:   plugin.Error("consul_catalog", fmt.Errorf("no next plugin found")),
			from:          "192.168.1.1",
		},
		{
			qname:         "git.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"192.168.100.3", "192.168.100.4"},
			expectedErr:   nil,
			from:          "10.42.0.1",
		},
		{
			qname:         "sub.domain.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"192.168.100.2"},
			expectedErr:   nil,
			from:          "192.168.100.42",
		},
		{
			qname:         "domain.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"192.168.100.2"},
			expectedErr:   nil,
			from:          "192.168.100.42",
		},
		{
			qname:         "does-not-exist.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeServerFailure,
			expectedReply: []string{},
			expectedErr:   plugin.Error("consul_catalog", fmt.Errorf("no next plugin found")),
			from:          "192.168.100.42",
		},
		{
			qname:         "whatever.star.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"192.168.100.2"},
			expectedErr:   nil,
			from:          "192.168.100.42",
		},
		{
			qname:         "alias.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"192.168.100.2"},
			expectedErr:   nil,
			from:          "192.168.100.42",
		},
		{
			qname:         "something.alias.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"192.168.100.2"},
			expectedErr:   nil,
			from:          "192.168.100.42",
		},
		{
			qname:         "recursive.something.alias.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeServerFailure,
			expectedReply: []string{},
			expectedErr:   plugin.Error("consul_catalog", fmt.Errorf("no next plugin found")),
			from:          "192.168.100.42",
		},
	}

	ctx := context.TODO()

	for i, tc := range tests {
		t.Run(fmt.Sprintf("%s-%s", tc.qname, tc.from), func(it *testing.T) {
			req := new(dns.Msg)
			req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)

			rec := dnstest.NewRecorder(&test.ResponseWriter{
				RemoteIP: tc.from,
			})
			code, err := c.ServeDNS(ctx, rec, req)

			if err == nil {
				if tc.expectedErr != nil {
					it.Fatalf("Expected error %v, got nil", tc.expectedErr)
				}
			} else if tc.expectedErr != nil && tc.expectedErr.Error() != err.Error() {
				it.Fatalf("Expected error %v, got %s", tc.expectedErr, err)
			}

			if code != tc.expectedCode {
				it.Fatalf("Test %d: Expected status code %d, but got %d", i, tc.expectedCode, code)
			}

			if len(tc.expectedReply) != 0 {
				if rec == nil || rec.Msg == nil {
					t.Fatal("Expected replies, got none")
				}

				if len(tc.expectedReply) != len(rec.Msg.Answer) {
					t.Fatalf("Expected %d replies, got %d", len(tc.expectedReply), len(rec.Msg.Answer))
				}

				for i, expected := range tc.expectedReply {
					actual, ok := rec.Msg.Answer[i].(*dns.A)

					if !ok {
						it.Fatalf("something crapped out: %v", rec.Msg.Answer)
						return
					}

					if !actual.A.Equal(net.ParseIP(expected)) {
						it.Errorf("Test %d: Expected answer %s, but got %s", i, expected, actual)
					}
				}
			}
		})
	}
}
