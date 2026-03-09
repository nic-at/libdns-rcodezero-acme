# libdns-rcodezeroacme

ACME-only DNS provider for **RcodeZero** using the **libdns** interfaces.

This provider is intentionally minimal and only supports the DNS operations required for **ACME DNS-01 challenges**.

It uses the dedicated RcodeZero ACME endpoint and does **not** implement full DNS management.

---

## Features

* Implements libdns interfaces required for ACME DNS-01
* Creates and removes `_acme-challenge` TXT records
* Uses the dedicated RcodeZero ACME API endpoint
* Designed for predictable ACME automation
* Integration test suite included

---

## API Endpoint Used

The provider uses the ACME endpoint:

* `PATCH /api/v1/acme/zones/{zone}/rrsets`
* `GET   /api/v1/acme/zones/{zone}/rrsets`

Official OpenAPI specification:

[https://my.rcodezero.at/openapi/](https://my.rcodezero.at/openapi/)

---

## Authentication

Authentication is performed using an HTTP Bearer token:

```
Authorization: Bearer <TOKEN>
```

The token must have permission to manage ACME challenge records for the zone.

---

## Supported Records

This provider is strictly ACME-only.

Supported:

* Record type: `TXT`
* Name must start with `_acme-challenge`

Accepted names:

* `_acme-challenge`
* `_acme-challenge.servera`
* `_acme-challenge.subdomain`

Rejected immediately:

* `A`, `AAAA`, `CNAME`, `MX`, etc.
* `www`
* `mail`
* Any non `_acme-challenge` name

Attempts to manage unsupported records will fail fast.

---

## ACME Behavior and Concurrency

### Important: Behavior Depends on FQDN

### Safe Scenario (No Collision)

If different hosts request certificates:

* `_acme-challenge.servera.example.com`
* `_acme-challenge.serverb.example.com`

These are **different rrsets**.

No collision occurs.
Concurrent validations are safe.

This is the most common real-world scenario.

---

### Collision Scenario (Same FQDN)

If multiple issuers request validation for:

* `example.com`
* `*.example.com`
* the same hostname simultaneously

They will all use:

```
_acme-challenge.example.com
```

The RcodeZero ACME endpoint does **not support safe per-value deletion**.

Current endpoint behavior:

* `update` writes the TXT rrset (single value semantics)
* `delete` removes the whole rrset

Therefore:

* Multiple concurrent validations for the same `_acme-challenge.<name>` are **not safe**
* External locking or a single issuer is required

This matches the behavior of:

* lego’s rcodezero provider
* cert-manager webhook implementation

---

## Limitations

### No Full DNS Management

This provider does NOT support:

* A / AAAA / CNAME / MX records
* Arbitrary zone management
* Non-ACME DNS operations

For full DNS record management, use the RcodeZero v2 API instead.

---

### Concurrent Validation on Same Name

If multiple ACME clients validate the same hostname simultaneously:

* One may overwrite the other's TXT record
* Cleanup may remove records still required by another client

Recommended approach:

* Use a single ACME issuer per domain
* Or ensure shared locking/storage between HA instances

---

## Usage (Go Example)

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/libdns/libdns"
	rcodezero "github.com/nic-at/libdns-rcodezero-acme"
)

func main() {
	provider := &rcodezero.Provider{
		APIToken: "your-token-here",
		BaseURL:  "https://my.rcodezero.at", // optional
	}

	ctx := context.Background()
	zone := "example.com."

	record := libdns.TXT{
		Name: "_acme-challenge",
		Text: "challenge-token-value",
		TTL:  60 * time.Second,
	}

	// Present challenge
	_, err := provider.AppendRecords(ctx, zone, []libdns.Record{record})
	if err != nil {
		panic(err)
	}

	fmt.Println("TXT challenge record created")

	// Cleanup
	_, err = provider.DeleteRecords(ctx, zone, []libdns.Record{record})
	if err != nil {
		panic(err)
	}

	fmt.Println("TXT challenge record removed")
}
```

---

## Running Tests

### Compile & Static Checks

```
go test ./...
go vet ./...
```

---

## Integration Tests

Integration tests require a real zone and API token.

### Required Environment Variables

```
export LIBDNSTEST_ZONE="example.com."
export LIBDNSTEST_API_TOKEN="YOUR_TOKEN"
export LIBDNSTEST_BASE_URL="https://my.rcodezero.at"
```

### Run Integration Tests

```
go test ./libdnstest -v
```

The integration suite verifies:

* TXT record creation under `_acme-challenge`
* Record cleanup
* Multiple FQDN validation without collision
* Correct behavior under ACME endpoint semantics

---

## CI Recommendation

Minimal GitHub Actions workflow:

```yaml
name: Go

on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go test ./...
      - run: go vet ./...
```

---

## Disclaimer

This provider is intentionally ACME-only and minimal.
