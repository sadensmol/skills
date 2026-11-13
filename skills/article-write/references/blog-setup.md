# sadensmol.com Blog Technical Setup

## Platform
Hugo static site generator. The blog directory is resolved at runtime (see Step 0 in SKILL.md) — never hardcode its path. All paths below (`content/posts/`, `static/images/posts/`) are relative to that resolved blog root.

## Creating a New Post
Use the Makefile command which auto-generates the file with correct naming:
```bash
# In the blog root directory, run:
echo "Post Title Here" | make add-post
```
This creates: `content/posts/YYYY-MM-DD-slug.md`

**Alternative (manual):** Create file directly at `content/posts/YYYY-MM-DD-slug.md`

## Frontmatter Format (TOML with +++ delimiters)
```toml
+++
title = 'Article Title'
slug = 'article-slug'
date = YYYY-MM-DD
draft = true
description = 'One-line description (80-150 chars)'
tags = ['tag1', 'tag2']
+++
```

### Field notes:
- `slug` - optional, auto-derived from filename if omitted
- `draft` - set to `true` or `false`
- `description` - keep concise, describes the core takeaway
- `tags` - lowercase, common tags: `go`, `golang`, `java`, `kotlin`, `grpc`, `microservices`, `clean-architecture`, `solid`

## Images
- Store in: `/static/images/posts/`
- Reference in article: `![alt text](/images/posts/filename.ext)`
- Formats: PNG, JPEG
- Hero images: every article should have one — a real photograph (not generated), placed after the opening paragraph
- All images are auto-centered via CSS (`display: block; margin: auto`)
- Dark theme auto-inverts only PNG images (`img[src$=".png"]`) via CSS `filter: invert(0.88) hue-rotate(180deg)` — so diagrams (PNG) are readable on dark theme, while hero photos (JPG) keep natural colors. Save monochrome diagrams as `.png`, colored diagrams as `.jpg` (to skip the inversion), and photos as `.jpg`

## File Naming Convention
`YYYY-MM-DD-slug-words-here.md`
Examples:
- `2023-02-28-from-java-kotlin-to-go.md`
- `2024-10-05-go-gems-1-powerful-go-context-in-msa.md`

## Local Preview
```bash
make up  # starts hugo server with drafts enabled
```

## Existing Tags (for consistency)
ai, analytics, authentication, backup, clean-architecture, clickhouse, closures, cloud-native, database, dependency-injection, devops, encryption, exception, gaming, go, golang, grpc, http, immutability, interfaces, java, kotlin, kubernetes, llm, microservices, money, mongodb, networking, opinion, postgresql, rag, rate-limiting, security, software-development, solid, spring-boot, sqlite, system-design, vector-database, web, webrtc

This list goes stale. Before suggesting tags, list the current ones from the blog's `content/posts/*.md` frontmatter.
