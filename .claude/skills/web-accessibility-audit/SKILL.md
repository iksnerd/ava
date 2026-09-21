---
name: web-accessibility-audit
description: >
  Audits a web page's accessibility by listening to it, not just reading a
  report: drives chrome-devtools MCP for Chrome's accessibility tree and a
  Lighthouse pass, then narrates that tree through this repo's local Kokoro
  TTS (`local-whisper mcp`), so reading order, unlabeled controls and six
  identical "Read more" links are heard the way a screen-reader user hits
  them. Use for "is this page accessible", WCAG / ARIA / axe / screen-reader
  questions, a failing Lighthouse accessibility score, focus-order or
  keyboard-trap checks, or an a11y regression. Not for performance
  profiling or general browser debugging (browser-testing-with-devtools).
---

# Web accessibility audit

## Why hear a page at all

`lighthouse_audit` runs axe's rules and scores them. It catches missing
labels, poor contrast, bad landmark structure — everything expressible as a
rule over the DOM. It cannot tell you that six cards all announce "Read
more", that a heading outline reads as nonsense out of context, that an alt
text passes the linter and still says `IMG_4021`, or that tabbing lands
somewhere the eye isn't.

That's the gap this skill covers: render the accessibility tree to speech and
listen to the page in the order a screen reader gives it to you.

## Prerequisites

Two MCP servers. Check both are connected *before* driving anything:

- `mcp__chrome-devtools__*` (or `mcp__plugin_<plugin>_chrome-devtools__*`).
  Missing? See `engineering-practices:browser-testing-with-devtools` for the
  connection check and the one-time setup — don't add a second registration.
- `mcp__local-whisper__*`. Missing? Register this repo's server once:

  ```bash
  claude mcp add -s user local-whisper -- local-whisper mcp
  ```

  MCP servers load at session start, so restart the session after adding one.

Speech needs the mlx-engine running (`local-whisper engine start`) and the
menu bar app's global Mute off. Every speaking tool reports when output was
muted rather than claiming success, so read the tool result rather than
assuming the page was heard.

When something is unavailable, don't silently degrade:

| Symptom | Cause | What to do |
|---|---|---|
| `speak` result says "voice output is muted" | menu bar Mute is on | Ask the user to unmute, or run the whole audit with `speak: false` and say the findings are from the text pass only |
| `mlx-engine is not running` / `did not come up` | server down, or auto-start disabled after an explicit Stop | `local-whisper engine start`; first start pays a few seconds of TTS warmup |
| `navigate_page` lands on a login wall | the page needs auth | Ask the user to sign in in the DevTools browser, then re-`take_snapshot` — never audit the login wall as if it were the page |
| The snapshot is thousands of lines | whole-app SPA | Audit one route or one interaction at a time; a reading pass nobody listens to the end of finds nothing |

## The pass

### 1. Baseline

```
navigate_page <url>
lighthouse_audit           # accessibility, SEO, best practices
```

Record the accessibility score and each violation. These are the findings
that already have a rule; everything below is what a rule can't see.

### 2. Hear the page in order

```
take_snapshot
speak_accessibility_tree { snapshot: <paste verbatim>, mode: "reading" }
```

Paste the snapshot exactly as the tool returned it. The result is every
announcement in document order plus findings; the audio is the same thing out
loud. Listen for:

- Announcements that make no sense without the visual layout.
- Content that arrives in a different order than it appears.
- Controls announced only as their role ("button, unlabeled").
- Text a synthesizer mangles: acronyms, IDs, unspaced camelCase.

### 3. Navigate the way screen-reader users do

Most screen-reader users never read a page top to bottom. They jump.

```
speak_accessibility_tree { snapshot, mode: "headings"  }   # is the outline usable alone?
speak_accessibility_tree { snapshot, mode: "links"     }   # do links make sense out of context?
speak_accessibility_tree { snapshot, mode: "landmarks" }   # can you reach the main region directly?
speak_accessibility_tree { snapshot, mode: "forms"     }   # is every field labeled and its state audible?
```

Use `speak: false` for a quiet pass when you only need the text.

### 4. Focus order

```
press_key Tab
take_snapshot     # what has focus now?
```

Repeat through the interactive elements and check:

- The tab order matches the visual order.
- Focus is visible at every stop.
- Nothing traps focus (a modal you can't leave, a widget that swallows Tab).
- Every interactive element is reachable at all — a `div` with a click
  handler and no role never appears.

### 5. Report

Two sections, kept apart because they carry different weight:

- **Rule violations** — from `lighthouse_audit`, with the score. These are
  objective and usually have a mechanical fix.
- **Heard problems** — from steps 2-4, each with the `uid` and what it
  actually announced. Quote the announcement; "button, unlabeled" is more
  persuasive to whoever fixes it than "missing accessible name".

Fix, then re-run steps 1-2 on the same page and diff the announcements.

## Worked example

> "the settings page fails our a11y check, can you see what's wrong"

1. `navigate_page http://localhost:3000/settings`, then `lighthouse_audit` —
   accessibility 84, two violations: a form field with no label, and a
   contrast failure on the secondary button.
2. `take_snapshot`, then `speak_accessibility_tree { snapshot, mode: "reading" }`.
   The audio reaches "edit text, unlabeled" right where the visual form shows
   "API key", and the findings list `uid=3_18: textbox with no accessible
   name` — the same field Lighthouse flagged, now with the uid to fix.
3. `mode: "links"` announces "link, Settings" three times: the sidebar,
   breadcrumb and a tab all announce identically, which Lighthouse scored as
   fine. That's the finding the audit alone would have missed.
4. Report both, separately: two rule violations with the score, plus the
   three indistinguishable links quoted as they were announced.

## Notes

- The narration models role + name + state. It is not VoiceOver: live
  regions, table navigation and the virtual cursor aren't simulated. Findings
  are a floor, not a ceiling — nothing here replaces testing with a real
  screen reader and real users.
- `take_snapshot` reflects the page *now*. Re-snapshot after any interaction;
  a stale tree silently audits a page that no longer exists.
- Slow the narration down (`speed: 0.9`) when the page is dense. The point is
  to notice what's wrong, not to get through it.
