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
