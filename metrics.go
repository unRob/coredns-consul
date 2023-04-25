// Copyright © 2022 Roberto Hidalgo <coredns-consul@un.rob.mx>
// SPDX-License-Identifier: Apache-2.0
package catalog

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestBlockCount is the number of DNS requests being blocked.
	RequestBlockCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "blocked_requests_total",
		Help:      "Counter of DNS requests being blocked.",
	}, []string{"server", "view"})
	// RequestACLDeniedCount is the number of DNS requests being filtered.
	RequestACLDeniedCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "denied_requests_total",
		Help:      "Counter of DNS requests being filtered.",
	}, []string{"server", "view"})
	// RequestServedCount is the number of DNS requests being Allowed.
	RequestServedCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "served_requests_total",
		Help:      "Counter of DNS requests being served by plugin.",
	}, []string{"server", "view", "source"})
	// RequestDropCount is the number of DNS requests being dropped.
	RequestDropCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "dropped_requests_total",
		Help:      "Counter of DNS requests being dropped.",
	}, []string{"server", "view"})
)
