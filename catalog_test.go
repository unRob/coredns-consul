package catalog

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

func TestServeDNS(t *testing.T) {
	nextIndex := uint64(42)
	allServices := map[string][]string{
		"nomad": []string{
			"coredns.enabled",
			"traefik.enable=true",
		},
		"nomad-client": []string{},
		"traefik": []string{
			"coredns.enabled",
			"traefik.enable=true",
		},
		"git": []string{
			"coredns.enabled",
		},
	}
	allHosts := map[string][]string{
		"nomad.service.consul.":   []string{"10.0.0.2"},
		"traefik.service.consul.": []string{"10.0.0.2"},
		"git.service.consul.":     []string{"10.0.0.3", "10.0.0.4"},
	}
	fetchFromConsul = func(lastIndex uint64) (map[string][]string, uint64, error) {
		return allServices, nextIndex, nil
	}

	c := New()
	c.FQDN = []string{"example.com."}
	err := c.FetchServices()
	if err != nil {
		t.Fatalf("Fetch services: %v", err)
	}

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
		return res, err
	}

	tests := []struct {
		qname         string
		qtype         uint16
		expectedCode  int
		expectedReply []string
		expectedErr   error
	}{
		{
			qname:         "nomad.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"10.0.0.2"},
			expectedErr:   nil,
		},
		{
			qname:         "traefik.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"10.0.0.2"},
			expectedErr:   nil,
		},
		{
			qname:         "git.example.com",
			qtype:         dns.TypeA,
			expectedCode:  dns.RcodeSuccess,
			expectedReply: []string{"10.0.0.3", "10.0.0.4"},
			expectedErr:   nil,
		},
	}

	ctx := context.TODO()

	for i, tc := range tests {
		req := new(dns.Msg)
		req.SetQuestion(dns.Fqdn(tc.qname), tc.qtype)

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		code, err := c.ServeDNS(ctx, rec, req)

		if err != tc.expectedErr {
			t.Errorf("Test %d: Expected error %v, but got %v", i, tc.expectedErr, err)
			continue
		}

		if code != int(tc.expectedCode) {
			t.Errorf("Test %d: Expected status code %d, but got %d", i, tc.expectedCode, code)
			continue
		}

		if len(tc.expectedReply) != 0 {
			for i, expected := range tc.expectedReply {
				log.Debug(rec.Msg.Answer)
				actual := rec.Msg.Answer[i].(*dns.A).A
				if !actual.Equal(net.ParseIP(expected)) {
					t.Errorf("Test %d: Expected answer %s, but got %s", i, expected, actual)
				}
			}
		}
	}
}
