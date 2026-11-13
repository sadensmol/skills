# Greenfield Design Workflow

Read this when you start a Mode 2 greenfield design, before you write `docs/requirements.md`.

### Step 1: Create Requirements Document

**Goal:** Document requirements FIRST in `docs/requirements.md`

**Activities:**
1. Ask questions to clarify requirements
2. Document: business goals, constraints, key actors, success criteria
3. Mark unclear areas as hotspots

**Output:** `docs/requirements.md`

### Step 2: Create Plan Document

**Goal:** Document the system design in `docs/plans/{date}.md`

**Plan should include:**
- Link to requirements document
- Key components and their responsibilities
- Data flow and integration points
- API design (if applicable)
- Trade-offs and decisions made
- Links to supporting docs for complex topics

### Step 3: Create Supporting Documentation

**Goal:** Document complex topics in separate files

**For each complex topic (caching, rate limiting, etc.):**
1. Create `docs/{topic}.md`
2. Document ONLY the chosen approach
3. Keep concise and implementation-focused
4. Link from main plan

**Output:**
- `docs/requirements.md` (created first)
- `docs/plans/{date}.md` (main plan, links to requirements)
- `docs/{topic}.md` for each complex topic
