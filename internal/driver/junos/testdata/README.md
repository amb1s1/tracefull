# Junos 24.4R2-S5.3 captures

Authorized read-only real-device captures recorded October 29, 2026 (America/New_York;
UTC October 30). Source commands, UTC epoch times and stdout digests are in
`provenance.json`. Raw originals remain private outside Git.

Addresses, MACs, hostnames, descriptions, serials and AS identifiers are scrubbed.
Address-family and prefix lengths are preserved; address pairs retain their low
bit for the captured point-to-point prefixes. These anonymized addresses do not
reconstruct the original topology. Interface names, counters, units and diagnostic
measurements remain from the real output. Files marked excerpt omit records or
fields; absence from an excerpt does not imply absence from the device.

Empty route/optics responses are retained as observations, never manufactured
success cases. Command exit zero does not establish a usable measurement. Tests
label synthetic malformed/missing-field variants separately. Copyright notices
in source version output are retained.

`route4-isis-detail.json` is a complete detail lookup captured October 30, 2026,
from the same target, with Junos 24.4R2-S5.3 rechecked privately. It preserves two
next hops and the selection marker present on only one. Addresses and the local
AS are mapped consistently within this fixture, separately from older captures.
A bounded protocol-table read supplied the actual destination for the lookup;
summary/table/version originals remain private. This RIB observation does not
establish hardware installation or simultaneous multipath forwarding. No probes
or configuration changes ran. Source time and digest are in `provenance.json`.
