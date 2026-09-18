---
name: sadensmol-book-writer
description: "Write original stories, short stories, and books from a brief description. Use when the user wants to: (1) write a story or book, (2) create a children's story, (3) write a short story, (4) continue a previous story, (5) says 'write a story', 'write a book', 'new story', 'continue the story', or similar creative writing requests. Asks for target audience age, language, and style inspiration before writing. Outputs standalone .md files."
---

# Book Writer

## Workflow

### Step 0: Check History

Read [references/history.md](references/history.md) first. Use past preferences to:
- Pre-fill known defaults (favorite authors, language, audience) as suggested options
- Reference past stories if the user might want a continuation or similar style
- Apply learned preferences automatically

### Step 1: Gather Requirements

Before writing anything, ask the user (use AskUserQuestion) for ALL of the following. If some info is already provided or known from history, offer it as the default option.

1. **Description**: What the story is about (may already be provided)
2. **Target audience age**: Toddlers (2-4), Kids (5-8), Middle grade (9-12), Young adult (13-17), Adults (18+)
3. **Language**: What language to write in
4. **Style inspiration**: Ask one of:
   - "Which author's style do you love? I'll channel their voice."
   - "Any specific book you'd like this to feel similar to?"
   - If history has preferred authors, offer them as options
   - Let the user pick from both options or provide their own direction
5. **Length preference** (optional): Short story, longer story, full book chapter(s)

### Step 2: Research the Style

Once the user names an author or book:
- Recall key characteristics of that author's style (sentence rhythm, humor type, narrative voice, vocabulary level, recurring themes, structural patterns)
- For children's authors: note their specific techniques (e.g., Roald Dahl's gleeful mischief + made-up words, Dr. Seuss's rhyme + absurdity, Julia Donaldson's rhythmic verse)
- Adapt the style to the requested story concept naturally -- channel the spirit, don't copy

### Step 3: Select Audience Parameters

Read [references/audience-guide.md](references/audience-guide.md) for detailed parameters per age group. Key decisions:
- Word count range
- Vocabulary complexity
- Humor style
- Structural approach
- Narrative techniques

### Step 4: Write the Story

Read [references/formatting-guide.md](references/formatting-guide.md) for .md formatting conventions.

**Core writing principles:**
- **Hook immediately** -- first paragraph must grab attention
- **Show, don't tell** -- use sensory details, action, dialogue
- **Humor is mandatory for kids** -- at minimum one laugh per page equivalent
- **Authentic voice** -- match the inspired author's rhythm and tone
- **Satisfying arc** -- even short stories need setup, tension, resolution
- **End strong** -- last line should land with impact (funny, warm, surprising, or thought-provoking)

**For kids specifically:**
- Use the Rule of Three (three attempts, three friends, three challenges)
- Include onomatopoeia and sound effects for younger audiences
- Make protagonists resourceful -- they solve their own problems
- Sneak in subtle messages without being preachy
- Adults in the story can be funny/flawed (kids love this)

**Quality bar:**
- Would this hold attention if read aloud?
- Does it have genuine moments of delight?
- Would a real reader want to know what happens next?
- Does the ending feel earned?

### Step 5: Output the File

Save the story as a `.md` file in the current working directory.

**Filename**: Use a slugified version of the title, e.g., `the-great-adventure-of-captain-frog.md`

**File structure** (from formatting-guide.md):
- Title and metadata at the top
- Proper chapter/section headings
- Scene breaks with `---`
- Formatted dialogue
- A hidden `<!-- STORY-STATE -->` comment at the end with continuation metadata

### Step 6: Update History

After every story creation, update [references/history.md](references/history.md) using the Edit tool:

1. **Add to "Past Creations"**: title, date, audience, language, style inspiration, filename, 1-2 sentence summary
2. **Add to "Preferred Authors & Styles"**: if the user chose a new author/book, record it with style notes (only add if not already listed)
3. **Add to "Learned Preferences"**: if the user gave feedback (liked/disliked something, asked for adjustments), record the pattern

This builds a personalized profile that improves future stories. The history file lives in the skill directory so it persists across all sessions.

### Step 7: Offer Next Steps

After writing, ask: "Would you like me to continue this story, adjust the tone, or start something new?"

## Continuing a Previous Story

When the user wants to continue a previous story:

1. Read the existing `.md` file
2. Extract the `<!-- STORY-STATE -->` comment for context (characters, plot threads, setting, tone, audience, language)
3. If no state comment exists, analyze the story content to reconstruct context
4. Continue seamlessly -- match voice, maintain character consistency, advance open plot threads
5. Append new content to the same file (new chapters/sections)
6. Update the `<!-- STORY-STATE -->` comment with new state
