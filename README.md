# consul_catalog

[![Coverage Status](https://coveralls.io/repos/github/unRob/coredns-consul/badge.svg)](https://coveralls.io/github/unRob/coredns-consul)

## Name

*consul_catalog* - enables serving A resources for tagged consul services.

## Description

This plugin reads services from the [Consul Catalog](https://www.consul.io/api/catalog.html#list-services), and serves A records from them when tagged with `coredns.enabled`. A list of services can also be served from Consul's KV.

> ⚠️ While running in my home cluster for 3+ years, this is still **unstable, alpha software** with limited test coverage and a very narrow feature set (that's not likely to change, though).

## Installation

Grab a build from [the releases](https://github.com/unRob/coredns-consul/releases), or follow [these instructions](https://coredns.io/2017/03/01/how-to-add-plugins-to-coredns/#4-hooking-it-up) (tl;dr; add `consul_catalog:github.com/unRob/consul_catalog` to CoreDNS' `plugin.cfg` and `make coredns`). An example Ansible role for installing on a Ubiquiti Edgerouter can be found at [unRob/nidito](https://github.com/unRob/nidito/tree/c8b63f47b1703beb8a29171c1e7fc52abb4ecceb/ansible/roles/coredns).

## Syntax

~~~
consul_catalog [TAGS...]
~~~

**TAG** defaults to `coredns.enabled`, and only services tagged with this exact value will be served by this plugin.

```hcl
consul_catalog [TAGS...] {
    # the hostname and port to reach consul at
    endpoint URL
    # to enable tls encryption, might need your cluster's CA certificates installed!
    scheme https
    # a consul ACL token
    token TOKEN

    # ACL configuration
    acl_metadata_tag META_TAG
    acl_zone ZONE_NAME ZONE_CIDR

    # Service proxy allows static services to target a Catalog service
    service_proxy PROXY_TAG PROXY_SERVICE

    # Services can have multiple names
    alias_metadata_tag META_TAG_NAME

    # Or be fetched from the KV store at a path or prefix (ending in /)
    static_entries_path CONSUL_KV_PATH
    static_entries_prefix CONSUL_KV_PREFIX/

    # finally, records served can be attached with a default ttl
    ttl TTL
}
```

* `endpoint` (default `consul.service.consul:8500`) specifies the host and port where to find consul catalog.
* `token` specifies the token to authenticate with the consul service, having at least .
* `acl_metadata_tag` (default: `coredns-acl`) specifies the Consul Metadata tag to read ACL rules from. An ACL rule looks like: `allow network1; deny network2`. Rules are interpreted in order of appearance. If specified, requests will only receive answers when their IP address corresponds to any of the allowed `acl_zone`s' CIDR ranges for a service.
* `acl_zone` adds an ACL zone named **ZONE_NAME** with corresponding **ZONE_CIDR** range.
* `service_proxy` If specified, services tagged with **PROXY_TAG** will respond with the address for **PROXY_SERVICE** instead.
* `alias_metadata_tag` (default: `coredns-alias`) specifies the Consul Metadata tag to read aliases to setup for service. Aliases are semicolon separated dns prefixes that reply with the same target as the original service. For example: `coredns-alias = "*.myservice; client.myservice"`. Aliases that begin with `*.`, are treated as a wildcard prefix that will match any sub-domains of the `zone` (and/or dots after the `*.` prefix).
* `static_entries_path` If specified, consul's kv store will be queried at **CONSUL_KV_PATH** and specified entries will be served before querying for catalog records. The value at **CONSUL_KV_PATH** must contain json following this schema:
    ```jsonc
    {
        "staticService": { // matches staticService.{coredns_zone}
            "target": "serviceA", // the name of a service registered with consul
            "acl": ["allow network1", "deny network2"], // a list of ACL rules
            "aliases": ["*.static"] // a list of other names that should also reply with this service's info
        },
        "myServiceProxyService": {
            "target": "@service_proxy", // a run-time alias for acl_zone's PROXY_SERVICE
            "acl": ["allow network1"]
        },
        "my-a-record": {
          "addresses": ["127.0.0.1"], // static addresses for this name; no `target` is provided,
          "acl": ["allow network1"]
        }
    }
    ```
* `static_entries_prefix` If specified, consul's kv store will be queried for all keys under **CONSUL_KV_PREFIX** and found entries will be served before querying for catalog records. The keys at **CONSUL_KV_PREFIX** must contain json-encoded values following this schema:
    ```jsonc
    {
        "target": "serviceC", // the name of a service registered with consul
        "acl": ["allow network1", "deny network2"], // a list of ACL rules
        "aliases": ["qa.business", "demo.business"], // test in prod or live a lie
        // "addresses": ["127.0.0.1"] // static addresses for this name, if no `target` was provided
    }
    ```
* `ttl` (default: `5m`) specifies the **TTL** in [golang duration strings](https://golang.org/pkg/time/#ParseDuration) returned for matching service queries.

## Ready

This plugin reports readiness to the ready plugin. This will happen after it has synced to the Consul Catalog API.

## Examples

Handle all the queries in the `example.com` zone, first by looking into hosts, then consul, and finally a zone file. Queries for services in the catalog at `consul.service.consul:8500` with a `coredns.enabled` tag will be answered with the addresses for `$SERVICE_NAME.services.consul`. If the service also includes a `traefik.enabled` tag, queries will be answered with the addresses for `traefik.service.consul`.

~~~ txt

example.com {
    hosts {
        10.0.0.42 fourtytwo.example.com
        fallthrough
    }

    consul_catalog coredns.enabled {
        address localhost:8501
        scheme https
        token CONSUL_ACL_TOKEN

        // Enable ACL
        acl_metada_tag coredns-consul
        // A service with `coredns-acl = "trusted" will only reply to clients in 10.0.0.0/24
        acl_zone trusted 10.0.0.0/24
        acl_zone guests 192.168.10.0/24
        acl_zone iot 192.168.20.0/24
        acl_zone public 0.0.0.0/0

        static_entries_path dns-records
        static_entries_prefix dns/records/

        ttl 10m
    }

    # if a SOA is specified in this file, it'll be added
    # to responses from consul services
    file zones/example.com
}

consul {
    # Forward all requests to consul
    forward . 10.0.0.42:8600 10.0.0.43:8600 10.0.0.44:8600 {
        policy sequential
    }
}

. {
    forward . 1.1.1.1 8.8.8.8
    errors
    cache
}
~~~

## Consul configuration

Services registered in the catalog and tagged with `TAG` will be served by this plugin. If `acl_metadata_tag` was configured in coredns, services must also provide that key as part of it's [metadata](https://developer.hashicorp.com/consul/api-docs/agent/service#meta).

### Consul ACL policy

```hcl
// Catalog access requires reading service and node data
service_prefix "" {
  policy = "read"
}

node_prefix "" {
  policy = "read"
}

// When using static_entries_(path|prefix), access to the given path/prefix should be granted
// for a path:
key "dns-records" {
  policy = "read"
}

// for a prefix:
key_prefix "dns/records" {
  policy = "read"
}
```
