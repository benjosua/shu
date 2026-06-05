# shu

Shu is an API-first control plane for agentic work. The server owns identity, policy, scheduling, and durable state. Executors run outside the control plane, register providers, claim scoped work, and return artifacts through the API.

## Design goals

- Keep orchestration deterministic and server-owned.
- Treat local and cloud execution as deployment choices, not different product models.
- Scope every work item to an explicit resource and executor policy.
- Preserve auditable artifacts for logs, messages, patches, and final results.
- Keep the CLI replaceable by any future UI or automation client.

## Core concepts

- **Workspace**: tenancy and authorization boundary.
- **Agent**: named execution profile with provider, model, instructions, and policy.
- **Executor**: registered runtime capable of running one or more providers.
- **Resource**: target the work may touch, such as a local checkout or repository.
- **Work item**: durable unit of execution assigned to an executor.
- **Artifact**: append-only output generated during execution.

## Execution flow

1. Client creates work against a workspace and optional resource.
2. Server selects an eligible executor from current policy and availability.
3. Executor claims work through the daemon API.
4. Daemon prepares isolated execution context and invokes the provider.
5. Logs, messages, usage, and results stream back as artifacts.
6. Server records completion, failure, or cancellation for later review.

Wakeups use websocket delivery when available. Polling remains as fallback, so missed realtime events do not lose work.

## API model

The API has two audiences:

- **User API**: workspaces, agents, issues, comments, attachments, inbox, squads, autopilots, resources, work, and artifacts.
- **Daemon API**: executor registration, heartbeat, work claim, status, artifact upload, completion, and failure reporting.

All long-running execution state flows through durable database rows. Realtime paths are optimization only.

## Security model

- Server is source of truth for identity, workspace membership, and assignment.
- Executors do not receive broad authority; they receive one scoped work payload.
- Resource access is explicit and attached to the work item.
- Artifacts return through the server instead of bypassing review paths.
- Local resources are restricted to local executors; cloud executors require remotely accessible resources.

## Operational notes

- PostgreSQL is required for durable state.
- Redis is optional and used only for cross-node realtime fanout.
- Daemons should be disposable; loss of a daemon must not corrupt server state.
- Work execution should be idempotent at the control-plane level: failed or cancelled items can be retried as new work.
- Migrations are applied at startup and can also be run as a standalone command.

## Development

Use the standard Go toolchain for build and test. Keep package boundaries aligned with runtime responsibilities: command dispatch, API client, CLI, configuration, daemon runtime, and server control plane.
