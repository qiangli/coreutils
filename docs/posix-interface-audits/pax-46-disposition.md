# `pax:46` public-safe disposition

Review result: **still undetermined**. No Coreutils defect was reproduced, no
identity was retired, and no product code was changed.

This disposition uses only the public, suite-free Sprint 85 capability probe
from `vsc-pcts-harness-kit` revision
`5ebeb22830c2cc138fff8ed8aad4ed739e6f5174`. It does not read or reproduce a
licensed journal, assertion, operand, expected result, private archive, or
private path.

## Frozen and current product measurements

The probe was run against independently built multicall binaries exposed as
`pax`:

| product tree | revision | result |
|---|---|---|
| requested frozen Coreutils | `2ec1d618e6e84372ab9d2701d6da171ef453348d` | `first_observable=none` |
| current Coreutils | `34b665265d4d38995325c32a31ca6e92f8fc32c3` | `first_observable=none` |

Both measurements reported the same capability tuple:

```text
check.format_ustar=ok
check.format_cpio=ok
check.format_pax=ok
check.exthdr_longname=ok
check.opt_exthdr_name=ok
check.list_mode=ok
check.opt_listopt=ok
check.copy_mode=ok
check.error_exit=ok
posix_required_missing=none
environment_dependent=privilege_preserve:ok
first_observable=none
reopen_product_story=undetermined
```

The binary digests differed because these were separate development builds;
after removing build identity fields, the emitted semantic tuples were
byte-identical. No commit after the requested frozen revision changed
`cmds/pax` or `pkg/pax`.

## Reducer decision

The existing probe is already the smallest justified reducer available from
public evidence. It creates all payloads itself and checks nine
host-independent POSIX capabilities in dependency order. It found no red
boundary to reduce further.

No additional behavioral reducer was added. The retained public differential
names `pax:46` but contains no public operand or assertion mapping. Inventing a
case from that identity would guess at licensed suite content, while adding an
unrelated POSIX test would not establish ownership of this result.

## Ownership decision

The provider resolved to the Coreutils multicall in both measurements, so an
observed required-capability gap would have been product-owned. None was
observed. A green capability probe is not an exoneration: the identity may
depend on an unmeasured provider behavior, authority, fixture, or environment.
Consequently the only supported review result is **still undetermined**, not
fixed and not external/environment-owned.

Reopen a product patch only when public-safe evidence produces a non-`none`
`first_observable`, or when an authorized matched replay supplies a lawful
TP-to-capability mapping.
