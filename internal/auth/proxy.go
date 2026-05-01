package auth

import (
	"net"
	"net/http"
	"strings"
)

// ProxyAuth pulls the configured headers off a request when the remote
// IP is in TrustedCIDRs. Used by the Service in middleware composition.
type ProxyAuth struct {
	Enabled      bool
	TrustedCIDRs []*net.IPNet
	UserHeader   string
	EmailHeader  string
	GroupsHeader string
	AdminGroups  []string
}

// NewProxyAuth parses CIDR strings and returns a configured ProxyAuth.
// Bad CIDRs are skipped (callers should validate config separately).
func NewProxyAuth(enabled bool, cidrs []string, userH, emailH, groupsH string, adminGroups []string) *ProxyAuth {
	pa := &ProxyAuth{
		Enabled:      enabled,
		UserHeader:   firstNonEmpty(userH, "Remote-User"),
		EmailHeader:  firstNonEmpty(emailH, "Remote-Email"),
		GroupsHeader: firstNonEmpty(groupsH, "Remote-Groups"),
		AdminGroups:  adminGroups,
	}
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(strings.TrimSpace(c))
		if err == nil {
			pa.TrustedCIDRs = append(pa.TrustedCIDRs, n)
		}
	}
	return pa
}

// IsTrusted reports whether r.RemoteAddr (after RealIP) is inside any
// configured CIDR.
func (p *ProxyAuth) IsTrusted(r *http.Request) bool {
	if p == nil || !p.Enabled || len(p.TrustedCIDRs) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	for _, n := range p.TrustedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// HeaderIdentity reads the configured headers off r. Returns nil if the
// user header is empty (caller falls through to the next mode).
func (p *ProxyAuth) HeaderIdentity(r *http.Request) *Identity {
	if p == nil || !p.Enabled {
		return nil
	}
	user := strings.TrimSpace(r.Header.Get(p.UserHeader))
	if user == "" {
		return nil
	}
	email := strings.TrimSpace(r.Header.Get(p.EmailHeader))
	groupsRaw := strings.TrimSpace(r.Header.Get(p.GroupsHeader))
	roles := []string{"user"}
	if groupsRaw != "" {
		for _, g := range strings.Split(groupsRaw, ",") {
			g = strings.TrimSpace(g)
			if g == "" {
				continue
			}
			roles = appendUnique(roles, g)
			for _, admin := range p.AdminGroups {
				if g == admin {
					roles = appendUnique(roles, "admin")
				}
			}
		}
	}
	return &Identity{
		Username:   user,
		Email:      email,
		Roles:      roles,
		AuthMethod: MethodProxy,
	}
}

func firstNonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func appendUnique(roles []string, role string) []string {
	for _, r := range roles {
		if r == role {
			return roles
		}
	}
	return append(roles, role)
}
