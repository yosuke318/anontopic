package geoip

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

// testNetworks are the addresses the test databases know: one in Tokyo, one
// in the United States, both under the ASN given.
var testNetworks = []struct {
	cidr        string
	country     string
	subdivision string
	asn         uint32
}{
	{"203.0.113.0/24", "JP", "13", 2516},
	{"198.51.100.0/24", "US", "CA", 15169},
}

// testDatabase builds a database of edition holding testNetworks.
func testDatabase(t *testing.T, edition string) []byte {
	t.Helper()

	tree, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: edition, RecordSize: 24, IncludeReservedNetworks: true})
	if err != nil {
		t.Fatalf("mmdbwriter.New: %v", err)
	}
	for _, n := range testNetworks {
		_, network, err := net.ParseCIDR(n.cidr)
		if err != nil {
			t.Fatalf("parse %s: %v", n.cidr, err)
		}

		var record mmdbtype.Map
		if edition == editionCity {
			record = mmdbtype.Map{
				"country": mmdbtype.Map{"iso_code": mmdbtype.String(n.country)},
				"subdivisions": mmdbtype.Slice{
					mmdbtype.Map{"iso_code": mmdbtype.String(n.subdivision)},
				},
			}
		} else {
			record = mmdbtype.Map{"autonomous_system_number": mmdbtype.Uint32(n.asn)}
		}
		if err := tree.Insert(network, record); err != nil {
			t.Fatalf("insert %s: %v", n.cidr, err)
		}
	}

	var buf bytes.Buffer
	if _, err := tree.WriteTo(&buf); err != nil {
		t.Fatalf("write the database: %v", err)
	}
	return buf.Bytes()
}

// testArchive packs a database the way MaxMind ships it.
func testArchive(t *testing.T, edition string) []byte {
	t.Helper()

	data := testDatabase(t, edition)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string][]byte{
		edition + "_20261031/LICENSE.txt":          []byte("license"),
		edition + "_20261031/" + edition + ".mmdb": data,
	} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// fakeMaxMind serves the archives the way MaxMind does: it checks the
// credentials, then redirects to another host that serves the file.
type fakeMaxMind struct {
	server       *httptest.Server
	storage      *httptest.Server
	lastModified string
	downloads    atomic.Int32
}

func newFakeMaxMind(t *testing.T) *fakeMaxMind {
	t.Helper()

	f := &fakeMaxMind{lastModified: "Fri, 31 Oct 2026 00:00:00 GMT"}
	archives := map[string][]byte{
		editionCity: testArchive(t, editionCity),
		editionASN:  testArchive(t, editionASN),
	}

	f.storage = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); ok {
			t.Error("the credentials were sent to the storage host")
		}
		f.downloads.Add(1)
		_, _ = w.Write(archives[r.URL.Query().Get("edition")])
	}))
	t.Cleanup(f.storage.Close)

	mux := http.NewServeMux()
	mux.HandleFunc("/geoip/databases/{edition}/download", func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "1234" || pass != "key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		edition := r.PathValue("edition")
		if r.Method == http.MethodHead {
			w.Header().Set("Last-Modified", f.lastModified)
			return
		}
		// The storage sits on another host, which is what keeps the client
		// from carrying the credentials there.
		storage := strings.Replace(f.storage.URL, "127.0.0.1", "localhost", 1)
		http.Redirect(w, r, storage+"/file?edition="+edition, http.StatusFound)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)

	return f
}

func (f *fakeMaxMind) updater(db *Database, licenseKey string) *Updater {
	u := NewUpdater(db, "1234", licenseKey)
	u.baseURL = f.server.URL
	return u
}

func TestUpdateLoadsBothDatabasesAndLocates(t *testing.T) {
	f := newFakeMaxMind(t)
	db := &Database{}

	if got := db.Locate(netip.MustParseAddr("203.0.113.7")); got != (Location{}) {
		t.Fatalf("Locate before loading = %+v, want nothing", got)
	}

	if err := f.updater(db, "key").Update(context.Background()); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !db.Loaded() {
		t.Fatal("Loaded = false after Update")
	}

	cases := []struct {
		addr string
		want Location
	}{
		{"203.0.113.7", Location{Country: "JP", Prefecture: "JP-13", ASN: 2516}},
		{"::ffff:203.0.113.7", Location{Country: "JP", Prefecture: "JP-13", ASN: 2516}},
		// The prefecture is kept for addresses in Japan only.
		{"198.51.100.1", Location{Country: "US", ASN: 15169}},
		{"10.0.0.1", Location{}},
	}
	for _, c := range cases {
		if got := db.Locate(netip.MustParseAddr(c.addr)); got != c.want {
			t.Errorf("Locate(%s) = %+v, want %+v", c.addr, got, c.want)
		}
	}
}

func TestUpdateSkipsAReleaseAlreadyLoaded(t *testing.T) {
	f := newFakeMaxMind(t)
	u := f.updater(&Database{}, "key")

	for range 2 {
		if err := u.Update(context.Background()); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if n := f.downloads.Load(); n != 2 {
		t.Fatalf("downloads = %d, want one per edition", n)
	}

	f.lastModified = "Fri, 07 Nov 2026 00:00:00 GMT"
	if err := u.Update(context.Background()); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if n := f.downloads.Load(); n != 4 {
		t.Fatalf("downloads = %d, want a new release downloaded again", n)
	}
}

func TestUpdateKeepsTheDatabasesInUseWhenItFails(t *testing.T) {
	f := newFakeMaxMind(t)
	db := &Database{}

	if err := f.updater(db, "key").Update(context.Background()); err != nil {
		t.Fatalf("Update: %v", err)
	}
	f.lastModified = "Fri, 07 Nov 2026 00:00:00 GMT"
	if err := f.updater(db, "wrong").Update(context.Background()); err == nil {
		t.Fatal("Update with a wrong key succeeded")
	}

	if got := db.Locate(netip.MustParseAddr("203.0.113.7")); got.Prefecture != "JP-13" {
		t.Fatalf("Locate after a failed update = %+v, want the databases kept", got)
	}
}
