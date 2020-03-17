# consul_catalog

## Name

*consul_catalog* - enables serving A resources for tagged consul services

## Description

This plugin reads services from the [Consul Catalog](https://www.consul.io/api/catalog.html#list-services), and serves A records to them if tagged with specified tags.


## Syntax

~~~
consul_catalog [TAGS...]
~~~

With only the plugin specified, the *consul_catalog* plugin will default to the "coredns.enabled" tag. If **TAGS** is specified, only services matching these exact tags will be considered for serving.

```
consul_catalog [TAGS...] {
    endpoint URL
    token TOKEN
    ttl TTL
}
```

* `endpoint` specifies the **URL** where to find consul catalog, by default `consul.service.consul:8500`.
* `token` specifies the token to authenticate with the consul service.
* `ttl` specifies the **TTL** in [golang duration strings](https://golang.org/pkg/time/#ParseDuration) returned for matching service queries, by default 5 minutes.

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
        address consul.service.consul:8500
        token CONSUL_ACL_TOKEN
        ttl 10m
    }

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
