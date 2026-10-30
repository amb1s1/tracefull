# Linux/FRR fixture reuse

The user selected the same NVIDIA Cumulus switch as the Linux/FRR validation
host. Cumulus Linux 5.16.0 runs Linux 6.1 and FRR 10.0.3. The real scrubbed inputs
are shared from [Cumulus testdata](../../cumulus/testdata/README.md), including
`frr-version.txt`, `frr-route.json`, iproute2 JSON, procfs and lldpd output.
Their original source hashes and commands remain in that directory's provenance.
The management-mode extension shares the additional detailed-address and FRR
IPv4/IPv6 management captures, with their October 30 UTC provenance. No duplicate
or fabricated captures were needed.

`frr-version.golden.json` pins the parsed FRR identity. Tests also replay all shared
formats and the existing reviewed route golden. Synthetic non-Cumulus OS labels
exercise branding independence only; they do not establish runtime validation on
Ubuntu, Debian, containers or other FRR builds. Missing executables, permissions
and unrecognized output remain explicit failures, without changing the policy.

The optics parser also reuses the captured OSFP revision 4.0 module output.
CLI replay covers the Linux/FRR profile; this does not validate optics on other
module formats or Linux distributions.
