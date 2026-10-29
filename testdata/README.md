# Fixture provenance

`dublin-upstream.json` comes from the upstream integration fixture
[want_8.8.8.8_one_path.json](https://github.com/insomniacslk/dublin-traceroute/blob/master/integ/test_data/want_8.8.8.8_one_path.json),
retrieved 2026-10-29. IP addresses were consistently replaced with RFC 5737
addresses. Shape, timing and response fields were preserved. This is an upstream
test fixture, not a new live capture. Its BSD-2-Clause notice is in DUBLIN-LICENSE.

Inline Go test cases are explicitly synthetic edge cases. They demonstrate parser
behavior, not live-device support. No production devices were contacted.
