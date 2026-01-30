package validation

import (
	"net"
	"sync"
)

type DNSCache struct {
	mu      sync.RWMutex
	entries map[string][]net.IP
}

func NewDNSCache() *DNSCache {
	return &DNSCache{
		entries: make(map[string][]net.IP),
	}
}

func (c *DNSCache) Lookup(hostname string) ([]net.IP, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ips, ok := c.entries[hostname]
	return ips, ok
}

func (c *DNSCache) Store(hostname string, ips []net.IP) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[hostname] = ips
}

func (c *DNSCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string][]net.IP)
}

type DNSRebindingValidator struct {
	cache             *DNSCache
	ssrfValidator     *SSRFValidator
	blockedIPv4Ranges []*net.IPNet
	blockedIPv6Ranges []*net.IPNet
}

func NewDNSRebindingValidator(allowPrivateNetworks []string) *DNSRebindingValidator {
	v := &DNSRebindingValidator{
		cache:         NewDNSCache(),
		ssrfValidator: NewSSRFValidator(allowPrivateNetworks),
	}

	ipv4Blocked := []string{
		"127.0.0.0/8",
		"169.254.0.0/16",
		"169.254.169.254/32",
		"100.100.100.200/32",
		"192.0.0.0/24",
		"0.0.0.0/8",
	}

	for _, cidr := range ipv4Blocked {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err == nil {
			v.blockedIPv4Ranges = append(v.blockedIPv4Ranges, ipnet)
		}
	}

	ipv6Blocked := []string{
		"::1/128",
		"::/128",
		"fc00::/7",
		"fe80::/10",
		"ff00::/8",
		"::ffff:0:0/96",
		"64:ff9b::/96",
		"2001:db8::/32",
	}

	for _, cidr := range ipv6Blocked {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err == nil {
			v.blockedIPv6Ranges = append(v.blockedIPv6Ranges, ipnet)
		}
	}

	return v
}

func (v *DNSRebindingValidator) ValidateResolvedIPs(hostname string, ips []net.IP) *ValidationReport {
	report := NewValidationReport()

	cachedIPs, hasCached := v.cache.Lookup(hostname)

	for _, ip := range ips {
		if v.isIPBlocked(ip) {
			report.AddError(CodeDNSRebindingBlocked,
				"DNS resolution returned blocked IP address",
				"/target/url")
			return report
		}
	}

	if hasCached {
		if !v.ipsMatch(cachedIPs, ips) {
			for _, ip := range ips {
				if v.isIPBlocked(ip) {
					report.AddError(CodeDNSRebindingBlocked,
						"DNS rebinding detected: hostname resolved to blocked IP",
						"/target/url")
					return report
				}
			}
		}
	}

	v.cache.Store(hostname, ips)
	return report
}

func (v *DNSRebindingValidator) isIPBlocked(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		for _, blocked := range v.blockedIPv4Ranges {
			if blocked.Contains(ip4) {
				return true
			}
		}

		rfc1918 := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
		for _, cidr := range rfc1918 {
			_, ipnet, _ := net.ParseCIDR(cidr)
			if ipnet.Contains(ip4) {
				if !v.ssrfValidator.isPrivateNetworkAllowed(ip4) {
					return true
				}
			}
		}
	} else {
		for _, blocked := range v.blockedIPv6Ranges {
			if blocked.Contains(ip) {
				return true
			}
		}
	}

	return false
}

func (v *DNSRebindingValidator) ipsMatch(a, b []net.IP) bool {
	if len(a) != len(b) {
		return false
	}

	aSet := make(map[string]bool)
	for _, ip := range a {
		aSet[ip.String()] = true
	}

	for _, ip := range b {
		if !aSet[ip.String()] {
			return false
		}
	}

	return true
}

func (v *DNSRebindingValidator) ClearCache() {
	v.cache.Clear()
}
