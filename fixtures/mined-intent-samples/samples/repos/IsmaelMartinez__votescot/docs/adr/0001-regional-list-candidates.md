# ADR-0001: Model regional list candidates as first-class data

## Status

Accepted — 27 April 2026.

## Context

The Scottish Parliament is elected via an Additional Member System (AMS). Each voter receives two ballots on 7 May 2026: a constituency ballot returning one of 73 first-past-the-post MSPs, and a regional list ballot allocating 56 additional MSPs across 8 regions (7 per region) by D'Hondt against ordered party lists.

Until 26 April 2026 VoteScot only modelled constituency candidates. The "regional" surfaces — `/quiz/regional` and `/candidates/region/[id]` — were stand-ins: they filtered constituency candidates by their parent region. Because parties stand different candidates on the regional list, this was misleading. A Glasgow voter looking at our Glasgow regional page saw the constituency contenders for that area, not the actual list candidates the same parties had nominated for the regional ballot.

Scottish Parliament voters cannot meaningfully be guided to a single ballot. The regional list returns 43% of MSPs and is the dominant pathway to Holyrood for smaller parties — Greens, Alba, Reform UK, and others — that struggle to win a constituency outright. A vote compass that omits the regional list cannot honestly claim to cover the election.

## Decision

Treat regional list candidates as a first-class entity in the data model.

Store them in `data/regional-candidates/<slug>.yaml`, separate from `data/candidates/`. Use Democracy Club's locked `sp.r.2026-05-07` ballots as the source of truth, refreshed by `scripts/sync-regional-candidates.ts`. Each record carries `region`, `regionLabel`, `listPosition`, and `ballotPaperId`; D'Hondt seat allocation depends on list ordering, so the rank matters and is captured. Validate the shape against `schemas/regional-candidate.schema.json`.

Use the official 8-region structure established by the 2025 Boundaries Scotland Second Periodic Review and reflected in Democracy Club's ballots. `Central Scotland and Lothians West` is one region — not two — under the new boundaries. The legacy 9-region split currently present in some constituency `region:` fields is wrong and will be reconciled in a follow-up.

## Consequences

The `/quiz/regional` and `/candidates/region/[id]` pages will, once rewired, show real list candidates and the "we don't yet model separate regional list candidates" disclaimer can be dropped. Voters can match against the actual ballot they will see on 7 May 2026, including list-only candidates from smaller parties. Modelling the regional list is a credibility prerequisite for VoteScot to claim election coverage.

The cost is roughly 589 new YAML files, one per regional candidacy — a 2.4× increase in candidate file count. Build time impact is negligible (loaders are module-cached) but bulk-edit diffs become noisier. Two parallel data shapes (`data/candidates/` for constituency, `data/regional-candidates/` for regional list) mean scripts that walk candidate data — `apply-party-positions.ts`, `fix-incumbents.ts` — need to walk both trees. Each script needs an explicit follow-up. Some people appear on both ballots (a candidate can stand in a constituency and on their party's regional list); they have separate records in each tree, and cross-references between the two are not modelled. That is acceptable for this election.

## Alternatives considered

A single flat candidate table with a `ballot: "constituency" | "regional"` discriminator field is cleaner conceptually but every existing loader, schema, page, script, and test assumes one record per candidate. Migrating in place is a high-risk refactor with 10 days to polling day, so it was rejected in favour of the parallel-tree approach which leaves the constituency side untouched.

Continuing the stand-in approach (filter constituency candidates by region with a disclaimer) is the status quo before this decision. It misleads voters and renders list-only parties invisible. The disclaimer mitigates the misleading-ness but does not remove it; voter trust is the product, so it was rejected.

Deferring to a 2027+ release was considered but the underlying data is already locked and freely available from Democracy Club. The implementation cost is bounded by the work plan in `docs/ROADMAP.md`; the cost of shipping a half-coverage compass on 7 May is reputational and unbounded.
