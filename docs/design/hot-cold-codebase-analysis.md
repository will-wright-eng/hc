# Hot/Cold Analysis in Codebases: Files, Directories, and Change Patterns

## Overview

Hot/cold separation in codebases is the application of the fundamental hot/cold principle to source files, modules, and directory structures. Unlike CPU cache optimization, codebase hot/cold analysis operates across two dimensions simultaneously: **how often a file changes** (churn rate) and **how complex or large a file is** (size of the investment at risk). The combination of these dimensions drives prioritization decisions for refactoring, architecture, testing, and ownership.

---

## The Two Dimensions

### Dimension 1: Churn Rate (Hot vs. Cold)

Churn rate measures how frequently a file changes over time, derived from version control history. A high-churn file is hot — it is actively worked on, frequently modified, and represents ongoing operational load for the team. A low-churn file is cold — it is stable, rarely touched, and represents settled code.

Churn rate is the VCS-native proxy for "access frequency" in the CPU cache analogy. The key insight is that **frequent change is a cost**, just as frequent cache misses are a cost. Every time a file is modified, developers must understand it, test it, review it, and reason about its interactions with other files. Hot files impose this cost repeatedly.

### Dimension 2: Complexity (Weight of the Investment)

Complexity — typically measured by cyclomatic complexity, lines of code, or coupling metrics — captures how difficult a file is to work with. A complex file takes longer to understand, is harder to change safely, and is more likely to contain latent bugs.

Complexity alone is not actionable: a complex file that nobody touches is a manageable liability. The risk emerges when complexity and churn combine.

---

## The Hotspot Matrix

Adam Tornhill's methodology (formalized in *Your Code as a Crime Scene* and implemented in CodeScene) produces a 2x2 prioritization matrix:

```
                    LOW CHURN              HIGH CHURN
                ┌──────────────────┬──────────────────────┐
HIGH COMPLEXITY │  Cold Complexity  │  🔥 Critical Hotspot  │
                │  Technical debt,  │  Complex AND actively │
                │  stable liability │  changing — refactor  │
                ├──────────────────┼──────────────────────┤
LOW COMPLEXITY  │  Cold & Fine      │  Hot but Manageable   │
                │  Leave it alone   │  Config, generated,   │
                │                  │  or simple glue code  │
                └──────────────────┴──────────────────────┘
```

The **Critical Hotspot** quadrant is the primary target. These files concentrate both ongoing development activity and structural complexity, making them simultaneously the most expensive to work in and the most likely to produce bugs. Empirical research across Mozilla, Microsoft Windows, the Eclipse IDE, and many open-source projects consistently shows that roughly **80% of bugs originate in 20% of files** — and that 20% maps closely onto the hotspot quadrant.

---

## Methods for Identifying Hot and Cold Files

### Method 1: Commit Frequency Analysis

The most direct measure of file heat is simply counting commits over a time window.

```bash
# Top 20 hottest files by commit count (all time)
git log --format=format: --name-only \
  | sort | uniq -c | sort -rg | head -20

# Restrict to a time window (last 6 months)
git log --since="6 months ago" --format=format: --name-only \
  | sort | uniq -c | sort -rg | head -20
```

This produces a raw ranking of file heat. Files at the top are operationally hot; files at the bottom (or absent) are cold. The time window matters: a file that was hot two years ago but cold now should be treated as cold for current prioritization.

### Method 2: Change Coupling Analysis

Change coupling identifies files that co-change — files that are frequently modified together in the same commit. Highly coupled files that are not co-located in the directory structure, or that have no obvious logical relationship, signal hidden dependencies and implicit architectural coupling.

```bash
# Generate a git log suitable for code-maat analysis
git log --all --numstat --format='%H' > git-log.txt

# Run coupling analysis with code-maat
maat -l git-log.txt -c git2 -a coupling
```

The output ranks file pairs by co-change frequency. Pairs with high coupling but no explicit dependency relationship are candidates for either extraction (making the coupling explicit) or consolidation (merging the files if they truly belong together).

Change coupling is the codebase analog of **false sharing** in CPU cache optimization — two things that have no logical relationship but are co-located, causing one to incur costs when the other changes.

### Method 3: Churn + Complexity Overlay

The hotspot methodology combines churn with static complexity metrics. The workflow is:

1. Extract churn rates from git log (commits per file over a time window)
2. Compute cyclomatic complexity per file using a static analysis tool (`lizard`, `radon` for Python, `gocyclo` for Go, etc.)
3. Normalize both metrics and plot or sort by their product

```bash
# Python example: generate per-file complexity
pip install lizard
lizard src/ --csv > complexity.csv

# Combine with churn data in any data tool
# Files with (churn_rank + complexity_rank) highest = hotspots
```

### Method 4: Author Concentration Analysis

A related hot/cold signal is **author concentration**: how many developers have touched a file. A file touched by only one author is fragile (knowledge is concentrated) and becomes a cold risk — if that author leaves, the file becomes effectively unmaintainable. A file touched by dozens of authors is hot in terms of coordination overhead.

```bash
# Count unique authors per file
git log --format="%ae" -- path/to/file | sort -u | wc -l

# Files with only 1 author = knowledge silos
git log --format=format: --name-only \
  | sort | uniq -c | sort -g | head -20  # cold by author diversity
```

### Method 5: Test Coverage as a Cold Proxy

Files with low or zero test coverage that also have high churn rates are a specific and dangerous pattern. The file is hot (being changed frequently) but lacks the safety net that would make hot changes safe. This is detectable by overlaying coverage reports with churn data.

---

## Directory Structure as a Hot/Cold Architecture Tool

### Co-location of Hot Neighbors

Files that exhibit high change coupling should be co-located in the directory structure. If `auth_service.go` and `token_validator.go` co-change in 80% of commits, they belong in the same package. Separation across packages or modules does not reduce coupling — it hides it while adding interface overhead.

The principle: **directory structure should reflect actual change patterns, not just logical taxonomy.**

### Isolation of Cold Foundational Code

Cold files — stable, foundational libraries, interfaces, or shared utilities — should be structurally isolated from hot files. The goal is to ensure that activity in hot zones does not force changes to cold zones.

This is the practical implementation of dependency direction rules from Clean Architecture and Hexagonal Architecture:

```
project/
├── domain/          ← hot zone: business logic, changes frequently
│   ├── orders/
│   └── pricing/
├── application/     ← warm zone: orchestration, changes moderately
│   └── services/
└── infrastructure/  ← cold zone: DB drivers, HTTP handlers, rarely changes
    ├── postgres/
    └── http/
```

The rule is that cold zones depend on hot zones (via interfaces), never the reverse. This ensures that a change to a hot domain file never propagates into cold infrastructure code. Violations of this direction — cold files that change because hot files changed — are signals that the dependency structure has inverted.

### Monorepo Hot Zone Governance

In large monorepos, hot directories warrant additional governance mechanisms:

- **Stricter CODEOWNERS rules** on high-churn directories to ensure review quality keeps pace with change velocity
- **Mandatory test coverage thresholds** on hot packages, enforced in CI
- **Change impact analysis** that warns when a commit touches a cold foundational package — this is a danger signal that should require additional scrutiny
- **Blast radius visualization** showing how many downstream packages are affected by a change to a given file

---

## Tooling Landscape

| Tool | What It Measures | Output |
|---|---|---|
| `git log` + shell | Raw churn rate | File rankings by commit count |
| `code-maat` | Change coupling, churn, author analysis | CSV reports, coupling maps |
| CodeScene | Churn + complexity hotspots, team patterns | Visual dashboards (commercial) |
| CodeClimate | Complexity + churn combined score | Per-file technical debt ratings |
| `lizard` | Cyclomatic complexity, function length | CSV/JSON complexity per file |
| `gitleaks` / `git-truck` | Ownership, contribution patterns | Visual ownership maps |
| SonarQube | Static complexity + historical trends | Web dashboard with history |

For a zero-dependency starting point, the following produces an immediately actionable hotspot list:

```bash
#!/bin/bash
# hotspots.sh — find your top 20 hottest files in the last year

echo "=== File churn (commits in last 12 months) ==="
git log --since="12 months ago" \
        --format=format: \
        --name-only \
  | grep -v '^$' \
  | sort \
  | uniq -c \
  | sort -rn \
  | head -20

echo ""
echo "=== Change coupling (top co-changing pairs) ==="
# Requires code-maat: https://github.com/adamtornhill/code-maat
# git log --all --numstat --format='%H' > git.log
# maat -l git.log -c git2 -a coupling | head -20
```

---

## Practical Prioritization Framework

Given the analysis outputs, the recommended approach to acting on hot/cold codebase data:

**Immediate action (Critical Hotspots):** Files in the high-churn + high-complexity quadrant. These should be the primary targets of any refactoring investment. The goal is not to eliminate complexity wholesale but to reduce it enough that the ongoing churn cost decreases. Extract sub-components, reduce coupling, increase test coverage.

**Monitor but defer (Hot + Low Complexity):** Files that change frequently but are structurally simple are functioning as intended. Watch for complexity growth, but do not refactor preemptively.

**Document and protect (Cold + High Complexity):** Complex files that rarely change are stable liabilities. The risk is that when they do need to change, nobody understands them. The appropriate response is documentation and test coverage — not refactoring, which risks destabilizing something that is currently working.

**Ignore (Cold + Low Complexity):** These files require no attention. Spending engineering time on cold, simple files is pure waste.

---

## The Underlying Insight

The most important reframe this methodology offers is moving from **static analysis** to **dynamic prioritization**. Static analysis tools produce lists of everything wrong with a codebase — every complexity violation, every code smell, every potential issue. These lists are so long they become useless.

Churn rate converts a static list into a prioritized queue. It answers the question: *of all the things that are wrong, which ones are actively costing us right now?*

A complex file that nobody touches is not costing you anything today. A complex file that your entire team touches every week is costing you in every sprint: slower development, higher defect rates, more difficult onboarding, harder code review. The churn signal surfaces exactly this operational cost in a way that no static metric can.

This is the direct codebase analog of a CPU profiler: it does not tell you which functions are slow in theory, it tells you which functions are slow *and* called frequently enough that optimizing them will actually matter.

---

## References

### Foundational Methodology

- Tornhill, A. (2015). *Your Code as a Crime Scene: Use Investigative Techniques to Arrest Defects, Bottlenecks, and Bad Design in Your Programs*. Pragmatic Bookshelf. — The primary text establishing the churn × complexity hotspot methodology.
- Tornhill, A. (2018). *Software Design X-Rays: Fix Technical Debt with Behavioral Code Analysis*. Pragmatic Bookshelf. — Extends the methodology to team patterns, change coupling, and organizational analysis.
- Tornhill, A. (2013). Code-maat. GitHub. <https://github.com/adamtornhill/code-maat> — Open-source tool implementing git log mining for coupling, churn, and author analysis.

### Defect Prediction & Empirical Software Engineering

- Nagappan, N., & Ball, T. (2005). Use of relative code churn measures to predict system defect density. *Proceedings of the 27th International Conference on Software Engineering (ICSE)*, 284–292. — Microsoft Research study demonstrating the correlation between churn rate and defect density.
- Moser, R., Pedrycz, W., & Succi, G. (2008). A comparative analysis of the efficiency of change metrics and static code attributes for defect prediction. *Proceedings of ICSE*, 181–190. — Shows change metrics (churn) outperform static metrics alone for bug prediction.
- Hassan, A. E. (2009). Predicting faults using the complexity of code changes. *Proceedings of ICSE*, 78–88. — Establishes complexity of change history as a predictor of fault-prone files.
- Kim, S., et al. (2007). Predicting faults from cached history. *Proceedings of ICSE*, 489–498. — Demonstrates the 80/20 defect distribution across files in large codebases.
- Zimmermann, T., & Nagappan, N. (2008). Predicting defects using network analysis on dependency graphs. *Proceedings of ICSE*, 531–540. — Extends defect prediction to structural coupling between files.

### Mining Software Repositories

- Hassan, A. E., & Holt, R. C. (2004). The top ten list: Dynamic fault prediction. *Proceedings of the 20th IEEE International Conference on Software Maintenance (ICSM)*. — Early work on using VCS history to identify fault-prone files.
- Bird, C., et al. (2009). Putting it all together: Using socio-technical networks to predict failures. *Proceedings of the 20th International Symposium on Software Reliability Engineering (ISSRE)*. — Combines social (author) and technical (change) networks for defect prediction.
- Kagdi, H., Collard, M. L., & Maletic, J. I. (2007). A survey and taxonomy of approaches for mining software repositories in the context of software evolution. *Journal of Software Maintenance and Evolution, 19*(2), 77–131. — Comprehensive survey of MSR techniques including change analysis.

### Software Architecture & Dependency Management

- Martin, R. C. (2017). *Clean Architecture: A Craftsman's Guide to Software Structure and Design*. Prentice Hall. — Defines dependency direction rules that formalize hot/cold zone isolation.
- Cockburn, A. (2005). Hexagonal architecture. <https://alistair.cockburn.us/hexagonal-architecture/> — Original description of ports-and-adapters pattern as a mechanism for isolating cold infrastructure from hot domain logic.
- Evans, E. (2003). *Domain-Driven Design: Tackling Complexity in the Heart of Software*. Addison-Wesley. — Foundational text on structuring code around domain concepts, directly applicable to hot zone identification.
- Parnas, D. L. (1972). On the criteria to be used in decomposing systems into modules. *Communications of the ACM, 15*(12), 1053–1058. — Classic paper on information hiding; the principle of isolating likely-to-change (hot) decisions maps directly onto hot/cold separation.

### Tooling & Static Analysis

- Campbell, G. A., & Papapetrou, P. P. (2013). *SonarQube in Action*. Manning Publications. — Covers complexity metrics and historical trend tracking in SonarQube.
- Feathers, M. (2004). *Working Effectively with Legacy Code*. Prentice Hall. — Practical techniques for safely modifying hot, complex legacy files.
- Kerievsky, J. (2004). *Refactoring to Patterns*. Addison-Wesley. — Refactoring strategies applicable to critical hotspot remediation.

### Build Systems & Incremental Compilation

- Bazel Authors. (2020). Bazel build system documentation. <https://bazel.build/docs> — Describes content-addressed caching and dependency graph analysis underlying incremental build hot/cold optimization.
- Taye, N. (2022). Turborepo handbook. <https://turbo.build/repo/docs> — Documents affected-file analysis and remote caching in monorepo build systems.
