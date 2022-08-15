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
	Name      string
	Target    string
	ACL       []*ServiceACL
	Addresses []net.IP
}

// RespondsTo returns if a service is allowed to talk to an IP
func (s Service) RespondsTo(ip net.IP) bool {
	Log.Debugf("Evaluating %d rules", len(s.ACL))
	for _, acl := range s.ACL {
		Log.Debugf("Evaluating %s", acl.Network)
		if acl.Network.Contains(ip) {
			if acl.Action == "allow" {
				Log.Debugf("Allowed %s from %s", ip, acl.Network)
				return true
			} else if acl.Action == "deny" {
				Log.Debugf("Denied %s from %s", ip, acl.Network)
				return false
			} else {
				Log.Errorf("unknown acl action: %s", acl.Action)
			}
		}
	}

	return false
}
