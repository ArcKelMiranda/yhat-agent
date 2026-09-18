# Third-Party Notices for Documentation Infrastructure

This file documents the provenance and license terms for tooling and generated assets
used by yhat-agent's documentation infrastructure. These notices apply **only** to the
documentation tooling and rendered HTML artifacts; they do not relicense yhat-agent or
its source code.

---

## Archify

**Upstream:** https://github.com/tt-a1i/archify

**Pinned commit:** `72c750bb070d95171dbb2244e5b62b1b7da69c12` (v2.16.0-57-g72c750b)

Archify is licensed under the **MIT License**:

> MIT License
>
> Copyright (c) 2026 tt-a1i (Archify)
> Copyright (c) 2025 Cocoon AI
>
> Permission is hereby granted, free of charge, to any person obtaining a copy
> of this software and associated documentation files (the "Software"), to deal
> in the Software without restriction, including without limitation the rights
> to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
> copies of the Software, and to permit persons to whom the Software is
> furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all
> copies or substantial portions of the Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
> IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
> FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
> AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
> LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
> OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
> SOFTWARE.

---

## Simple Icons (Embedded in Archify HTML Output)

Selected brand vector paths embedded in the rendered HTML are sourced from
**Simple Icons 16.28.0** (https://github.com/simple-icons/simple-icons/tree/16.28.0).

Simple Icons is released under **CC0 1.0 Universal** (Public Domain):
https://creativecommons.org/publicdomain/zero/1.0/

Individual icon licenses vary. Relevant marks in yhat-agent's architecture diagram
retain their recorded upstream licenses (e.g., MIT, Apache-2.0, CC0-1.0, CC-BY-4.0,
CC-BY-SA-3.0, CC-BY-SA-4.0, CC-BY-NC-SA-4.0). See `THIRD_PARTY_NOTICES.md` in the
Archify submodule for the full provenance table.

---

## JetBrains Mono (Embedded in Archify HTML Output)

The rendered HTML artifact embeds **JetBrains Mono** variable font subsets served by
Google Fonts for characters covered by those subsets.

JetBrains Mono is maintained at https://github.com/JetBrains/JetBrainsMono and is
distributed under the **SIL Open Font License 1.1** (OFL).

> Permission is hereby granted, free of charge, to any person obtaining a copy of
> this software and associated documentation files (the "Font Software"), to deal
> in the Font Software without restriction, including without limitation the rights
> to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
> of the Font Software, and to permit persons to whom the Font Software is furnished
> to do so, subject to the following conditions:
>
> The Font Software must be used in accordance with the OFL. The name "JetBrains Mono"
> must be preserved in the font's metadata.
>
> THE FONT SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND.

---

## Summary

| Tool / Asset | License | Applies to |
|---|---|---|
| Archify | MIT | Documentation generation tooling |
| Simple Icons (via Archify) | CC0 1.0 | Brand vector paths in rendered HTML |
| JetBrains Mono (via Archify) | OFL 1.1 | Font subsets in rendered HTML |

These licenses govern only the documentation tooling and generated HTML assets.
**yhat-agent** (the Go CLI) remains proprietary commercial software.
