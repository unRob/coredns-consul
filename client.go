// Copyright © 2022 Roberto Hidalgo <coredns-consul@un.rob.mx>
// Contributions by Charles Powell, 2023
// SPDX-License-Identifier: Apache-2.0
package catalog

import (
	"time"

	"github.com/hashicorp/consul/api"
)

var watchTimeout = 10 * time.Minute

// Client is implemented by github.com/hashicorp/consul/api.Catalog.
type Client interface {
	Service(string, string, *api.QueryOptions) ([]*api.CatalogService, *api.QueryMeta, error)
	Services(*api.QueryOptions) (map[string][]string, *api.QueryMeta, error)
}

// KVClient is implemented by github.com/hashicorp/consul/api.Catalog.
type KVClient interface {
	Get(key string, opts *api.QueryOptions) (*api.KVPair, *api.QueryMeta, error)
	List(prefix string, opts *api.QueryOptions) (api.KVPairs, *api.QueryMeta, error)
}

// CreateClient initializes the consul catalog client.
func CreateClient(scheme, endpoint, token string) (catalog Client, kv KVClient, err error) {
	cfg := api.DefaultConfig()
	cfg.Address = endpoint
	if token != "" {
		cfg.Token = token
	}

	if scheme == "https" {
		cfg.Scheme = "https"
	}

	client, err := api.NewClient(cfg)

	if err != nil {
		return
	}

	catalog = client.Catalog()
	kv = client.KV()
	return
}

// StaticEntry represents a consul value, json encoded.
type StaticEntry struct {
	Target    string   `json:"target"`
	Addresses []string `json:"addresses"`
	ACL       []string `json:"acl"`
	Aliases   []string `json:"aliases"`
}

type StaticEntries map[string]*StaticEntry
