# Superseded 1.x requirements left the registry

On 2026-10-14 the spec registry stopped waiving 1.x requirements that the 2.0 contract replaced without a successor.
A tier 1.1 waiver now names the `CONTRACT.md` section that supersedes the requirement, and `spec-score --lint` rejects one that does not.
319 tier 1.1 requirements had no such section and left the registry and the ratchet.
They were the generic "superseded" waivers, the unclassified test gaps, and the process and consumer items that no Evo test can observe.
The other 42 stay as waivers because each names a 2.0 successor, and each now cites its section (for example `Skipped` to `Exclude`, `CONTRACT §4`).
The 19 registry files that became empty keep their names: comments still cite the 1.x spec section numbers they mirror.
Removed per section: C01 3, C02 54, C03 6, C04 10, C05 7, C06 3, C07 3, C08 3, C09 3, C10 9, C11 5, C12 8, C13 11, C14 3, C15 7, C16 15, C17 9, C18 10, C19 8, C20 4, C21 18, C22 23, C23 2, C24 3, C25 2, C26 8, C27 10, C28 3, C29 3, C30 62, C33 4.
`git diff` of the deleting commit lists every removed ID.
