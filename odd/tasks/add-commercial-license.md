# Add custom commercial license

## Confirmed terms
- Licensor: ArcKelMiranda.
- Authorized customers may integrate the software into their own products or services.
- Source modifications are allowed only for the customer's internal use.
- Publication or redistribution of source code and modifications is prohibited unless separately authorized.
- Term, fees, support, warranties, governing law, jurisdiction, and dispute resolution are defined per customer contract or order form.

## Tasks
- [x] Draft a clear repository-level commercial license reflecting the confirmed terms.
- [x] Replace the README's MIT reference with the commercial licensing summary and contact requirement.
- [x] Verify consistency: run `grep` for MIT strings and licensor typos, `git diff --check`, and independent term-by-term review.
- [x] Work-unit commit the documentation change (NOT pushed, NOT merged).
- [x] Merge into main after explicit user approval.

## Evidence
- Work-unit commit: `5c8d575` (`docs: add proprietary commercial license`).
- Fast-forward merged and pushed to `main` at `64adec1` after explicit user approval.
- `LICENSE` created: proprietary commercial license covering all confirmed terms, ownership, trade-secret-scoped confidentiality (not blanket source-line confidentiality), reverse-engineering restrictions, dual-governing-document agreement-precedence clause, third-party component treatment, termination effects, and disclaimer/LL subject to Commercial Agreement.
- README `License` section replaced: no longer claims MIT; summary directs prospective users to obtain written commercial authorization via GitHub communication channels; proprietary/all-rights-reserved status retained; no blanket confidential property claim; full terms deferred to LICENSE.
- ODD task updated: drafting and README work marked complete; verification and commit deferred.

## Corrections applied (review feedback)
1. Contact language (LICENSE §11, README): directed to repository owner's official GitHub communication channels — no agreement channels assumed, no email invented.
2. Entire-agreement contradiction (LICENSE §10): removed "entire agreement" claim; §10 now states this License + the executed Commercial Agreement are the governing documents, with the Commercial Agreement prevailing on conflict (consistent with §5 and §6).
3. Over-scoped confidentiality (LICENSE §4, README): §4 scoped to trade secrets, designated confidential materials, and Commercial Agreement-protected disclosures — not to all visible source lines. README "confidential property" softened to trade secret acknowledgment with public-visibility carveout.

## Legal note
This repository license text is a practical template. It should receive qualified legal
review before commercial execution. The separately executed Commercial Agreement governs
actual rights and obligations.

## Files changed (expected)
- `LICENSE` — new file, custom proprietary commercial license
- `README.md` — License section replaced with proprietary summary
- `odd/tasks/add-commercial-license.md` — this file, evidence updated
