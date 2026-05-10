# Open Source Tooling Landscape: Hot/Cold Codebase Analysis

## Overview

This document summarizes available open source tools for performing hot/cold codebase analysis across three dimensions: **churn rate** (file change frequency from VCS history), **cyclomatic complexity** (structural difficulty of code), and **change coupling** (files that co-change in the same commits).

The ecosystem is fragmented. Most tools in this space were built between 2015–2019 in response to Adam Tornhill's methodology and have since gone dormant. No single actively-maintained tool covers all three analyses.

---

## All-in-One Tools

### code-maat

- **Repo:** github.com/adamtornhill/code-maat
- **Analyses:** Churn, change coupling, author analysis. Complexity requires piping in output from an external tool (e.g., lizard).
- **Language agnostic:** Yes — operates on git log output, not source code.
- **Invocation:** CLI (Java JAR)
- **Status:** Unmaintained since ~2020. ~2,000 stars. Still the reference implementation of Tornhill's behavioral code analysis methodology.

### code-forensics

- **Repo:** github.com/smontanari/code-forensics
- **Analyses:** Churn, change coupling, hotspot analysis (churn × complexity). Best-in-class interactive browser visualizations.
- **Language agnostic:** Mostly — churn and coupling are VCS-based; complexity requires an external tool.
- **Invocation:** CLI via Gulp tasks
- **Status:** Dormant since ~2019. ~400 stars.

### charlie-git

- **Package:** npmjs.com/package/charlie-git
- **Analyses:** Churn, complexity (indentation-based heuristic), change coupling, hotspot scoring. All three in one tool.
- **Language agnostic:** Yes — indentation-based complexity works on any language.
- **Invocation:** CLI; outputs HTML report.
- **Status:** Active as of 2024/2025. Most self-contained option available.

### Emerge

- **Repo:** github.com/glato/emerge
- **Analyses:** Churn, complexity, structural dependency graphs, change coupling (experimental).
- **Language agnostic:** Broad multi-language support; git metrics are language-agnostic.
- **Invocation:** CLI; browser-based graph visualizations.
- **Status:** Active ~2024. Best for structural/dependency visualization alongside git metrics.

---

## Best Single-Purpose Tools

| Tool | Analysis | Languages | Status |
|---|---|---|---|
| **lizard** (github.com/terryyin/lizard) | Cyclomatic complexity | 17+ languages | Actively maintained, ~2k stars |
| **scc** (github.com/boyter/scc) | Complexity + SLOC | 239 languages | Very active, ~5k stars, extremely fast |
| **hercules** (github.com/src-d/hercules) | Git burndown, code age, ownership | Language-agnostic | src-d shut down 2019; community maintained |

---

## The Standard Practitioner Pipeline

Most practitioners following Tornhill's methodology assemble a manual pipeline:

```
git log  →  code-maat  (churn + coupling)
                +
            lizard      (cyclomatic complexity per file)
                ↓
            manual join or code-forensics (visualization)
```

This works but requires multiple tools, manual data joining, and Java for code-maat.

---

## Gap in the Ecosystem

No actively-maintained, language-agnostic CLI exists that unifies all three analyses (churn + complexity + coupling) with clean, composable output. charlie-git comes closest but uses an indentation heuristic rather than true cyclomatic complexity. This represents the clearest opportunity for a new tool in this space.

---

## References

- Tornhill, A. (2015). *Your Code as a Crime Scene*. Pragmatic Bookshelf.
- Tornhill, A. (2018). *Software Design X-Rays*. Pragmatic Bookshelf.
- github.com/adamtornhill/code-maat
- github.com/smontanari/code-forensics
- github.com/terryyin/lizard
- github.com/boyter/scc
- github.com/src-d/hercules
- github.com/glato/emerge
- npmjs.com/package/charlie-git
