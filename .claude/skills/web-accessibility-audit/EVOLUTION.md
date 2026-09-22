# EVOLUTION

## 2026-09-21 — created

Written alongside `ava mcp`'s `speak_accessibility_tree` tool, which
is what makes the skill possible: the accessibility tree is rendered to
screen-reader announcements by tested Go (`internal/a11y`), not by prose the
model re-improvises each session.

Scored twice in the same pass while drafting:

- First draft — `rubric v2: 80/100, Ready (unverified)`. Lost points on A2/A3
  (the description named the domain but few of the phrases a user actually
  types), C3 (no failure handling) and E (tool-call fragments, no worked
  example). F 0: no eval cases.
- Shipped — `rubric v2: 80 → 89 (A +4, C +2, E +2, F +4)`. Added WCAG / ARIA /
  axe / "is this page accessible" triggers and a second boundary against
  performance profiling; a symptom→cause→fix table for muted output, a down
  engine, a login wall and oversized snapshots; a worked example; and five
  trigger eval cases (3 should-fire, 2 near-miss).

Capped at 89 and marked **unverified**: the eval suite exists but has never
been run. Running `claude plugin eval` against it is the next thing that moves
this score.
