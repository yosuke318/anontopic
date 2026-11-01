// Package geoip tells the country, the prefecture and the network operator
// (ASN) an IP address is registered to, from the GeoLite City and GeoLite ASN
// databases of MaxMind.
//
// The server downloads the databases itself when it starts and checks for a
// new release once a week, so that a release replaces the one in use well
// within the 30 days the GeoLite license allows. A server that cannot
// download them keeps running and locates nothing. The reasoning is in
// docs/adr/0045-locate-the-prefecture-of-a-session-with-geolite-downloaded-by-the-server.md.
//
// The location is recorded for statistics and moderation only, and is never
// used to pick a partner.
package geoip

import (
	"net/netip"
	"sync/atomic"

	"github.com/oschwald/geoip2-golang/v2"
)

// Location is what the databases know about one address. A field is empty
// when the databases do not know it.
type Location struct {
	// Country is the ISO 3166-1 alpha-2 code of the country, such as "JP".
	Country string
	// Prefecture is the ISO 3166-2 code of the prefecture, such as "JP-13".
	// It is set for an address in Japan only.
	Prefecture string
	// ASN is the number of the autonomous system the address belongs to.
	ASN uint32
}

// Database answers lookups from the databases loaded last. It answers an
// empty Location until a database is loaded, and is safe for concurrent use
// while a new release replaces the one in use.
type Database struct {
	city atomic.Pointer[geoip2.Reader]
	asn  atomic.Pointer[geoip2.Reader]
}

// Locate looks addr up in both databases. An address neither knows, such as
// a private one, answers an empty Location.
func (d *Database) Locate(addr netip.Addr) Location {
	addr = addr.Unmap()
	var loc Location

	if city := d.city.Load(); city != nil {
		if rec, err := city.City(addr); err == nil {
			loc.Country = rec.Country.ISOCode
			if loc.Country == "JP" && len(rec.Subdivisions) > 0 && rec.Subdivisions[0].ISOCode != "" {
				loc.Prefecture = "JP-" + rec.Subdivisions[0].ISOCode
			}
		}
	}

	if asn := d.asn.Load(); asn != nil {
		if rec, err := asn.ASN(addr); err == nil {
			loc.ASN = uint32(rec.AutonomousSystemNumber)
		}
	}
	return loc
}

// Loaded reports whether both databases are loaded.
func (d *Database) Loaded() bool {
	return d.city.Load() != nil && d.asn.Load() != nil
}

// The readers hold the whole database in memory, so a reader that is
// replaced while a lookup reads it stays valid until the garbage collector
// finds it unused. Nothing has to be closed.
func (d *Database) setCity(r *geoip2.Reader) { d.city.Store(r) }

func (d *Database) setASN(r *geoip2.Reader) { d.asn.Store(r) }
