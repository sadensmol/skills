# Follow-up / related tickets created during a task

Read this before you create a ticket from work on another ticket (a follow-up, a deferred review finding, a bug found on the way, a split-off part).

**Follow-up / related tickets created during a task (MUST FOLLOW):** when the
session works on a ticket (the *origin*: the key the user named, or the
`<KEY>-<n>` prefix of the branch) and you create a ticket from that work — a
follow-up, a deferred review finding, a bug found on the way, a split-off part:

1. **Fetch the origin first:**
   `curl "${J[@]}" "$JIRA_BASE_URL/rest/api/2/issue/ORIGIN?fields=summary,issuetype,parent,project"`.
2. **Assign it to the current user.** Take `accountId` from `/rest/api/2/myself`
   and set `"assignee":{"accountId":"<id>"}` in the create body. Never leave it
   unassigned, and never assign it to the origin's assignee instead.
3. **Same parent as the origin.** When the origin has `fields.parent`, set
   `"parent":{"key":"<origin parent key>"}` in the create body, so the new ticket
   sits under the same story/epic. When the origin is itself a sub-task, create
   the new ticket as a sub-task of that same parent. No parent on the origin →
   no parent on the new ticket; never invent one.
4. **Link it to the origin** in the same create body — `Relates` by default,
   `Blocks` when one really blocks the other:
   `"update":{"issuelinks":[{"add":{"type":{"name":"Relates"},"outwardIssue":{"key":"ORIGIN"}}}]}`.
   Link any other ticket the new one depends on or duplicates the same way.
5. **Give the context a reader needs** in the description: the origin key and
   why this ticket split off from it, where the problem is (`repo/path:line`,
   PR, failing test or log line), what was already found, and what "done" means.
   The ticket must make sense to someone who never saw the session.
6. After creating it, re-fetch it and check `assignee`, `parent` and
   `issuelinks`. Then report the new key with its link.

The normal rule still holds: show the JSON body to the user before the POST.
