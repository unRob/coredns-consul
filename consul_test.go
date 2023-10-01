// Copyright © 2022 Roberto Hidalgo <coredns-consul@un.rob.mx>
// SPDX-License-Identifier: Apache-2.0
package catalog_test

import (
	"testing"

	. "github.com/unRob/coredns-consul"
)

var serviceProxyName = "traefik"

func TestFetchStaticServiceKey(t *testing.T) {
	src := NewWatch(&WatchKVPath{Key: "static/path"})
	c, _, _ := NewTestCatalog(true, src)

	t.Run("static target", func(t *testing.T) {
		svc := c.ServiceFor("static-consul")
		if svc == nil {
			t.Fatalf("Service static-consul not found, got: %+v", c.Services())
		}

		if svc.Target != serviceProxyName {
			t.Fatalf("Unexpected target: %v", svc.Target)
		}
	})

	t.Run("static addresses", func(t *testing.T) {
		svc := c.ServiceFor("static-addr")
		if svc == nil {
			t.Fatalf("Service static-addr not found, got: %+v", c.Services())
		}

		if len(svc.Addresses) != 1 {
			t.Fatalf("Unexpected addresses: %v", svc.Target)
		}

		if ip := svc.Addresses[0].String(); ip != "127.0.0.1" {
			t.Fatalf("Unexpected address for static-addr: %v", ip)
		}
	})

	t.Run("invalid static addresses", func(t *testing.T) {
		svc := c.ServiceFor("static-addr-invalid")
		if svc == nil {
			t.Fatalf("Service static-addr-invalid not found, got: %+v", c.Services())
		}

		if len(svc.Addresses) != 1 {
			t.Fatalf("Unexpected addresses for static-addr-invalid: %v", svc.Target)
		}

		if ip := svc.Addresses[0].String(); ip != "127.0.0.1" {
			t.Fatalf("Unexpected address for static-addr-invalid: %v", ip)
		}
	})

	t.Run("ignored static", func(t *testing.T) {
		svc := c.ServiceFor("static-ignored")
		if svc != nil {
			t.Fatalf("Service static-addr found with invalid config, got: %+v", c.Services())
		}
	})
}

func TestFetchStaticServicePrefix(t *testing.T) {
	src := NewWatch(&WatcKVPrefix{Prefix: "static/prefix"})
	c, _, _ := NewTestCatalog(true, src)

	svc := c.ServiceFor("prefixed-static")
	if svc == nil {
		t.Fatalf("Service consul not found")
	}

	if svc.Target != serviceProxyName {
		t.Fatalf("Unexpected target: %v", svc.Target)
	}
}

func TestFetchServices(t *testing.T) {
	c, client, _ := NewTestCatalog(true)

	services := c.Services()
	if len(services) != 3 {
		t.Fatalf("Unexpected number of services: %d", len(services))
	}

	svcTests := map[string]string{
		"nomad":          ServiceProxyTag,
		serviceProxyName: ServiceProxyTag,
		"git":            "git",
	}
	for svc, expected := range svcTests {
		target, exists := services[svc]
		if !exists {
			t.Fatalf("Expected service %s not found", svc)
		}

		if target.Target != expected {
			t.Fatalf("Unexpected target: %v", target)
		}
	}

	lastUpdate := c.LastUpdated()
	err := c.ReloadAll()
	if err != nil {
		t.Fatalf("Fetch services: %v", err)
	}

	if lastUpdate != c.LastUpdated() {
		t.Fatalf("Services changed after timeout")
	}

	err = c.ReloadAll()
	if err != nil {
		t.Fatalf("Fetch services: %v", err)
	}

	testclient := client.(*testCatalogClient)
	testclient.DeleteService("git")
	if err := c.ReloadAll(); err != nil {
		t.Fatalf("could not fetch services: %s", err)
	}

	if lastUpdate == c.LastUpdated() {
		t.Fatalf("Services did not change after update")
	}

	if newCount := len(c.Services()); newCount != 2 {
		t.Fatalf("Unexpected number of services after update: %d", newCount)
	}
}
