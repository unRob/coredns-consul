// Copyright © 2022 Roberto Hidalgo <coredns-consul@un.rob.mx>
// SPDX-License-Identifier: Apache-2.0
package catalog

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

var defaultTag = "coredns.enabled"
var defaultEndpoint = "consul.service.consul:8500"
var defaultTTL = uint32((5 * time.Minute).Seconds())
var defaultMeta = "coredns-acl"
var defaultLookup = func(ctx context.Context, state request.Request, target string) (*dns.Msg, error) {
	recursor := upstream.New()
	req := state.NewWithQuestion(target, dns.TypeA)
	return recursor.Lookup(ctx, req, target, dns.TypeA)
}

// Catalog holds published Consul Catalog services.
type Catalog struct {
	sync.RWMutex
	Endpoint         string
	Tag              string
	services         map[string]*Service
	staticEntries    map[string]*Service
	FQDN             []string
	TTL              uint32
	Token            string
	ProxyService     string
	ProxyTag         string
	Networks         map[string]*net.IPNet
	MetadataTag      string
	ConfigKey        string
	Next             plugin.Handler
	Zone             string
	lastCatalogIndex uint64
	lastConfigIndex  uint64
	lastUpdate       time.Time
	ready            bool
	client           ClientCatalog
	kv               KVClient
}

// New returns a Catalog plugin.
func New() *Catalog {
	return &Catalog{
		Endpoint:    defaultEndpoint,
		TTL:         defaultTTL,
		Tag:         defaultTag,
		MetadataTag: defaultMeta,
		services:    map[string]*Service{},
	}
}

// SetClient sets a consul client for a catalog.
func (c *Catalog) SetClients(client ClientCatalog, kv KVClient) {
	c.client = client
	c.kv = kv
}

// Ready implements ready.Readiness.
func (c *Catalog) Ready() bool {
	return c.ready
}

// LastUpdated returns the last time services changed.
func (c *Catalog) LastUpdated() time.Time {
	return c.lastUpdate
}

// Services returns a map of services to their target.
func (c *Catalog) Services() map[string]*Service {
	return c.services
}

// Name implements plugin.Handler.
func (c *Catalog) Name() string { return "consul_catalog" }

func (c *Catalog) ServiceFor(name string) (svc *Service) {
	var exists bool
	c.RLock()
	if svc, exists = c.staticEntries[name]; !exists {
		Log.Debugf("Zone missing from static entries %s", name)
		svc = c.services[name]
	}
	c.RUnlock()

	return
}

// ServeDNS implements plugin.Handler.
func (c *Catalog) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r, Zone: c.Zone}

	name := state.QName()
	for _, fqdn := range c.FQDN {
		name = strings.Replace(name, "."+fqdn, "", 1)
	}

	svc := c.ServiceFor(name)

	if svc == nil {
		Log.Debugf("Zone not found: %s", name)
		return plugin.NextOrFailure("consul_catalog", c.Next, ctx, w, r)
	}

	if len(c.Networks) > 0 {
		ip := net.ParseIP(state.IP())
		if !svc.RespondsTo(ip) {
			Log.Warningf("Blocked resolution for service %s from ip %s", name, ip)
			return plugin.NextOrFailure("consul_catalog", c.Next, ctx, w, r)
		}
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Rcode = dns.RcodeSuccess
	m.Compress = true

	m.Answer = []dns.RR{}
	header := dns.RR_Header{
		Name:   state.QName(),
		Rrtype: state.QType(),
		Class:  dns.ClassINET,
		Ttl:    c.TTL,
	}

	if fp, ok := c.Next.(file.File); ok && len(fp.Zones.Z) > 0 {
		// grab the SOA from the file plugin, if available and next in the chain
		if zone, ok := fp.Zones.Z[c.FQDN[0]]; ok {
			Log.Debugf("Adding SOA %s", zone.SOA.String())
			m.Ns = []dns.RR{zone.SOA}
		}
	}

	if state.QType() != dns.TypeA {
		// return NODATA
		Log.Debugf("Record for %s does not contain answers for type %s", name, state.Type())
		err := w.WriteMsg(m)
		return dns.RcodeSuccess, err
	}

	lookupName := svc.Target
	if svc.Target == "@service_proxy" {
		lookupName = c.ProxyService
	}

	Log.Debugf("looking up target: %s", lookupName)

	if target := c.ServiceFor(lookupName); target != nil && len(target.Addresses) > 0 {
		Log.Debugf("Found addresses in catalog for %s: %v", lookupName, target.Addresses)
		for _, addr := range target.Addresses {
			m.Answer = append(m.Answer, &dns.A{
				Hdr: header,
				A:   addr,
			})
		}
	} else {
		Log.Debugf("Looking up address for %s upstream", lookupName)
		reply, err := defaultLookup(ctx, state, fmt.Sprintf("%s.service.consul", lookupName))

		if err != nil {
			return 0, plugin.Error("Failed to lookup target upstream", err)
		}
		Log.Debugf("Found record for %s upstream", name)

		for _, a := range reply.Answer {
			record, ok := a.(*dns.A)
			if !ok {
				Log.Warningf("Found non-A record upstream: %s", a.String())
				continue
			}

			m.Answer = append(m.Answer, &dns.A{
				Hdr: header,
				A:   record.A,
			})
		}
	}

	err := w.WriteMsg(m)
	return dns.RcodeSuccess, err
}
