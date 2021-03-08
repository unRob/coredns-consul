package catalog

import (
	"net"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var Log = clog.NewWithPlugin("consul_catalog")

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
		Log.Errorf("Could not obtain services from catalog, retrying in %vs: %v", cooldown.Truncate(time.Second), err)
	}

	c.OnStartup(func() error {
		Log.Infof("Starting consul catalog watch at %s", catalog.Endpoint)

		go func() {
			// poll for changes, backing off in case of failures
			for {
				Log.Debug("Waiting for consul catalog services...")

				err := backoff.RetryNotify(catalog.FetchServices, backoff.NewExponentialBackOff(), onUpdateError)

				if err != nil {
					plugin.Error("Failed to obtain services from catalog", err)
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
	networks := map[string]*net.IPNet{}
	for c.Next() {
		tags := c.RemainingArgs()
		if len(tags) > 0 {
			cc.Tag = tags[0]
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
			case "acl_metadata_tag":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				cc.MetadataTag = c.Val()
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
					return nil, c.Errf("ttl needs a time in second")
				}

				cc.ProxyTag = remaining[0]
				cc.ProxyService = remaining[1]
			default:
				return nil, c.Errf("unknown property %q", c.Val())
			}
		}
	}

	cc.Networks = networks

	client, err := CreateClient(cc.Endpoint, token)
	if err != nil {
		return nil, c.Errf("Could not create consul client: %v", err)
	}
	cc.SetClient(client)

	for _, server := range c.ServerBlockKeys {
		cc.FQDN = append(cc.FQDN, plugin.Host(server).Normalize())
	}

	return cc, nil
}
