package catalog_test

import (
	"fmt"
	"net"

	"github.com/hashicorp/consul/api"
	. "github.com/unRob/coredns-consul"
)

func NewTestCatalog(fetch bool, extraSources ...*Watch) (*Catalog, CatalogClient, KVClient) {
	c := New()
	c.FQDN = []string{"example.com."}
	c.ProxyTag = "traefik.enable=true"
	c.ProxyService = "traefik"
	c.Networks = map[string]*net.IPNet{}
	_, private, _ := net.ParseCIDR("192.168.100.0/24")
	c.Networks["private"] = private
	_, guest, _ := net.ParseCIDR("192.168.1.0/24")
	c.Networks["guest"] = guest
	_, public, _ := net.ParseCIDR("0.0.0.0/0")
	c.Networks["public"] = public
	client := NewTestCatalogClient()
	kvClient := NewTestKVClient()
	c.SetClients(client, kvClient)

	catalogSource := &WatchConsulCatalog{Tag: "coredns.enabled"}
	c.Sources = extraSources
	c.Sources = append(c.Sources, NewWatch(catalogSource))

	if fetch {
		for _, src := range c.Sources {
			_, err := src.Resolve(c)
			if err != nil {
				panic(err)
			}
		}
	}
	return c, client, kvClient
}

type testServiceData struct {
	Tags    []string
	Meta    map[string]string
	Address string
}

type testCatalogClient struct {
	services  map[string][]*testServiceData
	lastIndex uint64
}

func NewTestCatalogClient() CatalogClient {
	return &testCatalogClient{
		lastIndex: 4,
		services: map[string][]*testServiceData{
			"nomad": {
				{
					Address: "192.168.100.1",
					Tags: []string{
						"coredns.enabled",
						"traefik.enable=true",
					},
					Meta: map[string]string{
						"coredns-acl": "allow private",
					},
				},
			},
			"nomad-client": {
				{
					Address: "192.168.100.1",
					Tags:    []string{},
					Meta:    map[string]string{},
				},
			},
			"traefik": {
				{
					Address: "192.168.100.2",
					Tags: []string{
						"coredns.enabled",
						"traefik.enable=true",
					},
					Meta: map[string]string{
						"coredns-acl": "allow private, guest; deny public",
					},
				},
			},
			"git": {
				{
					Address: "192.168.100.3",
					Tags:    []string{"coredns.enabled"},
					Meta: map[string]string{
						"coredns-acl": "deny guest; allow public",
					},
				},
				{
					Address: "192.168.100.4",
					Tags:    []string{"coredns.enabled"},
					Meta: map[string]string{
						"coredns-acl": "deny guest; allow public",
					},
				},
			},
		},
	}
}

func (c *testCatalogClient) DeleteService(name string) {
	if _, ok := c.services[name]; !ok {
		Log.Infof("deleting unknown service")
	}
	delete(c.services, name)
}

func (c *testCatalogClient) Service(name string, tag string, opts *api.QueryOptions) ([]*api.CatalogService, *api.QueryMeta, error) {
	sd, ok := c.services[name]
	if !ok {
		return []*api.CatalogService{}, nil, fmt.Errorf("Not found")
	}

	services := []*api.CatalogService{}
	for _, nodeService := range sd {
		services = append(services, &api.CatalogService{
			ID:          "42",
			ServiceName: name,
			Node:        fmt.Sprintf("node-%s", nodeService.Address),
			Address:     nodeService.Address,
			ServiceMeta: nodeService.Meta,
			ServiceTags: nodeService.Tags,
		})
	}
	return services, &api.QueryMeta{}, nil
}

func (c *testCatalogClient) Services(*api.QueryOptions) (map[string][]string, *api.QueryMeta, error) {
	services := map[string][]string{}
	for name, svc := range c.services {
		services[name] = svc[0].Tags
	}

	c.lastIndex = uint64(len(services))
	return services, &api.QueryMeta{LastIndex: c.lastIndex}, nil
}

type testKVClient struct {
	Keys        map[string]*api.KVPair
	keysIndex   uint64
	Prefixes    map[string]api.KVPairs
	prefixIndex uint64
}

func NewTestKVClient() KVClient {
	return &testKVClient{
		Keys: map[string]*api.KVPair{
			"static/path": {
				Key:   "static/path",
				Value: []byte(`{"static-consul": {"target": "traefik", "acl": ["allow private"]}}`),
			},
		},
		Prefixes: map[string]api.KVPairs{
			"static/prefix": {
				{
					Key:   "static/prefix/prefixed-static",
					Value: []byte(`{"target": "traefik", "acl": ["allow private"]}`),
				},
			},
		},
	}
}

func (kv *testKVClient) Get(path string, opts *api.QueryOptions) (*api.KVPair, *api.QueryMeta, error) {
	kv.keysIndex += 1
	return kv.Keys[path], &api.QueryMeta{LastIndex: kv.keysIndex}, nil
}

func (kv *testKVClient) List(prefix string, opts *api.QueryOptions) (api.KVPairs, *api.QueryMeta, error) {
	kv.prefixIndex += 1
	return kv.Prefixes[prefix], &api.QueryMeta{LastIndex: kv.prefixIndex}, nil
}
