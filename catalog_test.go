package catalog

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

func NewTestCatalog(fetch bool) (*Catalog, ClientCatalog) {
	c := New()
	c.FQDN = []string{"example.com."}
	c.ProxyTag = "traefik.enable=true"
	c.ProxyService = "traefik"
	c.Networks = map[string]*net.IPNet{}
	_, private, _ := net.ParseCIDR("192.168.100.0/24")
	c.Networks["private"] = private
	_, guest, _ := net.ParseCIDR("192.168.1.0/24")
	c.Networks["guest"] = guest
	_, public, _ := net.ParseCIDR("0.0.0.0/0")
	c.Networks["public"] = public
	client := NewTestCatalogClient()
	c.SetClient(client)

	if fetch {
		c.FetchServices()
	}
	return c, client
}

func TestServeDNS(t *testing.T) {
	allHosts := map[string][]string{
		"nomad.service.consul.":   {"192.168.100.1"},
		"traefik.service.consul.": {"192.168.100.2"},
		"git.service.consul.":     {"192.168.100.3", "192.168.100.4"},
	}

	c, _ := NewTestCatalog(true)

	for !c.Ready() {
		time.Sleep(10)
	}

	defaultLookup = func(ctx context.Context, req request.Request, target string) (*dns.Msg, error) {
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

			if code != int(tc.expectedCode) {
				it.Fatalf("Test %d: Expected status code %d, but got %d", i, tc.expectedCode, code)
			}

			if len(tc.expectedReply) != 0 {
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
