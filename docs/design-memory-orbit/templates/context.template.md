# {Context Name}

{1-2 sentences explaining what this context is and why it exists.}

## Language

**Term**:
A one-sentence definition for domain experts. Define what it is, not implementation details.
_Avoid_: synonym, overloaded word

## Relationships

- **Term A** produces one or more **Term B**.
- **Term B** belongs to exactly one **Term C**.

## Boundaries

- **Context A** owns ...
- **Context B** references ...

## Example Dialogue

> **Dev:** "When does **Term A** create **Term B**?"
> **Domain expert:** "Only after **Term C** is confirmed."

## Flagged Ambiguities

- "account" was used to mean both **Customer** and **User**. Resolution: these are distinct concepts.

## Open Questions

- ...
