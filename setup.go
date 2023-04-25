// Copyright © 2022 Roberto Hidalgo <coredns-consul@un.rob.mx>
// SPDX-License-Identifier: Apache-2.0
package catalog

import (
	"net"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var pluginName = "consul_catalog"

var Log = clog.NewWithPlugin(pluginName)

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	catalog, err := parse(c)
	if err != nil {
		return plugin.Error("Failed to parse", err)
	}

	config := dnsserver.GetConfig(c)
	config.AddPlugin(func(next plugin.Handler) plugin.Handler {
		catalog.Next = next
		catalog.Zone = config.Zone
		return catalog
	})

	c.OnStartup(func() error {
		Log.Infof("Starting %d consul catalog watches for %s", len(catalog.Sources), catalog.Endpoint)

		m := dnsserver.GetConfig(c).Handler("prometheus")
		if m != nil {
			catalog.metrics = m.(*metrics.Metrics)
		}

		for _, watch := range catalog.Sources {
			go func(w *Watch) {
				Log.Infof("Starting lookup for %s", w.Name())
				for {
					Log.Debugf("Looking up %s", w.Name())

					onUpdateError := func(err error, cooldown time.Duration) {
						Log.Errorf("Could not lookup %s, retrying in %vs: %v", w.Name(), cooldown.Truncate(time.Second), err)
					}
					err := backoff.RetryNotify(func() error {
						_, err := w.Resolve(catalog)
						return err
					}, backoff.NewExponentialBackOff(), onUpdateError)

					if err != nil {
						continue
					}

					catalog.Lock()
					catalog.lastUpdate = time.Now()
					catalog.Unlock()
				}
			}(watch)
		}
		return nil
	})

	return nil
}

func parse(c *caddy.Controller) (cc *Catalog, err error) { // nolint: gocyclo
	cc = New()

	token := ""
	networks := map[string]*net.IPNet{}
	tag := defaultTag
	for c.Next() {
		tags := c.RemainingArgs()
		if len(tags) > 0 {
			tag = tags[0]
		}

		for c.NextBlock() {
			switch c.Val() {
			case "endpoint":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				cc.Endpoint = c.Val()
			case "scheme":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				cc.Scheme = c.Val()
			case "token":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				token = c.Val()
			case "ttl":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				ttl, err := time.ParseDuration(c.Val())
				if err != nil {
					return nil, c.Errf("Could not parse ttl as golang duration: %v", err)
				}

				cc.TTL = uint32(ttl.Seconds())
			case "acl_metadata_tag":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				cc.ACLTag = c.Val()
			case "acl_zone":
				remaining := c.RemainingArgs()
				if len(remaining) < 1 {
					return nil, c.Errf("must supply a name and cidr range for acl_zone")
				}
				name := remaining[0]
				_, network, err := net.ParseCIDR(remaining[1])
				if err != nil {
					return nil, c.Errf("unable to parse network range <%s>", remaining[1])
				}

				networks[name] = network
			case "service_proxy":
				remaining := c.RemainingArgs()
				if len(remaining) < 1 {
					return nil, c.Errf("service_proxy needs a tag and service")
				}

				cc.ProxyTag = remaining[0]
				cc.ProxyService = remaining[1]
				Log.Debugf("Found proxy config for tag %s and service %s", cc.ProxyTag, cc.ProxyService)
			case "alias_metadata_tag":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				cc.AliasTag = c.Val()
			case "static_entries_path":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}

				kvPath := c.Val()
				watcher := NewWatch(&WatchKVPath{Key: kvPath})
				cc.Sources = append(cc.Sources, watcher)
			case "static_entries_prefix":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}

				prefix := c.Val()
				watcher := NewWatch(&WatcKVPrefix{Prefix: prefix})
				cc.Sources = append(cc.Sources, watcher)
			default:
				return nil, c.Errf("unknown property %q", c.Val())
			}
		}
	}

	// Add catalog services watcher last
	cc.Sources = append(cc.Sources, NewWatch(&WatchConsulCatalog{Tag: tag}))

	cc.Networks = networks

	catalogClient, kvClient, err := CreateClient(cc.Scheme, cc.Endpoint, token)
	if err != nil {
		return nil, c.Errf("Could not create consul client: %v", err)
	}
	cc.SetClients(catalogClient, kvClient)

	for _, server := range c.ServerBlockKeys {
		cc.FQDN = append(cc.FQDN, plugin.Host(server).Normalize())
	}

	return cc, nil
}
