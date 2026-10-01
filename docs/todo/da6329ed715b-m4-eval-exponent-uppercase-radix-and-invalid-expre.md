---
id: da6329ed715b
kind: bug
title: 'm4 eval: exponent, uppercase radix, and invalid expression exit'
seq: 169
status: doing
priority: p0
created: 2026-10-01T11:24:09.145091Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

C focused TP 47, 48, 51, 61. Implement signed 32-bit exponentiation extension expected by the licensed POSIX08 profile, uppercase alphabetic radix digits, and nonzero exit for invalid eval expressions. Add focused behavior tests and rerun licensed m4 after the full baseline. Cite Open Group public m4 spec for required behavior; preserve VSC details privately.
