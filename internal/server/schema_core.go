package server

const schemaCore = `
-- Core orchestrator abstraction: executor mesh + work queue.
create table if not exists executors (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  daemon_id text not null,
  name text not null,
  mode text not null default 'local' check (mode in ('local','cloud')),
  provider text not null,
  status text not null default 'offline',
  version text not null default '',
  last_seen_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(workspace_id, daemon_id, provider)
);
create index if not exists idx_executors_pick on executors(workspace_id, provider, mode, status, last_seen_at desc);

create table if not exists resources (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  kind text not null,
  locator text not null,
  metadata jsonb not null default '{}',
  created_at timestamptz not null default now()
);
create index if not exists idx_resources_workspace on resources(workspace_id, created_at desc);

create table if not exists runs (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  kind text not null,
  subject_type text not null default '',
  subject_id uuid,
  status text not null default 'queued',
  input jsonb not null default '{}',
  result jsonb not null default '{}',
  error text not null default '',
  started_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists idx_runs_workspace on runs(workspace_id, status, created_at desc);
create index if not exists idx_runs_subject on runs(subject_type, subject_id);

create table if not exists work_items (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  run_id uuid references runs(id) on delete set null,
  kind text not null default 'work',
  parent_id uuid references work_items(id) on delete set null,
  title text not null,
  prompt text not null default '',
  resource_id uuid references resources(id) on delete set null,
  policy jsonb not null default '{}',
  provider text not null default 'codex',
  agent_id uuid references agents(id) on delete set null,
  executor_id uuid references executors(id) on delete set null,
  status text not null default 'queued',
  priority int not null default 0,
  result text not null default '',
  error text not null default '',
  created_at timestamptz not null default now(),
  dispatched_at timestamptz,
  started_at timestamptz,
  completed_at timestamptz
);
alter table work_items add column if not exists agent_id uuid references agents(id) on delete set null;
alter table work_items add column if not exists run_id uuid references runs(id) on delete set null;
create index if not exists idx_work_claim on work_items(executor_id, priority desc, created_at asc) where status='queued';
create index if not exists idx_work_workspace on work_items(workspace_id, created_at desc);
alter table autopilot_runs add column if not exists work_id uuid references work_items(id) on delete set null;
alter table autopilot_runs add column if not exists run_id uuid references runs(id) on delete set null;

create table if not exists artifacts (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  work_id uuid not null references work_items(id) on delete cascade,
  type text not null,
  data jsonb not null default '{}',
  created_at timestamptz not null default now()
);
create index if not exists idx_artifacts_work on artifacts(work_id, created_at);

`
