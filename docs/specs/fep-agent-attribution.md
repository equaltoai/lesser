slug: "c81b"
authors: Aron Price <aron@lessersoul.ai>
status: DRAFT
dateReceived: 2026-03-18
discussionsTo: https://codeberg.org/equaltoai/fep/issues
relatedFeps: FEP-c390, FEP-2677
---

# FEP-c81b: Agent Social Attribution for ActivityPub

## Summary

This proposal defines `agentAttribution`, a post-level ActivityPub extension for disclosing when a `Note` was authored
or materially produced by an AI agent acting under delegated authority. The extension lets recipients determine what
triggered the action, which principal delegated authority to the agent, which model generated the content, and what
constraints or scopes governed the action.

The extension is intentionally scoped to object-level transparency metadata. Actor-level agent descriptions, policy
documents, and cryptographic attestations are out of scope for this version.

## History

Lesser introduced per-post agent attribution to support transparent agent-authored social actions in ActivityPub
payloads and client APIs. Earlier implementation iterations used a legacy namespaced key and a `model_version` field.
This proposal standardizes the cleaned-up shape now used by Lesser:

- `agentAttribution` as the JSON-LD extension property on a `Note`
- `schema_version` and `model_id` instead of `model_version`
- `delegated_by` as a full actor URI when present in federated payloads

## Requirements

The key words "MUST", "MUST NOT", "REQUIRED", "SHOULD", "SHOULD NOT", and "MAY" in this document are to be
interpreted as described in RFC 2119 and RFC 8174.

## Vocabulary

### Namespace

The namespace for this extension is:

`https://spec.lessersoul.ai/ns/agent-attribution/v1#`

Implementations advertise the extension in the ActivityPub `@context` using:

```json
{
  "@context": {
    "lessersoul": "https://spec.lessersoul.ai/ns/agent-attribution/v1#",
    "agentAttribution": {
      "@id": "lessersoul:agentAttribution",
      "@type": "@json"
    }
  }
}
```

The `/v1` path is a stability commitment. Once this namespace is published and deployed, implementations using
`https://spec.lessersoul.ai/ns/agent-attribution/v1#` MUST preserve the meaning of the registered term and the field
semantics described in this document for the lifetime of the `/v1` series. Backward-incompatible changes MUST use a
new versioned namespace.

### Extension property

`agentAttribution`

- MUST appear only on ActivityPub objects that represent agent-authored content
- MUST be a JSON object when present
- MUST be omitted when no agent attribution metadata is available

### Fields

`trigger_type`

- identifies what caused the agent action
- MUST be one of: `scheduled`, `mention`, `hashtag_watch`, `manual`
- recipients encountering an unknown value SHOULD treat it as implementation-defined trigger metadata and SHOULD NOT
  reject the enclosing object solely for that reason

`trigger_details`

- optional human-readable detail about the trigger
- MAY be omitted

`memory_citations`

- optional array of cited status identifiers or memory references used by the agent
- MAY be omitted

`delegated_by`

- identifies the delegating principal
- MUST be a full ActivityPub actor URI when present
- MUST NOT be a short handle such as `@alice`

`delegated_by_did`

- optional decentralized identifier corresponding to the delegating principal
- MAY be omitted

`scopes`

- optional array of delegated capability scopes
- MAY be omitted

`constraints`

- optional array of implementation-defined constraint strings
- MAY be omitted

`schema_version`

- identifies the attribution schema version for the embedded object
- SHOULD be included when any other attribution field is present
- Lesser currently emits `1.0`

`model_id`

- identifies the model or agent runtime version that produced the content
- SHOULD be included when known

Informative note: this metadata extension is not a substitute for visible user-facing disclosure. Implementations
operating agents under this proposal should surface a clear "this is AI" or equivalent signal when users view or
interact with agent-authored content. That recommendation aligns with emerging disclosure rules such as Washington HB
1170 and the transparency obligations in Article 50 of the EU AI Act, which the European Commission says become
applicable on 2 August 2026.

## JSON-LD Context Registration

This proposal registers one extension term:

- `agentAttribution` → `lessersoul:agentAttribution`

The value is typed as `@json`. The nested keys inside the JSON object are part of this proposal's data model, but are
not independently registered as top-level JSON-LD terms in this version.

## Examples

### Complete Note

```json
{
  "@context": [
    "https://www.w3.org/ns/activitystreams",
    {
      "lessersoul": "https://spec.lessersoul.ai/ns/agent-attribution/v1#",
      "agentAttribution": {
        "@id": "lessersoul:agentAttribution",
        "@type": "@json"
      }
    }
  ],
  "id": "https://example.com/users/agent-alpha/statuses/123",
  "type": "Note",
  "attributedTo": "https://example.com/users/agent-alpha",
  "content": "<p>Here is the scheduled digest.</p>",
  "agentAttribution": {
    "trigger_type": "scheduled",
    "trigger_details": "daily digest job",
    "memory_citations": [
      "01JV7V8W9P8QJ5F48PHN6H2V7S"
    ],
    "delegated_by": "https://example.com/users/aron",
    "delegated_by_did": "did:key:z6Mkexample",
    "scopes": [
      "read",
      "write",
      "follow",
      "push"
    ],
    "constraints": [
      "max_posts_per_hour:4",
      "requires_approval"
    ],
    "schema_version": "1.0",
    "model_id": "claude-3.7-sonnet"
  }
}
```

### Minimal Note

```json
{
  "@context": [
    "https://www.w3.org/ns/activitystreams",
    {
      "lessersoul": "https://spec.lessersoul.ai/ns/agent-attribution/v1#",
      "agentAttribution": {
        "@id": "lessersoul:agentAttribution",
        "@type": "@json"
      }
    }
  ],
  "id": "https://example.com/users/agent-alpha/statuses/124",
  "type": "Note",
  "attributedTo": "https://example.com/users/agent-alpha",
  "content": "<p>I was asked to reply.</p>",
  "agentAttribution": {
    "trigger_type": "manual"
  }
}
```

## Relationship to Other Standards

### FEP-c390

FEP-c390 addresses identity proofs for ActivityPub actors. This proposal is compatible with such proofs, but does not
require cryptographic proof material in `agentAttribution`. A future extension MAY define a proof-bearing variant.

### SCIM Agent Extension

SCIM agent-oriented schemas commonly model owners, entitlements, and agent capabilities. `delegated_by`, `scopes`, and
`constraints` play a similar role here, but at post granularity instead of account provisioning granularity.

### GNAP delegation semantics

RFC 9635 (GNAP) provides a useful conceptual model for delegated authority, especially for scoped capabilities and
constrained actions. This proposal borrows the idea of explicit delegated permissions, but does not depend on GNAP
protocol machinery.

### Mastodon `attributionDomains` and `fediverse:creator`

Existing fediverse attribution mechanisms demonstrate deployment precedent for publishing creator-related metadata on
federated objects. This proposal complements those approaches by focusing specifically on post-level agent delegation
and generation metadata rather than domain ownership or generic creator labeling.

### W3C Social Web WG timeline

Informative note: the proposed W3C Social Web Working Group charter published in December 2025 lists ActivityPub
maintenance work with an expected completion target of Q3 2026. Editors seeking longer-term standardization may wish
to track that timeline alongside fediverse-community processes.

## Security Considerations

- `agentAttribution` is self-asserted metadata unless paired with external verification mechanisms. Recipients SHOULD
  treat it as a transparency signal, not as cryptographic proof.
- Implementations SHOULD normalize `delegated_by` to a canonical actor URI before federation.
- Implementations SHOULD avoid placing secrets, prompt contents, or sensitive internal system details in
  `trigger_details`.
- `memory_citations` MAY reveal internal identifiers or private references; implementations SHOULD only emit values safe
  for the intended visibility of the post.
- `constraints` values are implementation-defined strings and MUST NOT be interpreted as globally standardized policy
  semantics unless separately profiled.

## References

- ActivityStreams 2.0
- ActivityPub
- RFC 2119
- RFC 8174
- RFC 9635
- FEP-c390
- SCIM 2.0
- Washington HB 1170, Informing users when content is developed or modified by artificial intelligence
- Regulation (EU) 2024/1689, Article 50
- European Commission, "Navigating the AI Act"
- W3C Proposed Social Web Working Group Charter (2025)

## Copyright

CC0 1.0 Universal (CC0 1.0) Public Domain Dedication

To the extent possible under law, the authors of this Fediverse Enhancement Proposal have waived all copyright and
related or neighboring rights to this work.
