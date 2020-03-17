package catalog

import (
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
)

var watchTimeout = time.Duration(10 * time.Minute)

var fetchFromConsul func(lastIndex uint64) (map[string][]string, uint64, error)

// CreateClient initializes the consul catalog client
func CreateClient(endpoint string, token string) (err error) {
	cfg := api.DefaultConfig()
	cfg.Address = endpoint
	if token != "" {
		cfg.Token = token
	}
	client, err := api.NewClient(cfg)

	if err != nil {
		return err
	}

	fetchFromConsul = func(lastIndex uint64) (svcs map[string][]string, nextIndex uint64, err error) {
		var meta *api.QueryMeta
		svcs, meta, err = client.Catalog().Services(&api.QueryOptions{
			WaitTime:  watchTimeout,
			WaitIndex: lastIndex,
		})

		if err != nil {
			return svcs, lastIndex, err
		}

		nextIndex = meta.LastIndex
		// reset the index if it goes backwards
		// https://www.consul.io/api/features/blocking.html#implementation-details
		if nextIndex < lastIndex {
			log.Debugf("Resetting consul catalog watch index")
			nextIndex = 0
		}

		return svcs, nextIndex, err
	}
	return nil
}

func validService(svc string, tags []string, filters []string) (string, bool) {
	target := svc
	valid := false
	for _, tag := range tags {
		if tag == defaultTraefikTag {
			target = "traefik"
			continue
		}

		if !valid {
			for _, filter := range filters {
				if tag == filter {
					valid = true
					continue
				}
			}
		}
	}

	if !valid {
		target = ""
	}

	return target, valid
}

// FetchServices populates zones
func (c *Catalog) FetchServices() error {
	c.RLock()
	lastIndex := c.lastIndex
	c.RUnlock()

	svcs, nextIndex, err := fetchFromConsul(lastIndex)
	if err != nil {
		return err
	}

	if nextIndex == lastIndex {
		// watch timed out, safe to retry
		log.Debugf("No changes found, %d", nextIndex)
		return nil
	}

	log.Debugf("Found %d catalog services", len(svcs))

	found := []string{}
	currentServices := map[string]string{}
	for svc, tags := range svcs {
		if dst, valid := validService(svc, tags, c.Tags); valid {
			currentServices[svc] = fmt.Sprintf("%s.service.consul.", dst)
			found = append(found, svc)
		}
	}

	c.Lock()
	c.ready = true
	c.services = currentServices
	c.lastIndex = nextIndex
	c.lastUpdate = time.Now()
	c.Unlock()

	log.Debugf("Serving records for %d catalog services: %s", len(found), strings.Join(found, ","))
	return nil
}
