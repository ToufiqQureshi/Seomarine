// Package geo resolves public IP addresses with an embedded, offline country database.
package geo

import (
	_ "embed"
	"fmt"
	"net"
	"net/netip"

	"github.com/oschwald/maxminddb-golang"
)

//go:embed dbip-country-lite-2026-10.mmdb
var database []byte

// Lookup is safe for concurrent use after construction.
type Lookup struct{ reader *maxminddb.Reader }

// New opens the embedded country database for lookups.
func New() (*Lookup, error) {
	reader, err := maxminddb.FromBytes(database)
	if err != nil {
		return nil, fmt.Errorf("open embedded country database: %w", err)
	}
	return &Lookup{reader: reader}, nil
}

// Country returns an ISO 3166-1 alpha-2 code, or an empty string when the
// address is invalid, private, or absent from the database.
func (l *Lookup) Country(raw string) (string, error) {
	ip, err := netip.ParseAddr(raw)
	if err != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return "", nil
	}
	var record struct {
		Country struct {
			Code string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}
	if err := l.reader.Lookup(net.IP(ip.Unmap().AsSlice()), &record); err != nil {
		return "", fmt.Errorf("look up country: %w", err)
	}
	if len(record.Country.Code) != 2 {
		return "", nil
	}
	return record.Country.Code, nil
}
