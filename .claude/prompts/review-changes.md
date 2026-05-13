# Review Changes Orchestrator

Review the current git changes using four specialized reviewer personas running in parallel.

**Input**: Optionally specify a git ref range (e.g., `main..HEAD`, `HEAD~3`, a branch name, or a specific commit SHA). If omitted, defaults to `git diff HEAD` (staged + unstaged changes). If the working tree is clean, falls back to `git diff HEAD~1..HEAD` (last commit).

## Step 1: Get the diff

Run the appropriate git command based on input:
- If a ref range or branch was given: `git diff <input>`
- If no input: try `git diff HEAD` first; if empty, use `git diff HEAD~1..HEAD`

Also run `git log --oneline -5` and `git diff --stat HEAD` (or the specified range) to understand the scope.

If the diff is empty, tell the user there are no changes to review and stop.

## Step 2: Invoke all four reviewer personas in parallel

Verify that all four persona files exist before proceeding:
- `~/work/aep-devops-agent/subagents/security-reviewer.md`
- `~/work/aep-devops-agent/subagents/correctness-reviewer.md`
- `~/work/aep-devops-agent/subagents/architect-reviewer.md`
- `~/work/aep-devops-agent/subagents/devops-reviewer.md`

If any file is missing, stop and report:
```
Error: reviewer persona files not found at ~/aep-devops-agent/subagents/.
Ensure the aep-devops-agent repository is cloned from git@github.com:Adobe-Experience-Platform/aep-devops-agent.git at ~/aep-devops-agent and try again.
```

Read the persona instructions from these files, then launch all four as sub-agents simultaneously using the Agent tool (subagent_type: `general-purpose`). Pass each agent:
- The full diff content
- The diff stat summary
- The recent git log for context
- Its persona instructions (from the file above)

## Step 3: Collect all four responses

Wait for all four agents to complete. If any agent fails, note it in the final report but continue with the others.

## Step 4: Produce consolidated output

Format the final report as follows:

```
# Code Review Report

**Changes reviewed**: [git stat summary — files changed, insertions, deletions]
**Scope**: [commit range or "staged/unstaged changes"]
**Context**: [1-2 sentence description of what the changes accomplish, inferred from diff + git log]

---

## Security Review
[Paste security reviewer output here]

---

## Correctness Review
[Paste correctness reviewer output here]

---

## Architecture Review
[Paste architect reviewer output here]

---

## Operations Review
[Paste devops reviewer output here]

---

## Consolidated Findings

All findings sorted by priority:

### CRITICAL
[List any critical findings with reviewer tag, e.g. [Security] ...]

### HIGH
[List high priority findings with reviewer tag]

### MEDIUM
[List medium priority findings with reviewer tag]

### LOW
[List low priority findings with reviewer tag]

_(If no findings at a level, omit that section)_

---

## What Looks Good
[Positive observations from any reviewer — good patterns, solid error handling, well-tested code, etc. Max 5 items. Omit section if none noted.]

---

_Reviewers: security · correctness · architecture · operations_
```

## Guardrails

- Always run all four personas — never skip one even if the diff looks narrow
- Run personas in parallel for speed
- If the diff is very large (>500 lines changed), note it in the report header and focus each reviewer on their highest-signal areas
- Don't invent issues — base findings only on code visible in the diff (with codebase context as supporting evidence only)
- Keep the consolidated findings section free of duplication — if two reviewers flag the same issue, merge them and credit both
- If a reviewer finds nothing in their domain, preserve their "no concerns" statement — it's signal too
