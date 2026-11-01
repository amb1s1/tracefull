# Cisco NX-OS 10.5(6) captures

Authorized read-only captures recorded November 1, 2026. `provenance.json`
records each source command, UTC epoch timestamps and original stdout SHA-256.
Private originals and identifying maps remain outside Git. All 20 stream hashes
for these ten fixtures were verified. Strict host keys, noninteractive SSH,
25-second deadlines and 1 MiB per stream applied; no probes or changes ran.

Addresses, MACs, hostnames, descriptions and serials are scrubbed. Address family
and prefix length are retained; anonymized addresses do not reconstruct topology.
The LLDP fixture is an excerpt of the first two complete rows, labelled in the
manifest. All other fixtures are complete. Copyright notices are preserved.

Default-context brief interface JSON returned zero bytes, while explicit all-VRF
brief reads returned management addresses with membership. Empty bytes must fail
parsing; they are not a valid empty lookup. The IPv4 route capture contains an
explicit default IPv4 table with no prefix. The IPv6 static default selects a
next hop in management without a resolved interface; its RIB selection does not
establish hardware installation or default-context interface ownership.

The DOM capture is single-lane with no lane identifier. No optic-health or
SVI/port-channel member inference is supported. Counters retain unknown omitted
fields; configured bandwidth is not line speed. Synthetic malformed/type/state
variations are tests, not additional real-device fixtures.
