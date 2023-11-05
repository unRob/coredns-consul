// Copyright © 2022 Roberto Hidalgo <coredns-consul@un.rob.mx>
// Contributions by Charles Powell, 2023
// SPDX-License-Identifier: Apache-2.0
package catalog

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

var defaultTag = "coredns.enabled"
var defaultEndpoint = "consul.service.consul:8500"
var defaultTTL = uint32((5 * time.Minute).Seconds())
var defaultACLTag = "coredns-acl"
var defaultAliasTag = "coredns-alias"
var DefaultLookup = func(ctx context.Context, state request.Request, target string) (*dns.Msg, error) {
	recursor := upstream.New()
	req := state.NewWithQuestion(target, dns.TypeA)
	return recursor.Lookup(ctx, req, target, dns.TypeA)
}

// Catalog holds published Consul Catalog services.
type Catalog struct {
	sync.RWMutex
	Endpoint     string
	Scheme       string
	FQDN         []string
	TTL          uint32
	Token        string
	ProxyService string
	ProxyTag     string
	Networks     map[string][]*net.IPNet
	ACLTag       string
	AliasTag     string
	Next         plugin.Handler
	Zone         string
	lastUpdate   time.Time
	client       Client
	kv           KVClient
	Sources      []*Watch
	metrics      *metrics.Metrics
}

// New returns a Catalog plugin.
func New() *Catalog {
	return &Catalog{
		Endpoint: defaultEndpoint,
		Scheme:   "http",
		TTL:      defaultTTL,
		ACLTag:   defaultACLTag,
		AliasTag: defaultAliasTag,
		Sources:  []*Watch{},
	}
}

// SetClient sets a consul client for a catalog.
func (c *Catalog) SetClients(client Client, kv KVClient) {
	c.client = client
	c.kv = kv
}

// Ready implements ready.Readiness.
func (c *Catalog) Ready() bool {
	return c.client != nil && c.kv != nil
}

// LastUpdated returns the last time services changed.
func (c *Catalog) LastUpdated() time.Time {
	return c.lastUpdate
}

// Services returns a map of services to their target.
func (c *Catalog) Services() ServiceMap {
	m := ServiceMap{}
	for _, src := range c.Sources {
		for n, s := range src.Known() {
			if _, ok := m[n]; ok {
				Log.Warningf("Repeated service named %s from %s", n, src.Name())
				continue
			}
			m[n] = s
		}
	}

	return m
}

// Name implements plugin.Handler.
func (c *Catalog) Name() string { return "consul_catalog" }

func (c *Catalog) ServiceFor(name string) *Service {
	c.RLock()
	defer c.RUnlock()
	for _, src := range c.Sources {
		if svc := src.Get(name); svc != nil {
			return svc
		}
	}

	return nil
}

func (c *Catalog) ReloadAll() error {
	didUpdate := false
	for _, src := range c.Sources {
		changed, err := src.Resolve(c)
		if err != nil {
			return err
		}
		if changed {
			didUpdate = true
		}
	}

	if didUpdate {
		c.Lock()
		c.lastUpdate = time.Now()
		c.Unlock()
	}

	return nil
}

func (c *Catalog) parseACLString(svc *Service, acl string) error {
	aclRules := regexp.MustCompile(`;\s*`).Split(acl, -1)
	return c.parseACL(svc, aclRules)
}

func (c *Catalog) parseACL(svc *Service, rules []string) error {
	Log.Debugf("Parsing ACL for %s: %s", svc.Name, rules)
	for _, rule := range rules {
		ruleParts := strings.SplitN(rule, " ", 2)
		if len(ruleParts) != 2 {
			return fmt.Errorf("could not parse acl rule <%s>", rule)
		}
		action := ruleParts[0]
		for _, networkName := range regexp.MustCompile(`,\s*`).Split(ruleParts[1], -1) {
			if ranges, ok := c.Networks[networkName]; ok {
				svc.ACL = append(svc.ACL, &ServiceACL{
					Action:   action,
					Networks: ranges,
				})
			} else {
				return fmt.Errorf("unknown network %s", networkName)
			}
		}
	}

	return nil
}
