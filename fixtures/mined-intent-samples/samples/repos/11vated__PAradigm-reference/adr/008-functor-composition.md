# ADR-008: Use Functors for Cross-Domain Composition

**Status:** Accepted
**Date:** 2024-10-22
**Layer:** Layer 5 (Composition)

## Context

Paradigm has 25+ domains. Users want to take a Character seed and "make music for them," or take a Music seed and "make a sprite that visualizes it." We need a principled way to translate seeds across domains that:

1. Preserves identity — the resulting seed must demonstrably *come from* the source.
2. Propagates lineage — royalties and attribution survive translation.
3. Composes — `A → B → C` should work and yield reproducible results.
4. Is principled — not ad-hoc string-matching but grounded in something users (and AIs) can reason about.
5. Doesn't fabricate information — translating Character to Music must use only what's in the Character; novel facts are not allowed.

## Decision

Cross-domain translations are **functors** in the category-theoretic sense: structure-preserving maps from one domain to another. Each functor `F: A → B` is a registered, named, deterministic function that satisfies six laws:

1. **Determinism** — same input seed → same output seed, every time.
2. **Hash consistency** — `hash(F(a)) == hash(F(a))` for all `a`.
3. **Lineage propagation** — `F(a).lineage` includes `a` as a parent edge.
4. **Identity** — for some functors, `F(F(a)) == F(a)` (idempotent).
5. **Associativity** — `(F ∘ G)(a) == F(G(a))`.
6. **No information fabrication** — `F(a)` derives only from facts present in `a` plus the functor's own constants.

The functor registry is a directed graph; cross-domain translation is **shortest-path search** through this graph (see [`algorithms/functor-composition.md`](../algorithms/functor-composition.md)).

The 9 pre-registered functors and their costs are listed in [`architecture/cross-domain-composition.md`](../architecture/cross-domain-composition.md).

## Consequences

**Positive:**

- Cross-domain composition is a small, principled module — not 25² ad-hoc translators.
- Pathfinding is `O(domains × functors)` ≈ microseconds for any realistic registry size.
- Lineage and royalty propagation come for free from law #3.
- New domains plug in by adding outgoing/incoming functors; no special-case code anywhere else.
- Users can discover non-obvious paths (`Sprite → Character → Music`) the platform suggests automatically.
- Functor laws are testable: every PR adding a new functor must ship a property-based test that verifies all 6 laws.

**Negative:**

- The category-theoretic framing is intimidating for some contributors. We document it in plain English in the architecture doc.
- Some translations are fundamentally lossy and the cost values (0.0–1.0) we attach are subjective. We tune them based on user studies and quality-vector measurements on validation seeds.
- Multi-hop paths can chain losses. We cap the max path length at 5 hops to bound output degradation.

## Alternatives Considered

- **Hand-written ad-hoc translators:** What every other tool does. Rejected because it scales as O(N²) and provides no composition guarantees.
- **LLM-based translation:** "Ask GPT-4 to convert this Character to a Song." Rejected because it's non-deterministic, expensive, and violates information-fabrication law #6.
- **Embedding-space neighborhood:** Represent every seed as a vector and find nearest-neighbor in the target domain. Rejected because it doesn't preserve enough structure (a tall warrior maps to a tall building rather than tall music).
- **Rule-based templates:** A library of "if A then B" rules. Rejected because it requires manual rule authoring per domain pair.

## References

- Mac Lane, *Categories for the Working Mathematician* (1971)
- Backus, *Can Programming Be Liberated from the von Neumann Style?* (CACM 1978) — composition philosophy
