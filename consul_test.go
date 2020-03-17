package catalog

import (
	"fmt"
	"testing"
)

func TestFetchServices(t *testing.T) {
	nextIndex := uint64(42)
	allServices := map[string][]string{
		"nomad": []string{
			"coredns.enabled",
			"traefik.enable=true",
		},
		"nomad-client": []string{},
		"traefik": []string{
			"coredns.enabled",
			"traefik.enable=true",
		},
		"git": []string{
			"coredns.enabled",
		},
	}
	fetchFromConsul = func(lastIndex uint64) (map[string][]string, uint64, error) {
		return allServices, nextIndex, nil
	}

	c := New()
	err := c.FetchServices()
	if err != nil {
		t.Fatalf("Fetch services: %v", err)
	}

	services := c.Services()
	if len(services) != 3 {
		t.Fatalf("Unexpected number of services: %d", len(services))
	}

	svcTests := map[string]string{
		"nomad":   "traefik",
		"traefik": "traefik",
		"git":     "git",
	}
	for svc, expected := range svcTests {
		target, exists := services[svc]
		if !exists {
			t.Fatalf("Expected service %s not found", svc)
		}

		if target != fmt.Sprintf("%s.service.consul.", expected) {
			t.Fatalf("Unexpected target: %s", target)
		}
	}

	lastUpdate := c.LastUpdated()
	err = c.FetchServices()
	if err != nil {
		t.Fatalf("Fetch services: %v", err)
	}

	if lastUpdate != c.LastUpdated() {
		t.Fatalf("Services changed after timeout")
	}

	nextIndex = uint64(314)
	delete(allServices, "git")
	err = c.FetchServices()
	if err != nil {
		t.Fatalf("Fetch services: %v", err)
	}

	if lastUpdate == c.LastUpdated() {
		t.Fatalf("Services did not change after update")
	}

	newCount := len(c.Services())
	if newCount != 2 {
		t.Fatalf("Unexpected number of services after update: %d", newCount)
	}

}
