package validation

import (
	"encoding/json"
	"net"
	"net/url"
	"strings"
)

type SSRFValidator struct {
	allowPrivateNetworks []string
	allowedPrivateRanges []*net.IPNet
}

func NewSSRFValidator(allowPrivateNetworks []string) *SSRFValidator {
	v := &SSRFValidator{
		allowPrivateNetworks: allowPrivateNetworks,
	}
	for _, cidrStr := range allowPrivateNetworks {
		_, ipnet, err := net.ParseCIDR(cidrStr)
		if err == nil {
			v.allowedPrivateRanges = append(v.allowedPrivateRanges, ipnet)
		}
	}
	return v
}

func (v *SSRFValidator) Validate(data []byte) *ValidationReport {
	report := NewValidationReport()

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		report.AddError(CodeSchemaViolation, "Invalid JSON", "")
		return report
	}

	target, ok := config["target"].(map[string]interface{})
	if !ok {
		return report
	}

	urlStr, ok := target["url"].(string)
	if !ok || urlStr == "" {
		return report
	}

	v.validateURL(urlStr, report)
	return report
}

func (v *SSRFValidator) validateURL(urlStr string, report *ValidationReport) {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		report.AddError(CodeSchemaViolation, "Invalid URL format", "/target/url")
		return
	}

	v.validateScheme(parsed, report)
	v.validateUserInfo(parsed, report)
	v.validateHost(parsed, report)
}

func (v *SSRFValidator) validateScheme(parsed *url.URL, report *ValidationReport) {
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		report.AddErrorWithRemediation(CodeInvalidURLScheme,
			"Only HTTP and HTTPS schemes are allowed",
			"/target/url",
			"Use https:// or http:// URL scheme")
	}
}

func (v *SSRFValidator) validateUserInfo(parsed *url.URL, report *ValidationReport) {
	if parsed.User != nil {
		report.AddErrorWithRemediation(CodeUserInfoBlocked,
			"URLs with userinfo (user:pass@host) are not allowed",
			"/target/url",
			"Remove credentials from URL and use auth configuration instead")
	}
}

func (v *SSRFValidator) validateHost(parsed *url.URL, report *ValidationReport) {
	host := parsed.Hostname()
	if host == "" {
		report.AddError(CodeSchemaViolation, "URL must have a host", "/target/url")
		return
	}

	ip := net.ParseIP(host)
	if ip != nil {
		v.validateIPAddress(ip, report)
		return
	}

	v.validateHostname(host, report)
}

func (v *SSRFValidator) validateIPAddress(ip net.IP, report *ValidationReport) {
	// If IP is in allowed private networks, skip all IP literal blocking
	if v.isPrivateNetworkAllowed(ip) {
		return
	}

	report.AddErrorWithRemediation(CodeIPLiteralBlocked,
		"IP literal targets are not allowed",
		"/target/url",
		"Use a hostname instead of an IP address")

	if ip.IsLoopback() {
		report.AddError(CodeLoopbackBlocked, "Loopback addresses are blocked", "/target/url")
	}

	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		report.AddError(CodeLinkLocalBlocked, "Link-local addresses are blocked", "/target/url")
	}

	if ip.IsMulticast() {
		report.AddError(CodeMulticastBlocked, "Multicast addresses are blocked", "/target/url")
	}

	if ip4 := ip.To4(); ip4 != nil {
		v.validateIPv4(ip4, report)
	} else {
		v.validateIPv6(ip, report)
	}
}

func (v *SSRFValidator) validateIPv4(ip net.IP, report *ValidationReport) {
	blockedRanges := []struct {
		cidr string
		code string
		msg  string
	}{
		{"127.0.0.0/8", CodeLoopbackBlocked, "Loopback range (127.0.0.0/8) is blocked"},
		{"169.254.0.0/16", CodeLinkLocalBlocked, "Link-local range (169.254.0.0/16) is blocked"},
		{"169.254.169.254/32", CodeMetadataIPBlocked, "Cloud metadata IP (169.254.169.254) is blocked"},
		{"100.100.100.200/32", CodeMetadataIPBlocked, "Alibaba Cloud metadata IP is blocked"},
		{"192.0.0.0/24", CodePrivateAddressBlocked, "IETF protocol assignments (192.0.0.0/24) are blocked"},
		{"0.0.0.0/8", CodePrivateAddressBlocked, "This network (0.0.0.0/8) is blocked"},
	}

	for _, br := range blockedRanges {
		_, cidr, err := net.ParseCIDR(br.cidr)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			report.AddError(br.code, br.msg, "/target/url")
		}
	}

	rfc1918Ranges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, cidrStr := range rfc1918Ranges {
		_, cidr, err := net.ParseCIDR(cidrStr)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			if !v.isPrivateNetworkAllowed(ip) {
				report.AddErrorWithRemediation(CodePrivateAddressBlocked,
					"RFC 1918 private address is blocked by default",
					"/target/url",
					"Configure system_policy.allow_private_networks to allow specific private ranges")
			}
		}
	}
}

func (v *SSRFValidator) validateIPv6(ip net.IP, report *ValidationReport) {
	blockedRanges := []struct {
		cidr string
		code string
		msg  string
	}{
		{"::1/128", CodeLoopbackBlocked, "IPv6 loopback (::1) is blocked"},
		{"::/128", CodePrivateAddressBlocked, "IPv6 unspecified address (::) is blocked"},
		{"fc00::/7", CodeUniqueLocalBlocked, "IPv6 unique local addresses (fc00::/7) are blocked"},
		{"fe80::/10", CodeLinkLocalBlocked, "IPv6 link-local addresses (fe80::/10) are blocked"},
		{"ff00::/8", CodeMulticastBlocked, "IPv6 multicast addresses (ff00::/8) are blocked"},
		{"::ffff:0:0/96", CodeIPv4MappedBlocked, "IPv4-mapped IPv6 addresses are blocked"},
		{"64:ff9b::/96", CodeNAT64Blocked, "NAT64 addresses (64:ff9b::/96) are blocked"},
		{"2001:db8::/32", CodeDocumentationIPBlocked, "Documentation addresses (2001:db8::/32) are blocked"},
	}

	for _, br := range blockedRanges {
		_, cidr, err := net.ParseCIDR(br.cidr)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			report.AddError(br.code, br.msg, "/target/url")
		}
	}

	if len(ip) == 16 && ip[0] == 0 && ip[1] == 0 && ip[2] == 0 && ip[3] == 0 &&
		ip[4] == 0 && ip[5] == 0 && ip[6] == 0 && ip[7] == 0 &&
		ip[8] == 0 && ip[9] == 0 && ip[10] == 0xff && ip[11] == 0xff {
		ipv4 := ip[12:16]
		v.validateIPv4(ipv4, report)
	}
}

func (v *SSRFValidator) validateHostname(host string, report *ValidationReport) {
	lowerHost := strings.ToLower(host)

	localhostPatterns := []string{
		"localhost",
		"localhost.localdomain",
		"local",
	}

	for _, pattern := range localhostPatterns {
		if lowerHost == pattern || strings.HasSuffix(lowerHost, "."+pattern) {
			if v.isLoopbackAllowed() {
				return
			}
			report.AddError(CodeLocalhostBlocked,
				"Localhost hostnames are blocked",
				"/target/url")
			return
		}
	}

	if strings.HasSuffix(lowerHost, ".internal") ||
		strings.HasSuffix(lowerHost, ".local") ||
		strings.HasSuffix(lowerHost, ".localhost") {
		report.AddWarning(CodeLocalhostBlocked,
			"Hostname appears to be a local/internal address",
			"/target/url")
	}
}

func (v *SSRFValidator) isPrivateNetworkAllowed(ip net.IP) bool {
	for _, allowed := range v.allowedPrivateRanges {
		if allowed.Contains(ip) {
			return true
		}
	}
	return false
}

func (v *SSRFValidator) isLoopbackAllowed() bool {
	loopbackIP := net.ParseIP("127.0.0.1")
	return v.isPrivateNetworkAllowed(loopbackIP)
}

func (v *SSRFValidator) ValidateRedirectTarget(targetURL string, report *ValidationReport) {
	v.validateURL(targetURL, report)
}

type RedirectPolicy struct {
	Mode         string
	MaxRedirects int
}

func (v *SSRFValidator) ValidateRedirectPolicy(config map[string]interface{}, report *ValidationReport) {
	target, ok := config["target"].(map[string]interface{})
	if !ok {
		return
	}

	redirectPolicy, ok := target["redirect_policy"].(map[string]interface{})
	if !ok {
		report.AddError(CodeRedirectPolicyRequired,
			"redirect_policy must be explicitly configured",
			"/target/redirect_policy")
		return
	}

	mode, ok := redirectPolicy["mode"].(string)
	if !ok {
		report.AddError(CodeRedirectPolicyRequired,
			"redirect_policy.mode must be set",
			"/target/redirect_policy/mode")
		return
	}

	validModes := map[string]bool{
		"deny":           true,
		"same_origin":    true,
		"allowlist_only": true,
	}

	if !validModes[mode] {
		report.AddError(CodeSchemaViolation,
			"redirect_policy.mode must be one of: deny, same_origin, allowlist_only",
			"/target/redirect_policy/mode")
	}

	maxRedirects, ok := redirectPolicy["max_redirects"].(float64)
	if !ok {
		maxRedirects = 0
	}
	if maxRedirects > 3 {
		report.AddError(CodeMaxRedirectsExceeded,
			"max_redirects must not exceed 3",
			"/target/redirect_policy/max_redirects")
	}
}
