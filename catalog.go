package catalog

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

var defaultTags = []string{"coredns.enabled"}
var defaultEndpoint = "consul.service.consul:8500"
var defaultTraefikTag = "traefik.enable=true"
var defaultTTL = uint32(time.Duration(5 * time.Minute).Seconds())
var defaultLookup = func(ctx context.Context, state request.Request, target string) (*dns.Msg, error) {
	recursor := upstream.New()
	return recursor.Lookup(ctx, state, target, dns.TypeA)
}

// Catalog holds published Consul Catalog services
type Catalog struct {
	sync.RWMutex
	Endpoint   string
	Tags       []string
	services   map[string]string
	FQDN       []string
	TTL        uint32
	Token      string
	Next       plugin.Handler
	lastIndex  uint64
	lastUpdate time.Time
	ready      bool
}

// New returns a Catalog plugin
func New() *Catalog {
	return &Catalog{
		Endpoint: defaultEndpoint,
		TTL:      defaultTTL,
		Tags:     defaultTags,
		services: map[string]string{},
	}
}

// Ready implements ready.Readiness
func (c *Catalog) Ready() bool {
	return c.ready
}

// LastUpdated returns the last time services changed
func (c *Catalog) LastUpdated() time.Time {
	return c.lastUpdate
}

// Services returns a map of services to their target
func (c *Catalog) Services() map[string]string {
	return c.services
}

// Name implements plugin.Handler
func (c *Catalog) Name() string { return "consul_catalog" }

// ServeDNS implements plugin.Handler
func (c *Catalog) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	if state.QType() != dns.TypeA {
		return plugin.NextOrFailure("consul_catalog", c.Next, ctx, w, r)
	}

	name := state.QName()
	for _, fqdn := range c.FQDN {
		name = strings.Replace(name, "."+fqdn, "", 1)
	}

	c.RLock()
	target, exists := c.services[name]
	c.RUnlock()

	if !exists {
		log.Debugf("Zone not found: %s", name)
		return plugin.NextOrFailure("consul_catalog", c.Next, ctx, w, r)
	}

	reply, err := defaultLookup(ctx, state, target)

	if err != nil {
		return 0, plugin.Error("Failed to lookup target", err)
	}

	log.Debugf("Found record for %s", name)
	m := new(dns.Msg)
	m.SetReply(r)
	m.Rcode = dns.RcodeSuccess
	m.Compress = true

	m.Answer = []dns.RR{}
	header := dns.RR_Header{
		Name:   state.QName(),
		Rrtype: dns.TypeA,
		Class:  dns.ClassINET,
		Ttl:    c.TTL,
	}

	for _, a := range reply.Answer {
		switch record := a.(type) {
		case *dns.A:
			m.Answer = append(m.Answer, &dns.A{
				Hdr: header,
				A:   record.A,
			})
		}
	}

	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}
