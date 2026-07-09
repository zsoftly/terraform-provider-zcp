# CLAUDE.md — terraform-provider-zcp

AI assistant instructions for this repository (the ZCP Terraform/OpenTofu
provider).

---

## Project Documentation

- **[README.md](./README.md)** — overview
- **[CONTRIBUTING.md](./CONTRIBUTING.md)** — workflow, quality checks, commit
  style, PRs
- **[docs/](./docs/)** — provider docs: resource, data-source, and index pages
  (`tfplugindocs` frontmatter format, hand-maintained)
- **[examples/](./examples/)** — runnable HCL examples

---

## Critical AI Policies

### Always Run the Content Review Agent

After every documentation update, before handing back, you MUST run the
**[`content-review-agent`](~/.claude/agents/content-review-agent.md)** on the
changed files. Every engineer in the organization has this agent installed and
configured, so it is a required step of the development flow, not optional.

- Run it on each file you added or modified (diff against `HEAD` for edits, full
  content for new files).
- It detects AI writing patterns (em dashes, banned/hyperbolic words, weak "not
  just X but Y" constructions, filler adjectives, flowery marketing language)
  and fixes bad sentences.
- Apply its fixes so zero violations remain.
- Its writing style guide, banned-word list, and critical reminders are binding
  for all documentation in this repo.

This applies to all prose and content, including:

- Markdown under `docs/`, plus `README.md` and `CONTRIBUTING.md`
- Resource and data-source `description` and `MarkdownDescription` text in the
  Go schema
- Pull request and issue text
- Code comments that read as prose

Invoke it via the Agent tool with `subagent_type: content-review-agent`.

Respect the docs constraints while it edits: keep markdown structure (headers,
tables, code fences) and `tfplugindocs` frontmatter (`page_title`,
`description`). Do not change HCL examples, attribute or argument names, types,
schema tables, or any technical identifier. Apply the agent's prose rules to
prose only, never to code or schema content. Markdown is expected in these `.md`
docs, so do not strip it.

---

## Docs Authoring Conventions

- Each `docs/` page starts with `tfplugindocs` frontmatter:

  ```md
  ---
  page_title: "zcp_vpc Resource"
  description: |-
    Create and manage ZCP Virtual Private Clouds.
  ---
  ```

- Keep schema tables (argument reference, attribute reference) in sync with the
  Go schema in `internal/`.
- HCL examples must stay valid and runnable. Read `cloud_provider`/`region` from
  data sources rather than hardcoding.

---

## Validating Changes

After running the
[`content-review-agent`](~/.claude/agents/content-review-agent.md) on every
changed doc and applying its fixes, run these to validate code changes:

| Command          | Purpose                        |
| ---------------- | ------------------------------ |
| `make fmt`       | `gofmt` + prettier on `*.md`   |
| `make vet`       | `go vet ./...`                 |
| `make lint`      | Lint                           |
| `make test`      | `go test -v -count=1 ./...`    |
| `make test-race` | `go test -race -count=1 ./...` |
| `make build`     | Build the provider binary      |

Commit or push only when the user asks.
