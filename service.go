// Copyright © 2022 Roberto Hidalgo <coredns-consul@un.rob.mx>
// SPDX-License-Identifier: Apache-2.0
package catalog

import (
	"net"
	"strings"
)

// ServiceACL holds an action and corresponding network range.
type ServiceACL struct {
	Action   string
	Networks []*net.IPNet
}

// Service has a target and ACL rules.
type Service struct {
	Name      string
	Target    string
	ACL       []*ServiceACL
	Addresses []net.IP
}

func NewService(name, target string) *Service {
	svc := &Service{
		Name:      name,
		Target:    target,
		ACL:       []*ServiceACL{},
		Addresses: []net.IP{},
	}

	return svc
}

// RespondsTo returns if a service is allowed to talk to an IP.
func (s Service) RespondsTo(ip net.IP) bool {
	Log.Debugf("Evaluating %d rules", len(s.ACL))
	for _, acl := range s.ACL {
		Log.Debugf("Evaluating %s", acl.Networks)
		for _, net := range acl.Networks {
			if net.Contains(ip) {
				switch acl.Action {
				case "allow":
					Log.Debugf("Allowed %s from %s", ip, acl.Networks)
					return true
				case "deny":
					Log.Debugf("Denied %s from %s", ip, acl.Networks)
					return false
				default:
					Log.Errorf("unknown acl action: %s", acl.Action)
				}
			}
		}
	}

	return false
}

type ServiceMap map[string]*Service

func (s ServiceMap) Find(query string) *Service {
	if svc, ok := s[query]; ok {
		return svc
	}

	if strings.Contains(query, ".") {
		foundDot := false
		starName := "*." + strings.TrimLeftFunc(query, func(r rune) bool {
			if foundDot {
				return false
			}

			if r == '.' {
				foundDot = true
			}
			return true
		})
		if svc, ok := s[starName]; ok {
			return svc
		}
	}

	return nil
}
