package geoip

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/oschwald/geoip2-golang/v2"
)

const (
	// DefaultBaseURL is where MaxMind serves the databases.
	DefaultBaseURL = "https://download.maxmind.com"

	// DefaultInterval is how often the updater checks for a new release.
	DefaultInterval = 7 * 24 * time.Hour

	// maxArchiveBytes bounds what one download may unpack to. The City
	// database is about 60 MB.
	maxArchiveBytes = 256 << 20

	editionCity = "GeoLite2-City"
	editionASN  = "GeoLite2-ASN"
)

// Updater downloads the databases into a Database and replaces them when a
// new release is out.
type Updater struct {
	db         *Database
	client     *http.Client
	baseURL    string
	accountID  string
	licenseKey string

	// lastModified holds the Last-Modified of the release loaded for each
	// edition. Only the goroutine running Update touches it.
	lastModified map[string]string
}

// NewUpdater builds an updater that downloads with the account ID and the
// license key of a MaxMind account.
func NewUpdater(db *Database, accountID, licenseKey string) *Updater {
	return &Updater{
		db:           db,
		client:       &http.Client{Timeout: 5 * time.Minute},
		baseURL:      DefaultBaseURL,
		accountID:    accountID,
		licenseKey:   licenseKey,
		lastModified: map[string]string{},
	}
}

// Run updates the databases now, then again every interval until ctx ends.
// A failed update is logged and leaves the databases in use as they were.
func (u *Updater) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := u.Update(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("update the GeoLite databases", slog.Any("error", err))
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Update loads the release of each edition that is newer than the one in
// use. Asking whether there is one does not count against the downloads
// MaxMind allows a day.
func (u *Updater) Update(ctx context.Context) error {
	var errs []error
	for _, edition := range []string{editionCity, editionASN} {
		if err := u.update(ctx, edition); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", edition, err))
		}
	}
	return errors.Join(errs...)
}

func (u *Updater) update(ctx context.Context, edition string) error {
	modified, err := u.releaseModified(ctx, edition)
	if err != nil {
		return err
	}
	if modified != "" && modified == u.lastModified[edition] {
		return nil
	}

	data, err := u.download(ctx, edition)
	if err != nil {
		return err
	}
	reader, err := geoip2.OpenBytes(data)
	if err != nil {
		return fmt.Errorf("open the database: %w", err)
	}

	switch edition {
	case editionCity:
		u.db.setCity(reader)
	case editionASN:
		u.db.setASN(reader)
	}
	u.lastModified[edition] = modified
	slog.Info("loaded a GeoLite database", slog.String("edition", edition), slog.String("last_modified", modified))
	return nil
}

func (u *Updater) url(edition string) string {
	return u.baseURL + "/geoip/databases/" + edition + "/download?suffix=tar.gz"
}

// releaseModified asks when the current release of edition was published.
func (u *Updater) releaseModified(ctx context.Context, edition string) (string, error) {
	res, err := u.request(ctx, http.MethodHead, edition)
	if err != nil {
		return "", err
	}
	_ = res.Body.Close()
	return res.Header.Get("Last-Modified"), nil
}

// download fetches the current release of edition and returns the database
// file inside the archive.
func (u *Updater) download(ctx context.Context, edition string) ([]byte, error) {
	res, err := u.request(ctx, http.MethodGet, edition)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	return extractDatabase(res.Body)
}

// request sends method to the download URL of edition. The client follows
// the redirect to the storage the file lives in, and does not carry the
// credentials to another host.
func (u *Updater) request(ctx context.Context, method, edition string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, u.url(edition), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(u.accountID, u.licenseKey)

	res, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s the release: %w", strings.ToLower(method), err)
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, fmt.Errorf("%s the release: status %d", strings.ToLower(method), res.StatusCode)
	}
	return res, nil
}

// extractDatabase reads the .mmdb file out of a gzipped tar archive.
func extractDatabase(r io.Reader) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("read the archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(io.LimitReader(gz, maxArchiveBytes))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the archive holds no .mmdb file")
		}
		if err != nil {
			return nil, fmt.Errorf("read the archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && strings.HasSuffix(hdr.Name, ".mmdb") {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", hdr.Name, err)
			}
			return data, nil
		}
	}
}
