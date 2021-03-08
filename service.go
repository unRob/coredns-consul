package catalog

import (
	"net"
)

// ServiceACL holds an action and corresponding network range
type ServiceACL struct {
	Action  string
	Network *net.IPNet
}

// Service has a target and ACL rules
type Service struct {
	Target string
	ACL    []*ServiceACL
}

// RespondsTo returns if a service is allowed to talk to an IP
func (s Service) RespondsTo(ip net.IP) bool {
	for _, acl := range s.ACL {
		if acl.Network.Contains(ip) {
			if acl.Action == "allow" {
				return true
			} else if acl.Action == "deny" {
				return false
			} else {
				Log.Errorf("unknown acl action: %s", acl.Action)
			}
		}
	}

	return false
}
