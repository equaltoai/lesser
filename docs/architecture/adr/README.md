# Architecture Decision Records (ADRs)

This folder captures major **architecture-level decisions** that steer Lesser’s system design.

## Conventions

- **Filename:** `NNNN-short-topic.md`, where `NNNN` is a zero-padded sequence and `short-topic` is a descriptive `kebab-case` summary. The prefix helps keep the chronological order visible.
- **Sections:** every ADR should include at least the following headings:
  1. `Status` – whether the decision is **proposed**, **accepted**, or **superseded**.
 2. `Context` – why the decision surfaced and what constraints or requirements drove it.
 3. `Decision` – the chosen approach (preferably with numbered bullets for clarity).
 4. `Consequences` – what follows from the decision (positive and negative).
 5. `Next steps` (optional) – immediate work that depends on this decision.
 6. `References` – links to related docs, specs, or tickets.
- **Tone:** keep explanations implementation-aware but technology-agnostic when possible; include pointers to supporting code or storage models as needed.

Refer back to `docs/architecture.md` for navigation, and keep this folder scoped to architecture-level trade-offs rather than tactical bug fixes.
