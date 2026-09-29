# Agent Orchestrator — Product Evolution Plan

> Architecture and roadmap for the **Product Layer** that sits on top of the
> existing **Execution Engine**. This is a planning document, not an
> implementation. Nothing described as `PLANNED`, `CONCEPT` or `OPEN QUESTION`
> below exists yet.

## Table of Contents

1. [Document Control](#1-document-control)
2. [Executive Summary](#2-executive-summary)
3. [Product Vision](#3-product-vision)
4. [Product Inputs](#4-product-inputs)
5. [Execution Engine Today (R1-R7)](#5-execution-engine-today-r1-r7)
6. [Roadmap P1-P7](#6-roadmap-p1-p7)
7. [Domain Model Analysis](#7-domain-model-analysis)
8. [Two Kanbans](#8-two-kanbans)
9. [Agent and System Generated Intake](#9-agent-and-system-generated-intake)
10. [Plan to WorkItem Generation](#10-plan-to-workitem-generation)
11. [Traceability](#11-traceability)
12. [Human Gates](#12-human-gates)
13. [Open Architectural Questions](#13-open-architectural-questions)
14. [Implementation Strategy](#14-implementation-strategy)
15. [Success Criteria Check](#15-success-criteria-check)
16. [Non-Goals and Restrictions](#16-non-goals-and-restrictions)

---

## 1. Document Control

| Field | Value |
| --- | --- |
| Document | `docs/product/PRODUCT_EVOLUTION_PLAN.md` |
| Purpose | Durable, versioned architecture + roadmap for the Product Layer |
| Baseline | Execution Engine **COMPLETE** (R1-R7 implemented and deployed) |
| Baseline SHA | `9446665` (local `main` == `origin/main`) |
| Audience | Product, developers, operators, agents executing future phases |
| Nature | Planning only. No product entity, migration, API or UI is implemented here |
| Source of truth | GitHub `main` |

### Status vocabulary used throughout

- `[IMPLEMENTED]` — exists in the codebase today at the baseline SHA.
- `[PLANNED]` — agreed direction on the roadmap; not built yet.
- `[CONCEPT]` — a candidate design to be validated before planning.
- `[OPEN QUESTION]` — a decision that requires product/human input. Not resolved here.

### Current system snapshots (as of baseline `9446665`)

- **Domain entities (persisted):** `WorkItem`, `Session`, `Gate`, `Approval`,
  `Event`, plus value objects `Plan` (agent implementation plan), `GatePayload`,
  `SessionOutput`, `Evidence`. `[IMPLEMENTED]`
- **Database migrations:** `001_initial.sql`, `002_workitem_base_commit.sql`,
  `003_parallel_workitems.sql`, `004_workitem_capability.sql`. `[IMPLEMENTED]`
- **HTTP API:** `/healthz`, `/api/workitems` (+ `/events`, `/sessions`, `/gate`,
  `/approve`, `/reject`, `/request-changes`, `/retry`), `/api/projects`,
  `/api/diagnostics`. `[IMPLEMENTED]`
- **No product entities, product API or product UI exist yet.** The words
  Intake, Bug, UserStory, Task, ProductCard, Release and Feedback below refer
  only to candidate/product concepts, not to code.

---

## 2. Executive Summary

The Agent Orchestrator already contains a working **Execution Engine**: it takes
a `WorkItem`, drives it through Discovery → Decision → Human Gate →
Implementation → Validation → Complete, produces objective evidence, and is
governed by policies and diagnostics (`R1-R7`). `[IMPLEMENTED]`

What is missing is the layer **above** it: how work is *discovered, described,
prioritised, planned and released*. Today a human (or an API caller) must decide
by hand what a `WorkItem` is. There is no first-class notion of "a user reported
a bug", "this is a user story", "this plan bundles three pieces of work", "this
was released to the user who asked", or "this regressed".

This plan defines a **Product Layer** whose job is to answer **what** should be
built or fixed, while the Execution Engine continues to answer **how** a single
unit of engineering work is executed. The two layers are deliberately separate:

- **Product Layer (new):** Intake → Triage/Refinement → Bug/UserStory/Task →
  Plan → Product Kanban → WorkItems → Feedback/Release.
- **Execution Engine (existing R1-R7):** WorkItem → phases → evidence.

The roadmap is `P1 → P7`: Product Intake, Work/Bug/Story representation,
Planning, Product Kanban, Plan→WorkItem generation, Plan-level human gates, and
the Feedback/Release loop.

The guiding principles are: **build on R1-R7, do not duplicate it**; **persist
only what must be persisted** (not every noun becomes a table); **make
traceability first-class**; and **keep human/product policy in control of what
becomes committed work** (agents and the system may *propose*, humans decide).

---

## 3. Product Vision

Target conceptual flow (all boxes except the Execution Engine are `PLANNED`/`CONCEPT`):

```
USER / AGENT / SYSTEM / DEVELOPER / OPERATOR
                     |
                     v
               PRODUCT INTAKE
                     |
                     v
           TRIAGE / REFINEMENT
                     |
                     v
        BUG / USER STORY / TASK
                     |
                     v
                   PLAN
                     |
                     v
             PRODUCT KANBAN
                     |
                     v
               WORK ITEMS
                     |
                     v
          EXECUTION ENGINE (R1-R7)
          [IMPLEMENTED]
                     |
                     v
             EVIDENCE / RELEASE
                     |
                     v
              USER FEEDBACK
                     |
                     +-------------> PRODUCT INTAKE  (closed loop)
```

### The core separation

- **Product layer = WHAT should be built or fixed.** It owns intake, problem
  framing, prioritisation, planning and release communication.
- **Execution layer = HOW engineering work is executed.** It owns worktrees,
  OpenCode sessions, phases, gates, evidence and policies.

`R1-R7` are the Execution Engine in its current form. The Product Layer must be
constructed **on top of** them, reusing their guarantees:

| Execution Engine guarantee | Product Layer use |
| --- | --- |
| Objective evidence + validation (R4/R6) | Release decisions reference real evidence, never agent confidence |
| Human Gate + authorization | Natural attachment point for Plan approval and WorkItem approval (P6) |
| Policies/guardrails (R6) | Product policy can be expressed as execution guardrails |
| Concurrency + parallel WorkItems (R5) | A Plan may fan out into independent WorkItems |
| Capabilities / multi-agent (R7) | A PlanItem may request a specific capability |
| Diagnostics (R3) | Can propose System-originated Intake candidates |
| Reconciliation + GC (R1/R2) | Unchanged; product layer must not bypass them |

### Explicit anti-goals of the Product Layer

- It does **not** re-implement execution, worktrees, sessions or evidence.
- It does **not** decide engineering correctness; objective evidence does.
- It does **not** merge the product and engineering lifecycles (see §8).

---

## 4. Product Inputs

Many actors, one funnel. All of the following are `PLANNED`/`CONCEPT` as
*first-class intake sources*; today they are informal (a human writes a
WorkItem by hand).

| Source | Example inputs | Notes |
| --- | --- | --- |
| **USER** | Bug / failure report, improvement request, suggestion, new need, feedback | Highest volume; often lowest signal-to-noise. Needs triage. |
| **AGENT** | Technical problem, regression, code inconsistency, technical debt, suggested improvement | Produced while executing or reviewing. Must not self-commit (see §9). |
| **SYSTEM** | Monitoring finding, diagnostic finding (`R3`), operational failure, security/validation finding | Machine-generated, objective, evidence-backed. |
| **DEVELOPER / OPERATOR** | Engineering task, maintenance, upgrade, operational request | Already closest to a WorkItem; the "escape hatch" path. |

All sources converge **initially** on **Product Intake**. Intake is the single
entry point so that prioritisation, deduplication and traceability happen in one
place rather than in four disconnected channels.

The Product Layer must preserve **provenance**: which actor, which channel, and
(where applicable) which WorkItem/Session/Diagnostic produced the input, because
traceability (§11) and the feedback loop (§P7) depend on it.

---

## 5. Execution Engine Today (R1-R7)

Everything in this section is `[IMPLEMENTED]` at baseline `9446665`. The Product
Layer must treat these as stable contracts.

- **R1 — Startup reconciliation (fail-model).** On boot, sessions orphaned by a
  crash/restart are marked failed; affected WorkItems do not silently resume.
  A single-driver discipline prevents one WorkItem being driven twice.
- **R2 — Worktree lifecycle & GC.** Worktrees are ownership-classified;
  only leftovers of completed WorkItems are removed; active/unknown are kept.
- **R3 — Self-diagnostics.** `GET /api/diagnostics` returns read-only findings
  (database, worker/runtime, workitem/session consistency, worktrees) plus
  orchestrator identity and reported capabilities.
- **R4 — Runtime interface + robust process control.** `domain.Runtime`
  abstraction over OpenCode; bounded timeouts, cancellation, process-group kill,
  no orphaned processes, typed errors.
- **R5 — Parallel WorkItems.** A configurable `max_active_per_project`
  (global and per project) allows independent WorkItems to execute concurrently;
  creation is serialised so the limit cannot be overshot.
- **R6 — Policies & guardrails.** Per-project `allowed_agents`,
  `protected_paths`, `require_runtime`, `require_validation`,
  `max_active_per_project`, and global `require_registered_project`; rejections
  are audited as `policy.rejected`; policy only rejects, never auto-corrects.
- **R7 — Per-WorkItem capability (multi-agent).** A `WorkItem` may name an
  OpenCode agent profile (`capability`) that overrides per-phase agents for
  every phase; `allowed_agents` is enforced; capabilities are surfaced in
  diagnostics.

**Key boundary:** the Product Layer produces and links `WorkItem`s; it does not
open worktrees, run agents, collect evidence or resolve human gates. Those stay
entirely within R1-R7.

---

## 6. Roadmap P1-P7

Each phase below carries the same template: objective; problem solved;
entities; dependencies; relationship to R1-R7; explicitly out of scope;
possible human gates; preliminary completion criteria; risks/open doubts.

### P1 — Product Intake

- **Objective.** Provide one place where any actor's raw need/finding can be
  captured, attributed and queued for triage.
- **Problem solved.** Work currently starts as an ad-hoc `WorkItem`; there is no
  record of *why* it exists, *who* asked, or *what happened* to it.
- **Entities involved.** `Intake` (candidate). Source actor, channel, raw text,
  attachments, provenance, state (`new/triaged/accepted/rejected/duplicate`).
- **Dependencies.** None on product phases. Consumes `R3` diagnostics for
  System-originated candidates (see §9).
- **Relationship to R1-R7.** None at runtime. Intake is upstream of WorkItems;
  it must never mutate `WorkItem` state.
- **Explicitly out of scope.** Turning intake into work; prioritisation;
  deduplication; UI beyond a capture endpoint.
- **Possible human gates.** Intake acceptance (Gate #1, §12).
- **Preliminary completion criteria.** Any actor can submit intake; every intake
  has provenance and an immutable audit record; list/read API exists;
  acceptance/rejection is recorded.
- **Risks / open doubts.** Spam/noise volume; attachments storage; identity and
  permissions (see §13); whether Intake and the backlog are one entity.

### P2 — User Stories / Bugs / Tasks

- **Objective.** Represent committed work items at the product level as a
  typed, describable unit with acceptance framing.
- **Problem solved.** `WorkItem` mixes "what/why" with execution; product work
  needs its own record with type, acceptance criteria and priority.
- **Entities involved.** `Bug`, `UserStory`, `Task` — either three entities or
  one `WorkEntity` with a type discriminator (`CONCEPT`, see §7 and §13).
- **Dependencies.** P1 (a story/bug typically originates from accepted intake).
- **Relationship to R1-R7.** One product work entity may later produce one or
  more `WorkItem`s (P5). R1-R7 unchanged.
- **Explicitly out of scope.** Planning, Kanban, execution.
- **Possible human gates.** Story refinement/acceptance (Gate #2, §12).
- **Preliminary completion criteria.** A typed work entity exists with
  acceptance criteria and priority; it can reference its source intake;
  traceability from intake is navigable.
- **Risks / open doubts.** Entity-vs-type modelling; who sets priority/impact;
  duplication detection (§13).

### P3 — Planning

- **Objective.** Group product work into an approved `Plan` that expresses
  intent, sequence and (optionally) dependencies before execution.
- **Problem solved.** Today there is no durable artifact between "we agreed to
  do X" and "a WorkItem exists".
- **Entities involved.** `Plan`, `PlanItem`/`ProductCard` (candidate).
- **Dependencies.** P2.
- **Relationship to R1-R7.** A `Plan` is the source for one or more `WorkItem`s.
  It may declare dependencies between future WorkItems; R5 parallelism remains
  the executor of independent ones. **R5 is not modified.**
- **Explicitly out of scope.** Automatically generating WorkItems (that is P5);
  cross-team capacity planning.
- **Possible human gates.** Plan approval (Gate #3, §12).
- **Preliminary completion criteria.** A plan can be created from product work,
  reviewed, approved, and its items traced back to intake/stories.
- **Risks / open doubts.** Naming collision with the existing agent `Plan`
  (`domain.Plan`); how much structure is mandatory vs free-form.

### P4 — Product Kanban

- **Objective.** Give product work a board that reflects the *product* lifecycle
  (Inbox → … → Released), distinct from the engineering phase board.
- **Problem solved.** Operators need to see what is waiting for triage/refinement
  vs what engineering is doing.
- **Entities involved.** Derived views over `Intake`/work entities/`Plan`; a
  `ProductCard` may be a projection rather than a table (`CONCEPT`).
- **Dependencies.** P1-P3.
- **Relationship to R1-R7.** It reads execution status (e.g. "in development"
  when a linked WorkItem is in Implementation) but owns no execution state.
- **Explicitly out of scope.** Merging the two Kanbans; drag-and-drop workflow
  engine.
- **Possible human gates.** None directly; the column transitions may be gated
  by P1-P3/P6 decisions.
- **Preliminary completion criteria.** Product columns reflect the product
  lifecycle; cards link to their stories/plans/WorkItems; read-only status is
  derived from the Execution Engine.
- **Risks / open doubts.** Duplicating state instead of deriving it; ambiguity
  at the Product↔Engineering boundary (§8).

### P5 — Plan to WorkItem Generation

- **Objective.** Turn an approved `Plan` into one or more `WorkItem`s, preserving
  traceability and declaring dependencies.
- **Problem solved.** Manual WorkItem creation loses the plan context and cannot
  express "A before B".
- **Entities involved.** `PlanItem` → `WorkItem` linkage (new trace link).
- **Dependencies.** P3.
- **Relationship to R1-R7.** This is the **integration seam**. Generation must go
  through the existing `CreateWorkItem` path (thus inheriting R5 concurrency,
  R6 policy, R7 capability). Dependencies are advisory metadata for now; R5
  keeps executing independent WorkItems in parallel.
- **Explicitly out of scope.** A dependency scheduler/queue (would change R5);
  auto-generation without approval.
- **Possible human gates.** WorkItem implementation approval (Gate #4, §12).
- **Preliminary completion criteria.** An approved plan can produce N WorkItems,
  each linked back to its `PlanItem`, with capability/policy respected.
- **Risks / open doubts.** Dependency semantics (blocking vs advisory); what
  happens when a linked WorkItem fails/regresses.

### P6 — Plan-level Human Gates

- **Objective.** Formalise human decision points at the product boundary (plan
  approval, implementation approval), reusing the evidence and gate machinery.
- **Problem solved.** Today only the per-WorkItem engineering gate exists; there
  is no product-level approval.
- **Entities involved.** Product-level gate/approval records (candidate),
  possibly reusing the `Gate`/`Approval` vocabulary.
- **Dependencies.** P3, P5.
- **Relationship to R1-R7.** Extends the existing gate concept to the product
  boundary. The engineering gate (Decision → Awaiting Approval → Implementation)
  remains unchanged; product gates are **additional**, upstream.
- **Explicitly out of scope.** Mutating the existing gate state machine.
- **Possible human gates.** All of #1-#5 (§12) — some mandatory, some configurable.
- **Preliminary completion criteria.** Plan approval and WorkItem approval are
  recordable, auditable and block progression until resolved.
- **Risks / open doubts.** Which gates are mandatory vs configurable; who is an
  approver; authorization model.

### P7 — Feedback / Release Loop

- **Objective.** Close the loop: what was released, to whom, and what the
  originating user/actor reported afterwards (including regressions/reopens).
- **Problem solved.** There is no record connecting a shipped change to the
  person who asked, so feedback and regressions vanish.
- **Entities involved.** `Release`, `Feedback` (candidates); linkage back to
  `Intake`/work entity/`WorkItem`/evidence.
- **Dependencies.** P5, P6; relies on Execution Engine evidence for what was
  actually shipped.
- **Relationship to R1-R7.** Release references **objective evidence** (R4/R6)
  and the WorkItem's commit/diff; it does not re-run execution.
- **Explicitly out of scope.** Deployment automation; notification channels
  beyond a recorded feedback link.
- **Possible human gates.** Release approval (Gate #5, §12).
- **Preliminary completion criteria.** A release can be recorded against a
  WorkItem with evidence; feedback can be attached to the originating intake;
  regressions can reopen the loop.
- **Risks / open doubts.** How to notify the original user; reopen vs new
  intake; feedback metrics (§13).

---

## 7. Domain Model Analysis

Candidate model (nothing here is implemented). For each noun we decide whether
it likely needs to be **persistent** (own lifecycle/identity), **derived**
(projection/reference), or is a **type** rather than an entity.

| Concept | Likely form | Lifecycle | Rationale |
| --- | --- | --- | --- |
| `Intake` | **Persistent** `[CONCEPT]` | new → triaged → accepted/rejected/duplicate/closed | Needs identity, provenance, audit; exists before any work. |
| `Incident` | **Type or derived** `[CONCEPT]` | — | Likely a severity/classification of a bug or operational intake, not a separate table. |
| `Bug` | **Persistent or typed** `[CONCEPT]` | open → in progress → resolved → verified/closed | Long-lived product work. Could be a type on one work entity. |
| `UserStory` | **Persistent or typed** `[CONCEPT]` | draft → refined → ready → … | Same considerations as Bug. |
| `Task` | **Persistent or typed** `[CONCEPT]` | open → done | Smallest product work unit; may overlap heavily with `WorkItem`. |
| `Plan` | **Persistent** `[PLANNED]` | draft → approved → active → done/cancelled | The approval artifact; needs identity and versioning. **Naming collision** with agent `Plan`. |
| `PlanItem` | **Persistent** `[CONCEPT]` | within a plan | The linkage unit that becomes a WorkItem; carries ordering/dependencies. |
| `ProductCard` | **Derived (probably)** `[CONCEPT]` | — | A Kanban projection of a work entity/plan item; likely not its own table. |
| `WorkItem` | **Persistent — EXISTS** `[IMPLEMENTED]` | Discovery → … → Complete/Failed/Blocked | Owned by the Execution Engine. **Do not modify in P1-P7.** |
| `Session` | **Persistent — EXISTS** `[IMPLEMENTED]` | running → completed/failed | One OpenCode execution per phase. |
| `Evidence` | **Persistent + value objects — EXISTS** `[IMPLEMENTED]` | immutable | `SessionOutput`, `CommandResult`, git evidence. |
| `Release` | **Persistent (provisional)** `[CONCEPT]` | drafted → released | Depends on what "release" means per project (see §13). |
| `Feedback` | **Persistent (provisional)** `[CONCEPT]` | new → triaged | Closes the loop; may itself become Intake. |

**Guiding rule:** do not assume every noun needs a table. Derive
`ProductCard`; model `Incident`/`Task` as types unless proven otherwise.

### Preliminary relationships and cardinalities (`CONCEPT`)

```
Intake (1) ──── produces ────▶ (0..1) Bug/UserStory/Task      [many intake may merge into one work entity]
WorkEntity (1) ─── bundled into ───▶ (0..*) Plan              [a plan covers many work entities]
Plan (1) ─── contains ───▶ (1..*) PlanItem                    [a plan has at least one item]
PlanItem (1) ─── generates ───▶ (0..1) WorkItem               [not every plan item is executed immediately]
WorkEntity (*) ◀── traced by ───▶ (*) WorkItem                [many-to-many over time / over plans]
WorkItem (1) ─── produces ───▶ (1..*) Session                 [EXISTS]
Session (1) ─── produces ───▶ (0..*) Evidence                 [EXISTS]
WorkItem (*) ─── included in ───▶ (*) Release                 [a release ships many WorkItems]
Release (1) ─── elicits ───▶ (0..*) Feedback                  [feedback points back to Release]
Feedback (*) ─── may reopen ───▶ (0..1) Intake/WorkEntity     [regression loop]
```

All cardinalities are **preliminary** and must be validated per phase.

### Key modelling cautions

- **`Plan` name collision.** `domain.Plan` already means the *agent-proposed
  implementation plan* shown at the engineering gate. The product `Plan` is a
  different concept. Resolve naming before P3 (see §13).
- **`WorkItem` is frozen.** P1-P7 must not change the `WorkItem` domain model or
  schema. All product concepts are separate entities that *reference* WorkItems.
- **Traceability links are first-class**, not inferred from text. See §11.

---

## 8. Two Kanbans

The system has two distinct lifecycles. They must **not** be merged:
`[CONCEPT]`

### Product Kanban

```
Inbox → Triage → Refinement → Ready → Planned → In Development → Validation → Released
```

Answers **"What should we build or fix?"** Columns describe product intent and
readiness. Owned by the Product Layer.

### Engineering Kanban

```
Discovery → Decision → Human Gate → Implementation → Validation → Complete
```

Answers **"How is engineering work being executed?"** These are the actual
`WorkItem` phases (`[IMPLEMENTED]`). Owned by the Execution Engine.

### Why they must stay separate

1. **Different owners and cadence.** Product triage/refinement is human-driven
   and can take days; engineering phases are machine-driven and bounded.
2. **Different units.** One product card may map to many WorkItems (and vice
   versa over time); forcing a 1:1 column mapping would be wrong.
3. **Different truth.** Engineering status is objective (`CurrentPhase`,
   sessions, evidence). Product status is a human decision (accepted? ready?).
   Mixing them lets opinion masquerade as execution fact.
4. **Different gates.** Product gates (plan approval, release approval) are not
   the engineering Human Gate; conflating them would weaken both.

**Bridge:** the Product Kanban shows a card as *In Development* / *Validation*
by **deriving** the status of its linked WorkItem(s). It reads execution state;
it never writes it.

---

## 9. Agent and System Generated Intake

Question: can agents and `R3 Diagnostics` propose Intake? `[CONCEPT]`

**Initial principle (proposed, not yet policy):** *Agents and the system MAY
propose intake. Human/product policy decides whether a proposal becomes
committed product work.*

- **Agents** may propose candidates discovered while executing or reviewing
  (technical debt, regressions, inconsistencies). These are proposals only.
- **System / R3 Diagnostics** may propose candidates from findings
  (operational failures, drift, policy rejections). Diagnostics is read-only
  today and must remain so; a separate proposal step would forward findings into
  intake.
- **Developers/operators** may promote proposals into committed work.

**No auto-creation is implemented in this plan.** The safest first
implementation (P1) is a proposal queue where proposals are clearly marked as
`proposed_by=agent|system` and require the Intake acceptance gate (#1) before
they can become work.

Open points: provenance schema, dedup between repeated findings, and whether a
proposal can ever bypass triage (see §13).

---

## 10. Plan to WorkItem Generation

Conceptual design only (`PLANNED`, P5). R5 is **not** modified.

A single product `Plan` may generate multiple `WorkItem`s. Example:

```
PLAN "Fix reservation cancellation"
  |
  +-- WorkItem A  backend investigation + fix
  +-- WorkItem B  Flutter UI
  +-- WorkItem C  E2E + validation
```

### Generation rules (proposed)

1. Each generated WorkItem goes through the existing `CreateWorkItem` path so
   that **R5** (concurrency limit), **R6** (policies: registered project,
   protected branch/paths, require runtime/validation) and **R7** (capability,
   `allowed_agents`) all apply automatically.
2. Each WorkItem records a **trace link** to its originating `PlanItem`
   (and transitively to the plan, work entity and intake).
3. A `PlanItem` may optionally request a **capability** (R7) and a base branch,
   subject to R6 policy.
4. A `PlanItem` may declare **dependencies** on other plan items.

### Dependencies

- R5 lets **independent** WorkItems run in parallel (already true).
- The future Planning Engine may **declare** dependencies
  (`A before B`). In this plan, dependencies are recorded as **metadata/links**
  and influence *generation order/human readiness*, but do **not** add a
  scheduler or queue.
- A blocking-dependency scheduler would change R5 and is therefore **out of
  scope** for P5 (candidate for a later, explicitly-approved phase).

### Relationship to R1-R7

P5 is the integration seam and must be a thin adapter over `CreateWorkItem`.
It must not reach into worktrees, sessions or gates.

---

## 11. Traceability

Traceability must be **first-class** (explicit links/IDs), not inferred from
prose. Target navigable chain (`CONCEPT`):

```
User Report → Intake → UserStory/Bug/Task → Plan → ProductCard → WorkItem
   → Session → Commit → Validation → Deploy → Release → User Feedback
                    ( ↑ all of Session/Commit/Validation exist in R1-R7 today )
```

- `Intake → work entity → plan → plan item` are new links (Product Layer).
- `WorkItem → Session → Evidence/Commit/Validation` already exist
  (`[IMPLEMENTED]`: sessions store evidence; worktrees record commits).
- `WorkItem → Release → Feedback` are new links (P7).
- The chain must be traversable **forwards and backwards** (from a user report
  down to evidence, and from a commit/release back to the request).

Design implication: prefer explicit reference fields/join records over free-text
metadata, because `WorkItem.Metadata` is explicitly non-authoritative and must
never drive state.

**`Deploy` note:** "Deploy" in the chain refers to the GitOps-by-SHA deploy of
the orchestrator itself (documented in `docs/deployment-piave.md`); whether the
*orchestrated projects* get a first-class Deploy entity is an open question
(§13). Do not assume a Deploy entity exists.

---

## 12. Human Gates

Candidate gates. Which are mandatory vs configurable is a product decision
(`OPEN QUESTION`, §13). None are implemented by this plan.

| # | Gate | Where | Likely obligation | Notes |
| --- | --- | --- | --- | --- |
| 1 | Intake acceptance | After Intake | **Mandatory by default** | Prevents noise/agent proposals becoming work unreviewed. |
| 2 | Story refinement/acceptance | After P2 work entity | Likely mandatory | Confirms acceptance criteria and readiness. |
| 3 | Plan approval | After P3 `Plan` | **Mandatory by default** | P5 generation should require an approved plan. |
| 4 | WorkItem implementation approval | Before Implementation | **Exists today** (`[IMPLEMENTED]`) | The engineering Human Gate; product layer may add a *plan-level* pre-approval rather than a second per-WorkItem gate. |
| 5 | Release approval | Before `Release` recorded | Configurable | May be automatic for low-risk changes; mandatory for critical. |

Principles:

- **Agents/system propose; humans approve.** Acceptance gates are the mechanism.
- **Reuse the existing gate vocabulary** (`Gate`, `Approval`,
  `AuthorizationRecord`) where semantics match; do not mutate the engineering
  gate state machine.
- Gate #4 (engineering implementation approval) already exists and must remain
  the mechanism that authorises Implementation; product gates are **additional**
  and upstream.

---

## 13. Open Architectural Questions

These require product/human decisions and are deliberately **not** resolved here.

1. **Intake and Product Backlog: same entity or separate?** Is "accepted intake"
   just a state, or a distinct backlog entity?
2. **Incident/Bug/UserStory/Task: types or distinct entities?** One work entity
   with a type discriminator, or separate tables/APIs?
3. **Who performs triage: human, agent or hybrid?** And where is the boundary of
   what an agent may decide autonomously?
4. **How to detect duplicates?** Exact references, fuzzy text, embeddings, or
   human judgement only?
5. **Who determines priority?** Product owner, requester, system, or an agent
   proposal that a human confirms?
6. **How to represent impact?** Severity, reach, business value, a formula, or
   free-form?
7. **How to handle attachments/screenshots/logs?** Where are binaries stored
   (object storage?), size limits, redaction, retention?
8. **How to identify users and permissions?** Is there a real identity/actor
   model, or named strings? Who may approve which gate?
9. **Can agents propose Intake automatically?** (Proposed: yes, as proposals —
   see §9 — but this is not ratified policy.)
10. **When does a proposal become authorized work?** At which gate, and who
    signs off?
11. **How do Product Kanban and Engineering Kanban relate?** Derive-only, or a
    formal mapping table between columns?
12. **How is a Release communicated to the originating user?** In-app, email,
    webhook, or purely recorded?
13. **How are reopen/regression handled?** New intake that links back, or a state
    transition on the existing work entity?
14. **What feedback metrics are needed?** Time-to-triage, time-to-release,
    reopen rate, satisfaction — and at what aggregation?
15. **Product `Plan` vs agent `domain.Plan` naming.** Rename one to avoid
    confusion before P3.
16. **Is there a first-class Deploy entity for orchestrated projects,** or is
    "deploy" only the orchestrator's own GitOps (see §11)?

---

## 14. Implementation Strategy

Proposed future sequence: **P1 → P2 → P3 → P4 → P5 → P6 → P7**. Keep each
WorkItem small and executable. All entries below are `PLANNED`.

> Every phase must be expressible as ordinary `WorkItem`s executed by R1-R7.
> Generation should go through the existing `CreateWorkItem` path so R5/R6/R7
> apply automatically.

### P1 — Product Intake

- **Likely WorkItems:** intake domain type + persistence; create/list/read API;
  provenance/audit events; acceptance/rejection transitions; minimal capture UI.
- **Dependencies:** none (may consume R3 diagnostics later).
- **Human decisions:** identity model (§13.8); attachment storage (§13.7);
  mandatory vs optional acceptance gate.
- **Expected DB changes:** new table(s) for `Intake` (+ migration `005_*`).
- **API/UI impact:** new `/api/intake` endpoints; a minimal capture/inbox view.
- **Relation to R1-R7:** none at runtime; must not touch `WorkItem`.

### P2 — User Stories / Bugs / Tasks

- **Likely WorkItems:** work-entity domain type (or typed entity); persistence;
  intake→work-entity link; acceptance criteria + priority; API/UI list/detail.
- **Dependencies:** P1.
- **Human decisions:** type-vs-entity (§13.2); priority owner (§13.5); impact
  model (§13.6); dedup approach (§13.4).
- **Expected DB changes:** new table(s) for work entities (+ links).
- **API/UI impact:** new endpoints and product board read model.
- **Relation to R1-R7:** references WorkItems only later (P5).

### P3 — Planning

- **Likely WorkItems:** `Plan`/`PlanItem` domain + persistence; plan creation
  from work entities; approval state; dependency metadata; API/UI.
- **Dependencies:** P2.
- **Human decisions:** resolve `Plan` naming collision (§13.15); mandatory plan
  structure; which gate approves plans (Gate #3).
- **Expected DB changes:** new tables for plans/plan items (+ links).
- **API/UI impact:** plan endpoints; plan review view.
- **Relation to R1-R7:** none at runtime; plans are the source for P5.

### P4 — Product Kanban

- **Likely WorkItems:** product status derivation; board read model; card
  projection linking plan items/work entities/WorkItems; UI board.
- **Dependencies:** P1-P3.
- **Human decisions:** derive-only vs mapped columns (§13.11); column
  definitions/exit criteria.
- **Expected DB changes:** ideally **none** (derived); a status field only if a
  column cannot be derived.
- **API/UI impact:** new read endpoints + board UI.
- **Relation to R1-R7:** reads WorkItem phase/evidence to derive "In
  Development"/"Validation"; never writes execution state.

### P5 — Plan to WorkItem Generation

- **Likely WorkItems:** plan-item→WorkItem generator calling `CreateWorkItem`;
  trace-link persistence; capability/base-branch mapping (R7/R6); dependency
  metadata; API/UI action.
- **Dependencies:** P3 (P4 optional).
- **Human decisions:** dependency semantics (advisory vs blocking) (§13.10);
  whether generation requires plan approval.
- **Expected DB changes:** trace-link table(s) (plan item ↔ WorkItem).
- **API/UI impact:** "generate WorkItems" action; trace links in views.
- **Relation to R1-R7:** **the integration seam**; must reuse `CreateWorkItem`
  so R5/R6/R7 apply; no R5 changes.

### P6 — Plan-level Human Gates

- **Likely WorkItems:** product gate/approval records; blocking transitions;
  audit events; approval API/UI; wiring into P3/P5.
- **Dependencies:** P3, P5.
- **Human decisions:** mandatory vs configurable gates (#1-#5); approver model
  (§13.8).
- **Expected DB changes:** gate/approval tables (or reuse patterns).
- **API/UI impact:** approve/reject endpoints and UI for plans/WorkItems.
- **Relation to R1-R7:** extends, never replaces, the existing engineering gate.

### P7 — Feedback / Release Loop

- **Likely WorkItems:** `Release` record referencing WorkItem evidence;
  `Feedback` capture linked to intake/release; reopen/regression flow; API/UI;
  minimal notification hook (optional).
- **Dependencies:** P5, P6.
- **Human decisions:** release definition per project; notification channel
  (§13.12); reopen vs new intake (§13.13); feedback metrics (§13.14); whether a
  Deploy entity is needed (§13.16).
- **Expected DB changes:** release/feedback tables (+ links).
- **API/UI impact:** release/feedback endpoints and views.
- **Relation to R1-R7:** reads evidence/commit; never re-executes work.

### Suggested sequencing constraints

- P1 and P2 can overlap; P3 needs P2; P4 can start once P1-P3 exist; P5 needs
  an approved plan (thus P3, and P6 for the approval itself); P6 should land
  with/just after P5; P7 needs P5+P6.
- Keep the **Product Layer** free of execution responsibilities at every step.

---

## 15. Success Criteria Check

| # | Requirement | Where addressed |
| --- | --- | --- |
| 1 | What the product is | §3 Product Vision |
| 2 | What the Execution Engine is | §5 Execution Engine Today (R1-R7) |
| 3 | How they connect | §3 (table), §10 Plan→WorkItem, §14 |
| 4 | Roadmap P1-P7 | §6 Roadmap, §14 Implementation Strategy |
| 5 | Candidate domain model | §7 Domain Model Analysis |
| 6 | Lifecycle | §6 phases, §8 Two Kanbans |
| 7 | Traceability | §11 Traceability |
| 8 | Human gates | §12 Human Gates |
| 9 | Unresolved decisions | §13 Open Architectural Questions |
| 10 | Future implementation sequence | §14 Implementation Strategy |

**Definition of done for this document:** all of the above present and
internally consistent; status labels applied consistently; no claim that a
planned feature already exists; no executable change introduced.

---

## 16. Non-Goals and Restrictions

This document intentionally does **not**:

- implement Product Intake, Planning, Kanban, auto-generated WorkItems or a
  Planning Engine;
- create product migrations, product APIs or a Product Kanban UI;
- modify the `WorkItem` domain model or the R1-R7 implementation;
- modify the CondoSmart or fin-engine repositories;
- resolve product decisions that require human input (they are listed in §13).

Repository rules that remain in force:

- GitHub `main` is the source of truth; no reset and no force-push.
- Baseline `9446665` is preserved; R1-R7 are unchanged by this plan.
- Deploy to Piave is only required when a change is executable; this document
  produces no executable change.

### Change control for this document

Future phases should update this file (or add phase-specific plans under
`docs/product/`) when a decision in §13 is resolved, moving the corresponding
item from `OPEN QUESTION` to `PLANNED`/`IMPLEMENTED` with the resolving
baseline SHA.
