# Cisco IOS XR 25.4.2 captures

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

Text fixtures normalize CRLF to LF and remove trailing padding/blank lines for
repository diffs. Leading alignment and internal line positions are preserved.
Tests replay both LF and CRLF forms; original bytes remain in private captures.
