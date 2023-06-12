// Copyright © 2022 Roberto Hidalgo <coredns-consul@un.rob.mx>
// Contributions by Charles Powell, 2023
// SPDX-License-Identifier: Apache-2.0
package catalog

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
)

const ServiceProxyTag = "@service_proxy"

type WatchType interface {
	Name() string
	Fetch(*Catalog, *api.QueryOptions) (uint64, error)
	Process(*Catalog) (ServiceMap, []string, error)
}

type Watch struct {
	sync.RWMutex
	LastIndex uint64

	services  ServiceMap
	refreshed time.Time
	watcher   WatchType
	ready     bool
}

func NewWatch(impl WatchType) *Watch {
	w := &Watch{
		watcher: impl,
	}

	return w
}

func (w *Watch) Resolve(catalog *Catalog) (bool, error) {
	w.RLock()
	lastIndex := w.LastIndex
	w.RUnlock()

	opts := &api.QueryOptions{
		WaitTime:  watchTimeout,
		WaitIndex: lastIndex,
	}

	nextIndex, err := w.watcher.Fetch(catalog, opts)

	if err != nil {
		return false, err
	}

	if nextIndex == opts.WaitIndex {
		// watch timed out, safe to retry
		Log.Debugf("No changes found, %d", nextIndex)
		w.Lock()
		w.refreshed = time.Now()
		w.Unlock()
		return false, nil
	}

	// reset the index if it goes backwards
	// https://www.consul.io/api/features/blocking.html#implementation-details
	if nextIndex < opts.WaitIndex {
		Log.Debugf("Resetting consul kv watch index")
		nextIndex = 0
	}

	services, found, err := w.watcher.Process(catalog)
	if err != nil {
		return false, err
	}

	w.Lock()
	w.ready = true
	w.services = services
	w.LastIndex = nextIndex
	w.refreshed = time.Now()
	w.Unlock()
	Log.Debugf("Serving %d records from %s: %s", len(found), w.watcher.Name(), strings.Join(found, ","))
	return true, nil
}

func (w *Watch) Name() string {
	return w.watcher.Name()
}

func (w *Watch) Get(name string) *Service {
	return w.services.Find(name)
}

func (w *Watch) Known() ServiceMap {
	return w.services
}

func (w *Watch) Ready() bool {
	return w.ready
}

func staticEntriesToServiceMap(c *Catalog, entries StaticEntries) (ServiceMap, []string) {
	services := ServiceMap{}

	found := []string{}
	for name, entry := range entries {
		target := entry.Target
		if target == ServiceProxyTag {
			if c.ProxyService == "" {
				Log.Warningf("Ignoring service %s. Requested service proxy but none is configured", name)
				continue
			}
		}

		service := NewService(name, target)

		if c.ACLTag != "" {
			err := c.parseACL(service, entry.ACL)
			if err != nil {
				Log.Warningf("Ignoring service %s. Could not parse ACL: %s", name, err)
				continue
			}
		}

		if c.AliasTag != "" && len(entry.Aliases) > 0 {
			for _, alias := range entry.Aliases {
				services[alias] = aliasForService(alias, service)
				found = append(found, alias)
			}
		}

		if previous, ok := services[service.Name]; ok {
			Log.Warningf("Replacing service %s. Duplicate entry configured. Had: %+v, now: %+v", name, previous, service)
		}

		services[name] = service
		found = append(found, name)
	}

	return services, found
}

type WatcKVPrefix struct {
	Prefix  string
	entries api.KVPairs
}

func (src *WatcKVPrefix) Name() string {
	return fmt.Sprintf("static services at prefix %s", src.Prefix)
}

func (src *WatcKVPrefix) Fetch(catalog *Catalog, qo *api.QueryOptions) (uint64, error) {
	entryPairs, meta, err := catalog.kv.List(src.Prefix, qo)
	if err != nil {
		return qo.WaitIndex, err
	}
	src.entries = entryPairs
	return meta.LastIndex, nil
}

func (src *WatcKVPrefix) Process(catalog *Catalog) (ServiceMap, []string, error) {
	entries := StaticEntries{}
	for _, entry := range src.entries {
		e := &StaticEntry{}
		err := json.Unmarshal(entry.Value, &e)
		if err != nil {
			return nil, nil, err
		}

		parts := strings.Split(entry.Key, "/")
		name := parts[len(parts)-1]
		entries[name] = e
	}
	services, found := staticEntriesToServiceMap(catalog, entries)
	return services, found, nil
}

type WatchKVPath struct {
	Key  string
	data *api.KVPair
}

func (src *WatchKVPath) Name() string {
	return fmt.Sprintf("static services from key %s", src.Key)
}

func (src *WatchKVPath) Fetch(catalog *Catalog, qo *api.QueryOptions) (uint64, error) {
	configPair, meta, err := catalog.kv.Get(src.Key, qo)
	if err != nil {
		return qo.WaitIndex, err
	}
	src.data = configPair
	return meta.LastIndex, nil
}

func (src *WatchKVPath) Process(catalog *Catalog) (ServiceMap, []string, error) {
	entries := StaticEntries{}
	err := json.Unmarshal(src.data.Value, &entries)
	if err != nil {
		return nil, nil, err
	}

	services, found := staticEntriesToServiceMap(catalog, entries)
	return services, found, nil
}

type WatchConsulCatalog struct {
	Tag  string
	data map[string][]string
}

func (src *WatchConsulCatalog) Name() string {
	return fmt.Sprintf("consul catalog services tagged %s", src.Tag)
}

func (src *WatchConsulCatalog) Fetch(catalog *Catalog, qo *api.QueryOptions) (uint64, error) {
	svcs, meta, err := catalog.client.Services(qo)
	if err != nil {
		return qo.WaitIndex, err
	}
	src.data = svcs
	return meta.LastIndex, nil
}

func (src *WatchConsulCatalog) Process(catalog *Catalog) (ServiceMap, []string, error) {
	services := ServiceMap{}
	found := []string{}

	for svc, serviceTags := range src.data {
		target := svc
		exposed := false

		for _, tag := range serviceTags {
			switch tag {
			case catalog.ProxyTag:
				if catalog.ProxyTag != "" {
					target = ServiceProxyTag
				}
			case src.Tag:
				exposed = true
			default:
				Log.Debugf("ignoring unknown tag %s for svc %s", tag, svc)
			}
		}

		// do not publish services without the tag
		if !exposed {
			continue
		}

		hydratedServices, _, err := catalog.client.Service(svc, "", nil)
		if err != nil {
			// couldn't find service, ignore
			Log.Debugf("Failed to fetch service info for %s: %e", svc, err)
			continue
		}

		service := NewService(svc, target)

		if len(hydratedServices) > 0 {
			for _, svc := range hydratedServices {
				service.Addresses = append(service.Addresses, net.ParseIP(svc.Address))
			}
			metadata := hydratedServices[0].ServiceMeta
			if catalog.ACLTag != "" {
				acl, exists := metadata[catalog.ACLTag]
				if !exists {
					Log.Warningf("No ACL found for %s", svc)
					continue
				}

				if err := catalog.parseACLString(service, acl); err != nil {
					Log.Warningf("Ignoring service %s: %s", service.Name, err)
				}
			}

			if catalog.AliasTag != "" {
				if aliases, exists := metadata[catalog.AliasTag]; exists {
					matches := multiValueMetadataSplitter.Split(aliases, -1)
					for _, match := range matches {
						services[match] = aliasForService(match, service)
					}
					found = append(found, matches...)
				}
			}
		} else {
			Log.Warningf("No services found for %s, check the permissions for your token", svc)
		}

		services[svc] = service
		Log.Debugf("serving: %+v", service)
		found = append(found, svc)
	}

	return services, found, nil
}

var multiValueMetadataSplitter = regexp.MustCompile(`;\s*`)

func aliasForService(name string, service *Service) *Service {
	alias := NewService(name, service.Target)
	alias.ACL = service.ACL
	alias.Addresses = service.Addresses
	return alias
}

var _ WatchType = &WatchConsulCatalog{}
var _ WatchType = &WatchKVPath{}
var _ WatchType = &WatcKVPrefix{}
