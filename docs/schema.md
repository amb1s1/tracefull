# Report schema version 1

`tracefull schema` prints [the embedded schema](../internal/schema/report-v1.json).
It is the implemented v0.1 contract; the broader design specification includes
future device/probing fields that are not part of this version's output.

RTT values (`best`, `avg`, `worst`, `stdev`) are milliseconds. PMTU is bytes.
`loss_pct` is percent. Sample counts and UDP ports are integers. Unknown numeric,
boolean and time values are null. Unknown string identities are empty strings.
Lists/maps are empty where no entries are known; inventory `addresses` may be
null when omitted. Counts are not reconstructed from rounded MTR percentages,
except the exact 0%/100% cases. Zero-sample rows have null loss and RTT values.

`started_at` and `ended_at` are source measurement times, normalized to UTC only
when a timezone or Unix timestamp establishes one. Unknown or local-only MTR time
strings remain notes. `dns_observed_at` records the later enrichment run. The
report never substitutes processing time for the original measurement time.

`hops` are sorted by TTL and addresses lexically. `paths` retain Dublin flow IDs,
UDP source/destination ports, per-flow times and their own hop statistics. A
combined view of multiple flows does not average their RTT/loss. Findings follow
the published rule order and retain flow attribution, evidence and limitations.
JSON object keys have stable encoding; offline repeated input/config yields
identical output. DNS-enabled output is a new observation and can differ.

`source` stores the input name, byte count and SHA-256 digest. Hop/finding evidence
uses `line:N` (one-based) or `json:/pointer` (JSON Pointer). These refer to the saved
input with that digest. Keep the original file; v0.1 does not copy it to an evidence
directory. Raw unparsed extensions remain available there, with report notes.

`matches` is keyed by hop IP; each entry includes method, selected device and
candidate names. Ambiguous matches have an empty selected device. Inventory
metadata is in `devices`; that list is not collected device evidence.

Later per-fact collection must introduce explicit raw command references.
`owner_sweep`, command results and device facts are not emitted as empty claims
of completed work. Breaking changes to this contract require a schema-version bump.
