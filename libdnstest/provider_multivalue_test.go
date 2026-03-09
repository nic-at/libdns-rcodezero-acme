package libdnstest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/libdns/libdns"

	rcodezero "github.com/nic-at/libdns-rcodezero-acme"
)

func randHexFQDN(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func waitForTXTAtName(t *testing.T, p *rcodezero.Provider, zone string, name string, want map[string]bool, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		recs, err := p.GetRecords(ctx, zone)
		cancel()

		if err == nil {
			seen := map[string]bool{}
			for _, r := range recs {
				v, ok := r.(libdns.TXT)
				if !ok {
					continue
				}
				if strings.EqualFold(v.Name, name) {
					seen[v.Text] = true
				}
			}

			okAll := true
			for val, shouldExist := range want {
				if shouldExist && !seen[val] {
					okAll = false
					break
				}
				if !shouldExist && seen[val] {
					okAll = false
					break
				}
			}
			if okAll {
				return
			}
		}

		time.Sleep(250 * time.Millisecond)
	}

	// Debug dump on failure
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	recs, _ := p.GetRecords(ctx, zone)
	t.Fatalf("timeout waiting for name=%q state=%v; got=%v", name, want, recs)
}

func cleanupByPrefixAtNames(t *testing.T, p *rcodezero.Provider, zone string, names []string, prefixes []string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	recs, err := p.GetRecords(ctx, zone)
	if err != nil {
		return
	}

	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[strings.ToLower(n)] = true
	}

	var dels []libdns.Record
	for _, r := range recs {
		v, ok := r.(libdns.TXT)
		if !ok {
			continue
		}
		if !nameSet[strings.ToLower(v.Name)] {
			continue
		}
		for _, pref := range prefixes {
			if strings.HasPrefix(v.Text, pref) {
				dels = append(dels, libdns.TXT{Name: v.Name, Text: v.Text, TTL: v.TTL})
				break
			}
		}
	}

	if len(dels) > 0 {
		_, _ = p.DeleteRecords(context.Background(), zone, dels)
	}
}

func logTXTForNames(t *testing.T, p *rcodezero.Provider, zone string, names []string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	recs, err := p.GetRecords(ctx, zone)
	if err != nil {
		t.Logf("GetRecords failed while logging: %v", err)
		return
	}

	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[strings.ToLower(n)] = true
	}

	foundAny := false
	for _, r := range recs {
		v, ok := r.(libdns.TXT)
		if !ok {
			continue
		}
		if !nameSet[strings.ToLower(v.Name)] {
			continue
		}
		foundAny = true
		t.Logf("DNS TXT present: zone=%q name=%q ttl=%s value=%q", zone, v.Name, v.TTL, v.Text)
	}
	if !foundAny {
		t.Logf("DNS TXT present: none for zone=%q names=%v", zone, names)
	}
}

// Tests concurrent-style behavior using DIFFERENT FQDNs (no rrset collision):
// _acme-challenge.servera.<zone> and _acme-challenge.serverb.<zone>
func TestACME_MultipleFQDNNames_NoCollision(t *testing.T) {
	cfg, ok := FromEnv()
	if !ok {
		t.Skip("set LIBDNSTEST_ZONE and LIBDNSTEST_API_TOKEN to run integration tests")
	}

	p := &rcodezero.Provider{
		APIToken: cfg.APIToken,
		BaseURL:  cfg.BaseURL,
	}

	nameA := "_acme-challenge.servera"
	nameB := "_acme-challenge.serverb"

	// Always try to clean up leftovers from prior failed runs
	defer cleanupByPrefixAtNames(t, p, cfg.Zone, []string{nameA, nameB}, []string{"libdnstest-A-", "libdnstest-B-"})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tokenA := "libdnstest-A-" + randHexFQDN(10)
	tokenB := "libdnstest-B-" + randHexFQDN(10)

	txtA := libdns.TXT{Name: nameA, Text: tokenA, TTL: 60 * time.Second}
	txtB := libdns.TXT{Name: nameB, Text: tokenB, TTL: 60 * time.Second}

	t.Logf("Zone=%q", cfg.Zone)
	t.Logf("A fqdn=%q value=%q", nameA, tokenA)
	t.Logf("B fqdn=%q value=%q", nameB, tokenB)

	// Present both (simulating two different hosts)
	t.Logf("Append A: name=%q value=%q", txtA.Name, txtA.Text)
	if _, err := p.AppendRecords(ctx, cfg.Zone, []libdns.Record{txtA}); err != nil {
		t.Fatalf("AppendRecords(A) failed: %v", err)
	}
	logTXTForNames(t, p, cfg.Zone, []string{nameA, nameB})

	t.Logf("Append B: name=%q value=%q", txtB.Name, txtB.Text)
	if _, err := p.AppendRecords(ctx, cfg.Zone, []libdns.Record{txtB}); err != nil {
		t.Fatalf("AppendRecords(B) failed: %v", err)
	}
	logTXTForNames(t, p, cfg.Zone, []string{nameA, nameB})

	// Verify each exists ONLY at its own name
	waitForTXTAtName(t, p, cfg.Zone, nameA, map[string]bool{tokenA: true}, 30*time.Second)
	waitForTXTAtName(t, p, cfg.Zone, nameB, map[string]bool{tokenB: true}, 30*time.Second)

	// Cleanup A should not affect B
	t.Logf("Delete A: name=%q value=%q", txtA.Name, txtA.Text)
	if _, err := p.DeleteRecords(ctx, cfg.Zone, []libdns.Record{txtA}); err != nil {
		t.Fatalf("DeleteRecords(A) failed: %v", err)
	}
	logTXTForNames(t, p, cfg.Zone, []string{nameA, nameB})

	waitForTXTAtName(t, p, cfg.Zone, nameA, map[string]bool{tokenA: false}, 30*time.Second)
	waitForTXTAtName(t, p, cfg.Zone, nameB, map[string]bool{tokenB: true}, 30*time.Second)

	// Cleanup B
	t.Logf("Delete B: name=%q value=%q", txtB.Name, txtB.Text)
	if _, err := p.DeleteRecords(ctx, cfg.Zone, []libdns.Record{txtB}); err != nil {
		t.Fatalf("DeleteRecords(B) failed: %v", err)
	}
	logTXTForNames(t, p, cfg.Zone, []string{nameA, nameB})

	waitForTXTAtName(t, p, cfg.Zone, nameB, map[string]bool{tokenB: false}, 30*time.Second)
}

