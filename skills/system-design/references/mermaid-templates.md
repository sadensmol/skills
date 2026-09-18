# Mermaid Diagram Templates

## EventStorming Big Picture Template

```mermaid
flowchart LR
    %% EventStorming Color Conventions
    classDef event fill:#ff9800,stroke:#e65100,color:#000
    classDef command fill:#2196f3,stroke:#0d47a1,color:#fff
    classDef actor fill:#ffeb3b,stroke:#f57f17,color:#000
    classDef system fill:#9c27b0,stroke:#4a148c,color:#fff
    classDef aggregate fill:#4caf50,stroke:#1b5e20,color:#fff
    classDef hotspot fill:#f44336,stroke:#b71c1c,color:#fff

    %% Define actors
    Actor1[Actor Name]:::actor

    %% Define commands
    Cmd1[Command Name]:::command

    %% Define events
    Evt1[Event Name]:::event

    %% Define aggregates
    Agg1[Aggregate Name]:::aggregate

    %% Define external systems
    Sys1[External System]:::system

    %% Define hotspots
    Hot1[? Unclear aspect to discuss]:::hotspot

    %% Define flow
    Actor1 --> Cmd1
    Cmd1 --> Evt1
    Evt1 --> Agg1
    Agg1 --> Sys1
    Evt1 -.question.- Hot1
```

## Process EventStorming Template

```mermaid
flowchart LR
    %% EventStorming Process Level - More detailed than Big Picture
    classDef event fill:#ff9800,stroke:#e65100,color:#000
    classDef command fill:#2196f3,stroke:#0d47a1,color:#fff
    classDef actor fill:#ffeb3b,stroke:#f57f17,color:#000
    classDef policy fill:#ba68c8,stroke:#6a1b9a,color:#fff
    classDef aggregate fill:#4caf50,stroke:#1b5e20,color:#fff

    %% Start with triggering event or command
    Cmd1[Process Start Command]:::command
    Evt1[First Event]:::event
    Agg1[Aggregate Name]:::aggregate

    %% Policy (business rule)
    Policy1{Business Rule Check}:::policy

    Evt2[Next Event]:::event
    Evt3[Alternative Event]:::event

    %% Flow
    Cmd1 --> Evt1
    Evt1 --> Agg1
    Agg1 --> Policy1
    Policy1 -->|condition met| Evt2
    Policy1 -->|condition failed| Evt3

    %% Annotations
    Evt1 -.data changes.- Note1[field_x updated]
    Evt2 -.data changes.- Note2[field_y set]
```

## Entity-Relationship Diagram Template

```mermaid
erDiagram
    EntityA ||--o{ EntityB : relationship
    EntityA {
        uuid id PK
        string attribute1
        int attribute2
        timestamp created_at
    }
    EntityB {
        uuid id PK
        uuid entity_a_id FK
        string attribute1
        decimal attribute2
    }
    EntityC ||--|| EntityA : "one-to-one"
    EntityC {
        uuid id PK
        uuid entity_a_id FK "unique"
        string attribute1
    }
```

## State Chart Template

```mermaid
stateDiagram-v2
    [*] --> InitialState
    InitialState --> StateTwo: trigger_action()
    StateTwo --> StateThree: another_action()
    StateTwo --> StateFour: error_condition()
    StateThree --> [*]
    StateFour --> StateTwo: retry()

    note right of StateTwo
        Data changes:
        - field_name set to value
        - timestamp updated
    end note

    note right of StateFour
        Error state:
        - error_message captured
        - retry_count incremented
    end note
```

## Sequence Diagram Template

```mermaid
sequenceDiagram
    actor User
    participant SystemA
    participant SystemB
    participant ExternalService

    User->>SystemA: initiating action
    SystemA->>SystemB: request data
    SystemB->>ExternalService: external API call
    ExternalService-->>SystemB: response data
    SystemB-->>SystemA: processed data

    alt Success case
        SystemA->>SystemA: process internally
        SystemA-->>User: success response
    else Error case
        SystemA-->>User: error response with details
    end

    Note over SystemA,SystemB: Important interaction note
    Note right of ExternalService: External dependency
```

## Requirements Template

```markdown
# Requirements: {Project Name}

## Business Goals

{What problem does this system solve? What value does it provide?}

## Key Actors

| Actor | Description | Primary Goals |
|-------|-------------|---------------|
| {Actor 1} | {Who they are} | {What they want to achieve} |

## Constraints

### Technical
- {Technical constraints}

### Business
- {Budget, timeline, compliance requirements}

### Scale
- {Expected users, transactions, data volume}

## Success Criteria

- {How do we measure success?}

## External Systems

| System | Purpose | Integration Type |
|--------|---------|-----------------|
| {System 1} | {What it does} | {API, webhook, batch, etc.} |

## Open Questions / Hotspots

- {Unclear requirements}
- {Risk areas to explore}
```

## Plan Document Template

```markdown
# System Design: {Project Name}

> {Brief 1-2 sentence description}

## Core Documents

- [requirements.md](../requirements.md) - Actors, constraints, scale, success criteria
- [big-picture.mmd](../big-picture.mmd) - EventStorming big picture diagram

## Process Models

### {Process Name}

\`\`\`mermaid
{COPY ENTIRE CONTENT FROM ../process-{name}.mmd HERE}
\`\`\`

## Data Model

\`\`\`mermaid
{COPY ENTIRE CONTENT FROM ../erd.mmd HERE}
\`\`\`

## State Models

### {Entity} Lifecycle

\`\`\`mermaid
{COPY ENTIRE CONTENT FROM ../state-{entity}.mmd HERE}
\`\`\`

## Critical Flows

### {Flow} Flow

\`\`\`mermaid
{COPY ENTIRE CONTENT FROM ../sequence-{flow}.mmd HERE}
\`\`\`

## Technical Deep Dives

For complex topics, see supporting documentation:

- [Caching Strategy](../caching.md) - {if caching is used}
- [Rate Limiting](../rate-limiting.md) - {if rate limiting is used}
- [Message Queues](../message-queues.md) - {if async messaging is used}

## Hotspots & Open Questions

{List of unclear areas, risks, or decisions to be made}

## Next Steps

{Implementation order, dependencies, or next phase}
```

## Supporting Documentation Template

```markdown
# {Topic Name}

> Focused documentation for the chosen approach - not a comparison of alternatives.

## What It Is

{1-2 paragraph explanation of the concept}

## How It Works

{Explain the mechanism, include diagram if helpful}

\`\`\`mermaid
{Optional: Diagram showing how this works in context}
\`\`\`

## Why This Approach

{Brief rationale for why this was chosen over alternatives - keep concise}

## Key Configuration

{Important settings, thresholds, or parameters to consider}

## Related Patterns

- {Link to related docs if applicable}
```
