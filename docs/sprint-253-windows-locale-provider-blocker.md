# Sprint 253 Windows locale provider feasibility

Story: #711 (`f3f0e3ef7aae`)

## Result

The requested provider cannot truthfully satisfy the current contract from
Windows host data alone. No provider code is submitted. In particular, the
rejected `HostLocaleProvider`-only candidate was removed rather than leaving an
uninstalled interface that could make `locale -a` claim unserviceable names.

`EnumSystemLocalesEx` and `GetLocaleInfoEx` are suitable host sources for
locale enumeration and most of the values needed by `LC_CTYPE`, `LC_NUMERIC`,
`LC_TIME`, and `LC_MONETARY`. `LC_COLLATE` has no scalar `locale -k` keywords,
so a successful host collation capability check is sufficient for that
category. Windows NLS has no source for the required POSIX `LC_MESSAGES`
keywords:

- `yesexpr`
- `noexpr`
- `yesstr`
- `nostr`

The documented `GetLocaleInfoEx` `LCType` constants contain no affirmative or
negative response strings or regular expressions. Synthesizing those values
from a language tag would be locale data in code, not host-backed data.

Windows' system ICU is not an alternative for this category. Raw CLDR locale
XML contains a `posix/messages` section with `yesstr` and `nostr`, but the ICU
CLDR conversion omits the complete `posix` section from its compiled locale
resource bundles. Raw CLDR also does not provide the POSIX `yesexpr` and
`noexpr` values required by this command. Downloading or embedding the raw XML
would repeat the rejected bundled-table design.

Consequently, a Windows NLS locale cannot pass the existing invariant that
every name emitted by `locale -a` must support `locale -k` for every category.
Advertising any of the corpus names while returning an empty or fabricated
`LC_MESSAGES` category would weaken `TestAllLocales` in substance even if the
test were made to accept it.

## Corpus encoding assessment

Windows NLS documents usable code pages for six of the seven requested
locale/codeset pairs (subject to verifying the actual runner with
`IsValidCodePage` and lossless conversion):

| Corpus locale | Windows host encoding source |
| --- | --- |
| `en_US.UTF-8` | code page 65001 |
| `zh_TW.big5` | code page 950 |
| `ja_JP.SJIS` | code page 932 |
| `fr_FR.ISO8859-1` | code page 28591 |
| `de_DE.UTF-8` | code page 65001 |
| `ru_RU.CP1251` | code page 1251 |
| `zh_HK.big5hkscs` | no Windows NLS code-page identifier |

Upstream ICU has a `Big5-HKSCS` converter, but it is an ICU converter rather
than a Windows NLS code page. Its presence in a particular Windows system ICU
data package must be probed on the target runner. Even if present, it does not
resolve the independent `LC_MESSAGES` blocker above.

Therefore the complete seven-locale corpus set cannot be provisioned and
served under the present all-category, no-bundled-data contract using Windows
NLS. A serviceable implementation needs one of these contract changes or host
facilities:

1. a host-installed POSIX locale archive/service that exposes `LC_MESSAGES`
   expressions and Big5-HKSCS, with an in-process API coreutils may call;
2. authorization to ship compiled locale data (currently expressly forbidden);
   or
3. an explicit reduction of the advertised category contract (currently
   forbidden by `TestAllLocales` and this story).

## Primary references

- Microsoft `EnumSystemLocalesEx` documentation:
  <https://learn.microsoft.com/windows/win32/api/winnls/nf-winnls-enumsystemlocalesex>
- Microsoft `GetLocaleInfoEx` documentation and locale information constants:
  <https://learn.microsoft.com/windows/win32/api/winnls/nf-winnls-getlocaleinfoex>
  and
  <https://learn.microsoft.com/windows/win32/intl/locale-information-constants>
- Microsoft code-page identifiers:
  <https://learn.microsoft.com/windows/win32/intl/code-page-identifiers>
- Microsoft system ICU description:
  <https://learn.microsoft.com/windows/win32/intl/international-components-for-unicode--icu->
- CLDR's raw English POSIX messages:
  <https://github.com/unicode-org/cldr/blob/main/common/main/en.xml>
- ICU's generated English locale bundle, which contains no `posix` resource:
  <https://github.com/unicode-org/icu/blob/main/icu4c/source/data/locales/en.txt>
- ICU converter aliases, including `Big5-HKSCS`:
  <https://github.com/unicode-org/icu/blob/main/icu4c/source/data/mappings/convrtrs.txt>
