# MikroTik RouterOS 7.24.5 captures

Authorized read-only captures recorded November 1, 2026 on a CCR2216. The manifest
records source commands, original stdout SHA-256 and UTC epoch timestamps. All
16 stdout/stderr hashes for these eight fixtures verified. Strict host keys,
noninteractive authentication, 25-second deadlines and 1 MiB per stream applied.
The noninteractive reads did not trigger the password-change prompt. No password,
configuration or probing action occurred. Private originals/maps remain outside Git.

Addresses, MACs, neighbor identities, comments, serials and named WireGuard
interfaces are scrubbed consistently. Address family, prefix length and immediate
next-hop interface suffixes are preserved. Anonymized addresses do not reconstruct
topology. The optics text is an excerpt: the EEPROM field is omitted because it
also encodes the serial. Line endings/padding are normalized. Other fixtures are
complete; synthetic failure/prefix variants are clearly labelled in tests.

The reviewed JSON commands use RouterOS `:serialize` with no string conversion
and exact read-only expressions. Interface detail and statistics are consecutive
reads combined into one array. IDs/names must agree; this is not an atomic sample.
Missing rx-error/rx-drop fields remain unknown, not zero. TX drops are queue drops.

Resource and optics observations use explicit-unit text. Exploratory JSON captures
converted uptime into a date-like value and optical power into scaled integers;
those unvalidated encodings are not used. CPU percentage has no established
sampling interval. The optical capture reports Celsius and dBm, without a lane ID.

The IPv4 static default has one immediate gateway; the IPv6 BGP default has two
records, only one explicitly active. Missing active flags remain unknown. Main
is normalized to default. Lookup expressions request covering prefixes in main;
only the most-specific prefix is normalized, with no fallback from unknown state.
Hardware installation stays unknown. Neighbor rows include physical/bridge
interfaces; both are retained with their source reference, without topology claims.

Syntax reference: [MikroTik scripting](https://help.mikrotik.com/docs/spaces/ROS/pages/47579229/Scripting).
