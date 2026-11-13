# Writing from a link

Read this when the article source is a URL or a git repository (Step 2), and again in Step 7 to place the source links.

## Detect the link type and fetch the content

**If the link is a GitHub/git repository:**
1. Use `gh` CLI or the `sadensmol-github` skill to retrieve repo info: name, description, README, relevant code/docs
2. Check if the repo belongs to the user (sadensmol) — look for `sadensmol` in the URL
3. **If it's a `learning_system-design` repo (or similar `learning_*` pattern):**
   - This is a learning series — the article title must follow the format: `Learning System Design #<N>: <Topic>`
   - To determine `<N>`: scan existing posts in `content/posts/` for files matching `*learning-system-design*` and count them — the new article is the next number
   - The slug should be: `learning-system-design-<N>-topic-name` (e.g. `learning-system-design-1-how-http-works`)
   - **Intro opener**: start the article body with the `#N` numeral form, e.g. `Part #12 of the "Learning System Design" series!` — NEVER spell out the ordinal ("Twelfth part", "Second article")
   - Extract the specific topic/theme from the repo content (README, folder structure, code)
4. If it's any other git repo — use the repo description, README, and code to understand the topic, then proceed normally

**If the link is a regular article/blog/docs URL:**
1. Fetch the source URL. If the page needs a login, open it in the browser that the `sadensmol-router` browser gate selects instead
2. Extract the core topic, key points, code examples, and technical insights

In all cases:
- Do NOT copy the original — rewrite entirely in sadensmol's voice using your own experience and perspective
- Keep the original link to include in the article as a reference (see *Including source links* below)

## Including source links

**Including source links:** If the article is based on a source URL, include the original link naturally in the article body — as "more information", "source", or "for a more detailed look". Place it as a bare URL on its own line where it fits contextually.

For git repositories, link to the repo itself. For articles/docs, link to the original page. Example:

```markdown
I created a demo for this article, check it out:

https://github.com/sadensmol/article-go-gems-1

If you want to dig deeper into this topic, here is a great resource:

https://example.com/original-source
```

Resource links should also appear as bare URLs on their own lines throughout the article where relevant.
