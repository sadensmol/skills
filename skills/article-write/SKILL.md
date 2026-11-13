---
name: sadensmol-article-write
description: >
  Write new blog articles for sadensmol.com (Hugo blog). Use when the user wants to:
  (1) write a new blog post from a URL/link source,
  (2) write a new blog post from scratch on a topic,
  (3) create an article for sadensmol.com,
  (4) says "write an article", "new post", "blog post", or "article about".
  Supports two modes: rewriting content from a source link in the author's voice,
  or writing original articles from a topic description.
---

# Article Writer for sadensmol.com

## Workflow

### Step 0: Locate the blog directory

Resolve the Hugo blog directory before anything else — **never hardcode a local path**:

1. **Check the current directory** (and walk up parent dirs): treat it as the blog if it contains a Hugo config (`hugo.toml` / `hugo.yaml` / `config.toml` / `config.yaml`) **and** a `content/posts/` directory.
2. **If found**, use it as the blog directory.
3. **If not found**, ask the user which directory to use — present it as a selectable dropdown question (e.g. via AskUserQuestion) listing any detected candidates plus an "enter a path" option. Do **not** guess or fall back to a hardcoded location.

Use the resolved path as the blog directory for all file operations below (`content/posts/`, `static/images/posts/`).

### Step 1: Determine article source

Ask the user:

> Would you like to write an article from a link, or from scratch?
>
> - **From a link** — provide a URL and I'll read it, then rewrite the content in your voice
> - **From scratch** — describe the topic and I'll write an original article

### Step 2: If from a link — detect link type and fetch content

Read [references/from-link.md](references/from-link.md) now, and follow it for the link type (git repository, `learning_*` series repo, or regular URL).

### Step 3: Gather article details

Ask the user for:
- **Title** (suggest 2-3 options based on the topic — or auto-generate if it's a learning series)
- **Tags** (suggest relevant ones from existing tags, see [blog-setup.md](references/blog-setup.md))
- **Draft or publish?** (draft = true/false)

### Step 4: Read writing style reference

Read [references/writing-style.md](references/writing-style.md) for voice, tone, structure, and formatting rules. Match the style of existing articles on sadensmol.com.

Key style points:
- First person, conversational, opinionated
- Short paragraphs, code examples for every technical claim
- Problem → Solution structure
- No AI-sounding phrases
- End with Summary/Conclusion, optionally PS for reader engagement

### Step 5: Read blog setup reference

Read [references/blog-setup.md](references/blog-setup.md) for frontmatter format, file naming, and image handling.

### Step 6: Create the article file

1. Create the file at `content/posts/YYYY-MM-DD-slug.md` in the blog directory (resolved in Step 0)
2. Use TOML frontmatter with `+++` delimiters
3. Include: title, slug, date (today), draft status, description (80-150 chars), tags

### Step 7: Write the article

Write the full article following the style guide. Key rules:
- Write as sadensmol — first person, from personal experience
- Keep it practical and easy to understand
- Include code examples where relevant (with language tags)
- Use `##` for sections, `###` for subsections
- Keep articles 150-400 lines (target ~5 min read / 1000-1500 words)
- End with a short Summary/Conclusion

**Hero image:** Every article should have a hero image — a real photograph (not a generated graphic or diagram). **ALWAYS use the browser that the `sadensmol-router` browser gate selects** to search Google Images for a visually striking photo that metaphorically represents the article's topic (e.g. dominos for chain reactions, network cables for distributed systems). Download it via the browser and save as `/static/images/posts/<slug>-hero.jpg`. Place the hero image **after the opening paragraph**, not right after the title. Do NOT use a web search, a plain page fetch, or curl for hero image search/download — always drive the browser.

**Diagrams:** Every article MUST also include at least 1-2 diagrams to illustrate key concepts. Read [references/diagrams.md](references/diagrams.md) before you make one.

**Source links:** If the article is based on a source URL, follow *Including source links* in [references/from-link.md](references/from-link.md).

### Step 8: Generate social media posts

After writing the article, generate ready-to-post text for LinkedIn and X.com (Twitter).

Read [references/social-posts.md](references/social-posts.md) before you write them.

### Step 9: Review and refine

Present the article to the user for review. Offer to:
- Adjust tone or length
- Add/remove sections
- Change title or tags
- Add code examples
- Modify social media posts (LinkedIn/X.com/Threads)

**Terminology — do NOT say "published".** Creating the `.md` file (even with
`draft = false`) only means it will appear on the *next build*. The file is
still **uncommitted and undeployed**. When reporting back, say the post/draft
was *created* or *written* — never "published" or "live". Never run
`git add`/`git commit` (the user commits on their own); if helpful, you may list
the new/changed paths so they can stage them.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When | Read |
|------|------|
| Step 2, the source is a URL or git repo; Step 7, placing source links | [references/from-link.md](references/from-link.md) |
| Step 4, before writing | [references/writing-style.md](references/writing-style.md) |
| Step 3 (tags) and Step 5, frontmatter, file naming, images | [references/blog-setup.md](references/blog-setup.md) |
| Step 7, before making a diagram | [references/diagrams.md](references/diagrams.md) |
| Step 8, before writing the social posts | [references/social-posts.md](references/social-posts.md) |

## Important Rules

- NEVER copy source content verbatim — always rewrite from scratch in sadensmol's voice
- Read at least 2 existing articles before writing to calibrate the voice
- Use existing tags when possible, only create new tags if truly needed
- Target ~5 minute read time (1000-1500 words, 150-400 lines) — cover the topic thoroughly, not just a surface skim
- File naming must follow `YYYY-MM-DD-slug.md` pattern
- Description should be a single concise sentence capturing the article's value
- Every article must include at least 1-2 diagrams/images to illustrate key concepts
- Generate diagrams using `mmdc` (globally installed) with `-b transparent -t neutral` — blog CSS auto-inverts on dark theme
- **NEVER use ASCII-art tables or box diagrams** (`┌──┐`, `│ │`, `└──┘`) — they render badly. Use Mermaid diagrams for any table/box/layout schematic instead
- Always generate LinkedIn, X.com, and Threads posts after writing the article
- Always include source/reference links when article is based on external content
- For `learning_*` repos (e.g. `learning_system-design`), always use the series title format: `Learning System Design #<N>: <Topic>` — auto-detect the part number from existing posts
- Series intro opener uses the `#N` numeral (e.g. `Part #12 of the ... series!`), never a spelled-out ordinal
- Creating the file is NOT publishing — `draft = false` just queues it for the next build; the file stays uncommitted/undeployed. Never claim a post is "published" or "live"
