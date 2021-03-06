package catalog

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
)

var watchTimeout = time.Duration(10 * time.Minute)

// ClientCatalog is implemented by github.com/hashicorp/consul/api.Catalog
type ClientCatalog interface {
	Service(string, string, *api.QueryOptions) ([]*api.CatalogService, *api.QueryMeta, error)
	Services(*api.QueryOptions) (map[string][]string, *api.QueryMeta, error)
}

// CreateClient initializes the consul catalog client
func CreateClient(endpoint string, token string) (catalog ClientCatalog, err error) {
	cfg := api.DefaultConfig()
	cfg.Address = endpoint
	if token != "" {
		cfg.Token = token
	}

	client, err := api.NewClient(cfg)

	if err != nil {
		return
	}

	catalog = client.Catalog()

	return
}

// FetchServices populates zones
func (c *Catalog) FetchServices() error {
	c.RLock()
	lastIndex := c.lastIndex
	c.RUnlock()

	svcs, meta, err := c.client.Services(&api.QueryOptions{
		WaitTime:  watchTimeout,
		WaitIndex: lastIndex,
	})

	if err != nil {
		return err
	}

	nextIndex := meta.LastIndex
	// reset the index if it goes backwards
	// https://www.consul.io/api/features/blocking.html#implementation-details
	if nextIndex < lastIndex {
		Log.Debugf("Resetting consul catalog watch index")
		nextIndex = 0
	}

	if nextIndex == lastIndex {
		// watch timed out, safe to retry
		Log.Debugf("No changes found, %d", nextIndex)
		return nil
	}

	Log.Debugf("Found %d catalog services", len(svcs))

	found := []string{}
	currentServices := map[string]*Service{}

	for svc, serviceTags := range svcs {
		target := svc
		exposed := false

		for _, tag := range serviceTags {
			switch tag {
			case c.ProxyTag:
				if c.ProxyTag != "" {
					target = c.ProxyService
				}
			case c.Tag:
				exposed = true
			}
		}

		// do not publish services without the tag
		if !exposed {
			continue
		}

		hydratedServices, _, err := c.client.Service(svc, "", nil)
		if err != nil {
			// couldn't find service, ignore
			Log.Debugf("Failed to fetch service info for %s: %e", svc, err)
			continue
		}

		service := &Service{
			Target: fmt.Sprintf("%s.service.consul.", target),
			ACL:    []*ServiceACL{},
		}

		if len(hydratedServices) > 0 {
			metadata := hydratedServices[0].ServiceMeta
			acl, exists := metadata[c.MetadataTag]
			if !exists {
				continue
			}

			aclRules := regexp.MustCompile(`;\s*`).Split(acl, -1)
			for _, rule := range aclRules {
				ruleParts := strings.SplitN(rule, " ", 2)
				if len(ruleParts) != 2 {
					Log.Warningf("Ignoring service. Failed parsing acl rule <%s> for service %s", rule, svc)
					continue
				}
				action := ruleParts[0]
				for _, networkName := range regexp.MustCompile(`,\s*`).Split(ruleParts[1], -1) {
					if cidr, ok := c.Networks[networkName]; ok {
						service.ACL = append(service.ACL, &ServiceACL{
							Action:  action,
							Network: cidr,
						})
					} else {
						Log.Warningf("unknown network %s", networkName)
					}
				}
			}

		}

		currentServices[svc] = service
		found = append(found, svc)
	}

	c.Lock()
	c.ready = true
	c.services = currentServices
	c.lastIndex = nextIndex
	c.lastUpdate = time.Now()
	c.Unlock()

	Log.Debugf("Serving records for %d catalog services: %s", len(found), strings.Join(found, ","))
	return nil
}
