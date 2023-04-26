// Copyright © 2022 Roberto Hidalgo <coredns-consul@un.rob.mx>
// SPDX-License-Identifier: Apache-2.0
package catalog

import (
	"net"
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {
	defaultTTL := uint32(300)
	defaultEndpoint := "consul.service.consul:8500"
	defaultTags := []string{"coredns.enabled"}

	tests := []struct {
		input       string
		shouldError bool
		tags        []string
		endpoint    string
		ttl         uint32
		metaTag     string
		networks    map[string]*net.IPNet
	}{
		{
			input:       `consul_catalog`,
			shouldError: false,
			tags:        defaultTags,
			endpoint:    defaultEndpoint,
			ttl:         defaultTTL,
			metaTag:     defaultACLTag,
		},
		{
			input:       `consul_catalog some.tag`,
			shouldError: false,
			tags:        []string{"some.tag"},
			endpoint:    defaultEndpoint,
			ttl:         defaultTTL,
			metaTag:     defaultACLTag,
		},
		{
			input:       `consul_catalog some.tag other.tag`,
			shouldError: false,
			tags:        []string{"some.tag", "other.tag"},
			endpoint:    defaultEndpoint,
			ttl:         defaultTTL,
			metaTag:     defaultACLTag,
		},
		{
			input: `consul_catalog {
				endpoint consul.local:1111
			}`,
			shouldError: false,
			tags:        defaultTags,
			endpoint:    "consul.local:1111",
			ttl:         defaultTTL,
			metaTag:     defaultACLTag,
		},
		{
			input: `consul_catalog {
				ttl 15s
			}`,
			shouldError: false,
			tags:        defaultTags,
			endpoint:    defaultEndpoint,
			ttl:         15,
			metaTag:     defaultACLTag,
		},
		{

			input: `consul_catalog {
				acl_metadata_tag some-tag
			}`,
			shouldError: false,
			tags:        defaultTags,
			endpoint:    defaultEndpoint,
			ttl:         defaultTTL,
			metaTag:     "some-tag",
		},
		{
			input: `consul_catalog {
				acl_zone private 10.0.0.1/24
				acl_zone public 0.0.0.0/0
			}`,
			shouldError: false,
			tags:        defaultTags,
			endpoint:    defaultEndpoint,
			ttl:         defaultTTL,
			metaTag:     defaultACLTag,
			networks: map[string]*net.IPNet{
				"private": {IP: net.ParseIP("10.0.0.0"), Mask: net.IPv4Mask(255, 255, 255, 0)},
				"public":  {IP: net.ParseIP("0.0.0.0"), Mask: net.IPv4Mask(0, 0, 0, 0)},
			},
		},
		{
			input: `consul_catalog {
				whatever
			}`,
			shouldError: true,
		},
	}

	for _, tst := range tests {
		t.Run(tst.input, func(t *testing.T) {
			c := caddy.NewTestController("dns", tst.input)
			catalog, err := parse(c)

			if tst.shouldError {
				if err == nil {
					t.Fatalf("Expected errors, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Expected no errors, but got: %v", err)
			}

			lastSourceDesc := strings.Split(catalog.Sources[len(catalog.Sources)-1].Name(), " ")
			lastSourceName := lastSourceDesc[len(lastSourceDesc)-1]
			if lastSourceName != tst.tags[0] {
				t.Fatalf("Tags don't match: %v != %v", lastSourceName, tst.tags[0])
			}

			if catalog.Endpoint != tst.endpoint {
				t.Fatalf("Endpoint doesn't match: %v != %v", catalog.Endpoint, tst.endpoint)
			}

			if catalog.TTL != tst.ttl {
				t.Fatalf("TTL doesn't match: %v != %v", catalog.TTL, tst.ttl)
			}

			for name, cidr := range catalog.Networks {
				if expectedCIDR, ok := tst.networks[name]; !ok {
					t.Fatalf("Networks missing %s", name)
				} else if expectedCIDR.String() != cidr.String() {
					t.Fatalf("Wrong CIDR found: %s, expected %s", cidr, expectedCIDR)
				}
			}
		})
	}
}
