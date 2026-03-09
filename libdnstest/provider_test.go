package libdnstest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/libdns/libdns"

	rcodezero "github.com/nic-at/libdns-rcodezeroacme"
)

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func TestPresentCleanup_ACME(t *testing.T) {
	cfg, ok := FromEnv()
	if !ok {
		t.Skip("set LIBDNSTEST_ZONE and LIBDNSTEST_API_TOKEN to run integration tests")
	}

	p := &rcodezero.Provider{
		APIToken: cfg.APIToken,
		BaseURL:  cfg.BaseURL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	txt := libdns.TXT{
		Name: "_acme-challenge",
		Text: "libdnstest-" + randHex(12),
		TTL:  60 * time.Second,
	}

	// Present
	if _, err := p.AppendRecords(ctx, cfg.Zone, []libdns.Record{txt}); err != nil {
		t.Fatalf("AppendRecords failed: %v", err)
	}

	// Optional: if you implement GetRecords, verify it appears (with retries for eventual consistency)
	recs, err := p.GetRecords(ctx, cfg.Zone)
	if err == nil {
		found := false
		for _, r := range recs {
			v, ok := r.(libdns.TXT)
			if ok && v.Name == "_acme-challenge" && v.Text == txt.Text {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("record not found after append (GetRecords returned ok)")
		}
	}

	// Cleanup
	if _, err := p.DeleteRecords(ctx, cfg.Zone, []libdns.Record{txt}); err != nil {
		t.Fatalf("DeleteRecords failed: %v", err)
	}
}

