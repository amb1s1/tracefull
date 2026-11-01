# Cisco IOS-XE 17.12.03 captures

Authorized read-only captures from a Catalyst C9200L recorded November 1, 2026.
`provenance.json` records original stdout SHA-256 and UTC epoch capture times.
All 24 source stream hashes were verified. Strict host keys, noninteractive SSH,
25-second deadlines and 1 MiB per stream applied. No probes or changes ran.

Private originals and identifying maps remain outside Git. Addresses, MACs,
hostnames, descriptions and serials are scrubbed. Families/prefix lengths and
interface names are preserved; anonymized addresses do not reconstruct topology.
CRLF and trailing padding are normalized; tests verify CRLF/LF equivalence.
Copyright/license notices are retained. CPU is a labelled excerpt of the complete
summary and table heading; process rows are omitted. Other files are complete.

The source has no assigned IPv4 brief addresses and returns a missing IPv4
route. IPv6 captures include a static gateway-only default and a connected
receive route. No explicit best-path/FIB selection or resolved interface is
inferred. Address membership and prefix lengths omitted by brief reads stay
unknown. The detailed IPv6 interface capture is research evidence; the current
planner uses the brief format and does not infer its missing metadata.

Counters distinguish cumulative input-queue drops from total output drops and
use the negotiated speed, not configurable BW. SVI speed and MTU framing remain
unknown. DOM is captured on TenGigabitEthernet1/1/1, with lane N/A; thresholds do
not become optic-health verdicts. CPU total is five-second utilization including
interrupts, with separate one/five-minute percentages. Year/week uptime remains
raw because its calendar/rounding semantics are not validated. Synthetic tests
are labelled and do not extend platform/runtime claims.
