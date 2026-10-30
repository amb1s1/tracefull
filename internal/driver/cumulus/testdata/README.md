# Cumulus 5.16 capture fixtures

These are scrubbed excerpts of authorized read-only captures from a real NVIDIA
switch running Cumulus Linux 5.16.0 and FRR 10.0.3, recorded on October 29, 2026
(America/New_York). Private originals remain outside Git. `provenance.json`
records commands and source stdout digests; destination substitutions are labelled.

Addresses and counters retain 12 original interface records: loopback, management,
two VRFs and eight route egress interfaces. LLDP retains nine neighbor records for
those selected interfaces. The omissions are excerpts, not evidence that other
interfaces or neighbors were absent. Routing and text outputs retain full records.

IP addresses map consistently to documentation ranges (link-local addresses retain
link-local scope), MACs use locally administered values, and hostnames, interface
descriptions, tenant names and serials are anonymized. Counts, RTT-independent
measurements, timestamps in the captured output, flags, interface indices and
JSON structure are retained. No measurement or successful command output was
invented. The FRR copyright notice is preserved.

`route.golden.json` is the reviewed expected parsed route result. Synthetic
malformed/missing-field cases are explicitly labelled in the tests. The unprivileged
FRR and LLDP failures were retained privately; production code must report such
failures instead of silently escalating privileges.


Additional management captures were recorded on October 30, 2026 (UTC); each new
provenance entry records its capture time. `addresses-detailed.json` retains the
same 12 interface names with typed VRF and slave table metadata. `frr-route-mgmt.json`
is an IPv6 local /128 via eth0; `frr-route-mgmt4.json` is the IPv4 lookup of
127.0.0.1 that returned an unreachable default route, not a successful forwarding
path. CIDR keys and values use the existing consistent address mapping. The
separate kernel lookups remain private corroboration; the driver uses FRR only.
