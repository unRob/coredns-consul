package catalog

import (
	"time"

	"github.com/caddyserver/caddy"
	"github.com/cenkalti/backoff/v4"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var log = clog.NewWithPlugin("consul_catalog")

func init() { plugin.Register("consul_catalog", setup) }

func setup(c *caddy.Controller) error {
	catalog, err := parse(c)
	if err != nil {
		return plugin.Error("Failed to parse", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		catalog.Next = next
		return catalog
	})

	onUpdateError := func(err error, cooldown time.Duration) {
		log.Errorf("Could not obtain services from catalog, retrying in %vs: %v", cooldown.Truncate(time.Second), err)
	}

	c.OnStartup(func() error {
		log.Infof("Starting consul catalog watch at %s", catalog.Endpoint)
		go func() {
			// poll for changes, backing off in case of failures
			for {
				log.Debug("Waiting for consul catalog services...")

				err := backoff.RetryNotify(catalog.FetchServices, backoff.NewExponentialBackOff(), onUpdateError)

				if err != nil {
					plugin.Error("Failed not obtain services from catalog", err)
					continue
				}
			}
		}()
		return nil
	})

	return nil
}

func parse(c *caddy.Controller) (cc *Catalog, err error) {
	cc = New()

	token := ""
	for c.Next() {
		tags := c.RemainingArgs()
		if len(tags) > 0 {
			cc.Tags = tags
		}

		for c.NextBlock() {
			switch c.Val() {
			case "endpoint":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				cc.Endpoint = c.Val()
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
			default:
				return nil, c.Errf("unknown property %q", c.Val())
			}
		}
	}

	if err = CreateClient(cc.Endpoint, token); err != nil {
		return nil, c.Errf("Could not create consul client: %v", err)
	}

	for _, server := range c.ServerBlockKeys {
		cc.FQDN = append(cc.FQDN, plugin.Host(server).Normalize())
	}

	return cc, nil
}
