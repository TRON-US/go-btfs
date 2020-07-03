package corehttp

import (
	"net/http/httptest"
	"testing"

	config "github.com/TRON-US/go-btfs-config"
)

func TestToSubdomainURL(t *testing.T) {
	r := httptest.NewRequest("GET", "http://request-stub.example.com", nil)
	for _, test := range []struct {
		// in:
		hostname string
		path     string
		// out:
		url string
		ok  bool
	}{
		// DNSLink
		{"localhost", "/btns/dnslink.io", "http://dnslink.io.btns.localhost/", true},
		// Hostname with port
		{"localhost:8080", "/btns/dnslink.io", "http://dnslink.io.btns.localhost:8080/", true},
		// CIDv0 → CIDv1base32
		{"localhost", "/btfs/QmbCMUZw6JFeZ7Wp9jkzbye3Fzp2GGcPgC3nmeUjfVF87n", "http://bafybeif7a7gdklt6hodwdrmwmxnhksctcuav6lfxlcyfz4khzl3qfmvcgu.btfs.localhost/", true},
		// PeerID as CIDv1 needs to have libp2p-key multicodec
		{"localhost", "/btns/QmY3hE8xgFCjGcz6PHgnvJz5HZi1BaKRfPkn1ghZUcYMjD", "http://bafzbeieqhtl2l3mrszjnhv6hf2iloiitsx7mexiolcnywnbcrzkqxwslja.btns.localhost/", true},
		{"localhost", "/btns/bafybeickencdqw37dpz3ha36ewrh4undfjt2do52chtcky4rxkj447qhdm", "http://bafzbeickencdqw37dpz3ha36ewrh4undfjt2do52chtcky4rxkj447qhdm.btns.localhost/", true},
		// PeerID: ed25519+identity multihash
		{"localhost", "/btns/12D3KooWFB51PRY9BxcXSH6khFXw1BZeszeLDy7C8GciskqCTZn5", "http://bafzaajaiaejcat4yhiwnr2qz73mtu6vrnj2krxlpfoa3wo2pllfi37quorgwh2jw.btns.localhost/", true},
	} {
		url, ok := toSubdomainURL(test.hostname, test.path, r)
		if ok != test.ok || url != test.url {
			t.Errorf("(%s, %s) returned (%s, %t), expected (%s, %t)", test.hostname, test.path, url, ok, test.url, ok)
		}
	}
}

func TestHasPrefix(t *testing.T) {
	for _, test := range []struct {
		prefixes []string
		path     string
		out      bool
	}{
		{[]string{"/btfs"}, "/btfs/cid", true},
		{[]string{"/btfs/"}, "/btfs/cid", true},
		{[]string{"/version/"}, "/version", true},
		{[]string{"/version"}, "/version", true},
	} {
		out := hasPrefix(test.path, test.prefixes...)
		if out != test.out {
			t.Errorf("(%+v, %s) returned '%t', expected '%t'", test.prefixes, test.path, out, test.out)
		}
	}
}

func TestPortStripping(t *testing.T) {
	for _, test := range []struct {
		in  string
		out string
	}{
		{"localhost:8080", "localhost"},
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.btfs.localhost:8080", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.btfs.localhost"},
		{"example.com:443", "example.com"},
		{"example.com", "example.com"},
		{"foo-dweb.btfs.pvt.k12.ma.us:8080", "foo-dweb.btfs.pvt.k12.ma.us"},
		{"localhost", "localhost"},
		{"[::1]:8080", "::1"},
	} {
		out := stripPort(test.in)
		if out != test.out {
			t.Errorf("(%s): returned '%s', expected '%s'", test.in, out, test.out)
		}
	}

}

func TestKnownSubdomainDetails(t *testing.T) {
	gwSpec := config.GatewaySpec{
		UseSubdomains: true,
	}
	knownGateways := map[string]config.GatewaySpec{
		"localhost":               gwSpec,
		"dweb.link":               gwSpec,
		"dweb.btfs.pvt.k12.ma.us": gwSpec, // note the sneaky ".btfs." ;-)
	}

	for _, test := range []struct {
		// in:
		hostHeader string
		// out:
		hostname string
		ns       string
		rootID   string
		ok       bool
	}{
		// no subdomain
		{"127.0.0.1:8080", "", "", "", false},
		{"[::1]:8080", "", "", "", false},
		{"hey.look.example.com", "", "", "", false},
		{"dweb.link", "", "", "", false},
		// malformed Host header
		{".....dweb.link", "", "", "", false},
		{"link", "", "", "", false},
		{"8080:dweb.link", "", "", "", false},
		{" ", "", "", "", false},
		{"", "", "", "", false},
		// unknown gateway host
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.btfs.unknown.example.com", "", "", "", false},
		// cid in subdomain, known gateway
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.btfs.localhost:8080", "localhost:8080", "btfs", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am", true},
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.btfs.dweb.link", "dweb.link", "btfs", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am", true},
		// capture everything before .btfs.
		{"foo.bar.boo-buzz.btfs.dweb.link", "dweb.link", "btfs", "foo.bar.boo-buzz", true},
		// btns
		{"bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju.btns.localhost:8080", "localhost:8080", "btns", "bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju", true},
		{"bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju.btns.dweb.link", "dweb.link", "btns", "bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju", true},
		// edge case check: public gateway under long TLD (see: https://publicsuffix.org)
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.btfs.dweb.btfs.pvt.k12.ma.us", "dweb.btfs.pvt.k12.ma.us", "btfs", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am", true},
		{"bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju.btns.dweb.btfs.pvt.k12.ma.us", "dweb.btfs.pvt.k12.ma.us", "btns", "bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju", true},
		// dnslink in subdomain
		{"en.wikipedia-on-btfs.org.btns.localhost:8080", "localhost:8080", "btns", "en.wikipedia-on-btfs.org", true},
		{"en.wikipedia-on-btfs.org.btns.localhost", "localhost", "btns", "en.wikipedia-on-btfs.org", true},
		{"dist.btfs.io.btns.localhost:8080", "localhost:8080", "btns", "dist.btfs.io", true},
		{"en.wikipedia-on-btfs.org.btns.dweb.link", "dweb.link", "btns", "en.wikipedia-on-btfs.org", true},
		// edge case check: public gateway under long TLD (see: https://publicsuffix.org)
		{"foo.dweb.btfs.pvt.k12.ma.us", "", "", "", false},
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.btfs.dweb.btfs.pvt.k12.ma.us", "dweb.btfs.pvt.k12.ma.us", "btfs", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am", true},
		{"bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju.btns.dweb.btfs.pvt.k12.ma.us", "dweb.btfs.pvt.k12.ma.us", "btns", "bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju", true},
		// other namespaces
		{"api.localhost", "", "", "", false},
		{"peerid.p2p.localhost", "localhost", "p2p", "peerid", true},
	} {
		gw, hostname, ns, rootID, ok := knownSubdomainDetails(test.hostHeader, knownGateways)
		if ok != test.ok {
			t.Errorf("knownSubdomainDetails(%s): ok is %t, expected %t", test.hostHeader, ok, test.ok)
		}
		if rootID != test.rootID {
			t.Errorf("knownSubdomainDetails(%s): rootID is '%s', expected '%s'", test.hostHeader, rootID, test.rootID)
		}
		if ns != test.ns {
			t.Errorf("knownSubdomainDetails(%s): ns is '%s', expected '%s'", test.hostHeader, ns, test.ns)
		}
		if hostname != test.hostname {
			t.Errorf("knownSubdomainDetails(%s): hostname is '%s', expected '%s'", test.hostHeader, hostname, test.hostname)
		}
		if ok && gw.UseSubdomains != gwSpec.UseSubdomains {
			t.Errorf("knownSubdomainDetails(%s): gw is  %+v, expected %+v", test.hostHeader, gw, gwSpec)
		}
	}

}
