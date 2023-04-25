package catalog

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

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
			RequestACLDeniedCount.WithLabelValues(metrics.WithServer(ctx), metrics.WithView(ctx)).Inc()
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
		RequestDropCount.WithLabelValues(metrics.WithServer(ctx), metrics.WithView(ctx)).Inc()
		err := w.WriteMsg(m)
		return dns.RcodeSuccess, err
	}

	lookupName := svc.Target
	if svc.Target == "@service_proxy" {
		lookupName = c.ProxyService
	}

	Log.Debugf("looking up target: %s", lookupName)

	source := ""
	if target := c.ServiceFor(lookupName); target != nil && len(target.Addresses) > 0 {
		Log.Debugf("Found addresses in catalog for %s: %v", lookupName, target.Addresses)
		source = "api"
		for _, addr := range target.Addresses {
			m.Answer = append(m.Answer, &dns.A{
				Hdr: header,
				A:   addr,
			})
		}
	} else {
		Log.Debugf("Looking up address for %s upstream", lookupName)
		reply, err := DefaultLookup(ctx, state, fmt.Sprintf("%s.service.consul", lookupName))

		if err != nil {
			return 0, plugin.Error("Failed to lookup target upstream", err)
		}
		Log.Debugf("Found record for %s upstream", name)

		for _, a := range reply.Answer {
			source = "dns"
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

	RequestServedCount.WithLabelValues(metrics.WithServer(ctx), metrics.WithView(ctx), source).Inc()
	err := w.WriteMsg(m)
	return dns.RcodeSuccess, err
}
