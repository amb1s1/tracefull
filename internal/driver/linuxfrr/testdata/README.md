# Linux/FRR fixture reuse

The user selected the same NVIDIA Cumulus switch as the Linux/FRR validation
host. Cumulus Linux 5.16.0 runs Linux 6.1 and FRR 10.0.3. The real scrubbed inputs
are shared from [Cumulus testdata](../../cumulus/testdata/README.md), including
`frr-version.txt`, `frr-route.json`, iproute2 JSON, procfs and lldpd output.
Their original source hashes and commands remain in that directory's provenance.
No additional device access or duplicate/fabricated captures were needed.

`frr-version.golden.json` pins the parsed FRR identity. Tests also replay all shared
formats and the existing reviewed route golden. Synthetic non-Cumulus OS labels
exercise branding independence only; they do not establish runtime validation on
Ubuntu, Debian, containers or other FRR builds. Missing executables, permissions
and unrecognized output remain explicit failures, without changing the policy.
