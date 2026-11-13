# Moving code between files

Read this when you relocate code, a test or a doc section from one file to another.

When relocating code (a test into an existing suite, a helper into another
package, a section into another doc), the order is: **write the destination,
verify it compiles and carries every piece, and only then delete the source.**
Never delete first and reconstruct the moved content from memory — what is
missing is exactly what you fail to remember, and with an untracked source there
is no diff to check yourself against afterwards.

Verify by comparing the source's contents against the destination item by item
(each test case, each assertion, each function), not by an overall impression.
