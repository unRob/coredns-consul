package catalog

import (
	"testing"

	"github.com/caddyserver/caddy"
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
	}{
		{
			input:       `consul_catalog`,
			shouldError: false,
			tags:        defaultTags,
			endpoint:    defaultEndpoint,
			ttl:         defaultTTL,
		},
		{
			input:       `consul_catalog some.tag`,
			shouldError: false,
			tags:        []string{"some.tag"},
			endpoint:    defaultEndpoint,
			ttl:         defaultTTL,
		},
		{
			input:       `consul_catalog some.tag other.tag`,
			shouldError: false,
			tags:        []string{"some.tag", "other.tag"},
			endpoint:    defaultEndpoint,
			ttl:         defaultTTL,
		},
		{
			input: `consul_catalog {
				endpoint consul.local:1111
			}`,
			shouldError: false,
			tags:        defaultTags,
			endpoint:    "consul.local:1111",
			ttl:         defaultTTL,
		},
		{
			input: `consul_catalog {
				ttl 15s
			}`,
			shouldError: false,
			tags:        defaultTags,
			endpoint:    defaultEndpoint,
			ttl:         15,
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

			if len(catalog.Tags) != len(tst.tags) {
				t.Fatalf("Tags don't match: %v != %v", catalog.Tags, tst.tags)
			}

			if catalog.Endpoint != tst.endpoint {
				t.Fatalf("Endpoint doesn't match: %v != %v", catalog.Endpoint, tst.endpoint)
			}

			if catalog.TTL != tst.ttl {
				t.Fatalf("TTL doesn't match: %v != %v", catalog.TTL, tst.ttl)
			}
		})
	}

}
