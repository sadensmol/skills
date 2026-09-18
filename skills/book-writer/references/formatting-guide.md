# Book Formatting Guide (.md)

## Document Structure

### Front Matter
```markdown
# Title of the Book

**Author:** AI-assisted creative writing
**Genre:** [Genre]
**Audience:** [Age group]
**Language:** [Language]

---
```

### For Picture Books / Toddler Stories
```markdown
# The Adventures of [Character]

---

![Illustration suggestion: description of scene]

Big text for the main story line.

**"Dialogue in bold quotes!"**

*Sound effects in italics!*

---
```

### For Short Stories / Kids
```markdown
# Story Title

---

## Chapter 1: Chapter Title

Story text with paragraphs separated by blank lines.

"Character dialogue," said Character Name.

---

## Chapter 2: Chapter Title

Continuation...

---

*The End*
```

### For Longer Works (Middle Grade+)
```markdown
# Book Title

> *A [genre] story*

---

## Part One: Part Title

### Chapter 1: Chapter Title

Opening paragraph that hooks the reader...

---

### Chapter 2: Chapter Title

...

---

## Part Two: Part Title

### Chapter 3: Chapter Title

...

---

## Epilogue

...

---

*The End*
```

## Formatting Conventions

| Element | Markdown | When to use |
|---------|----------|-------------|
| Chapter titles | `## Chapter N: Title` | Every chapter |
| Scene breaks | `---` or `* * *` | Time/location jumps within chapter |
| Dialogue | `"Quoted text,"` | Character speech |
| Thoughts | `*Italicized text*` | Internal monologue |
| Emphasis | `**Bold**` | Key moments, sound effects |
| Sound effects | `**CRASH!**` or `***BOOM!***` | Dramatic moments (especially kids) |
| Letters/notes | `> Blockquote` | In-story documents |
| Whispers | `*"Whispered dialogue"*` | Quiet speech |

## Back Matter (for longer works)
```markdown
---

## About This Story

Brief note about the story's creation or themes.

## Characters

- **Name** - Brief description
- **Name** - Brief description

## Glossary (if needed)

- **Term** - Definition
```

## Continuation Marker

When a story is designed to be continued, end with:
```markdown
---

*To be continued...*

<!-- STORY-STATE
title: Story Title
chapter: N
characters: [list]
plot-threads: [open threads]
setting: [current setting]
tone: [tone]
audience: [age group]
language: [language]
-->
```

This hidden comment preserves state for seamless continuation.
