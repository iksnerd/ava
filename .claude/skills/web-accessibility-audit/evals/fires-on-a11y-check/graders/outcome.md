---
type: llm
---
The answer lays out an audit that combines a rule-based pass (Lighthouse or axe
via chrome-devtools MCP) with listening to the page's accessibility tree
through ava's speak_accessibility_tree, rather than only reading a
static report. It reports rule violations and heard problems separately.
