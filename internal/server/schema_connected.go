package server

const schemaConnected = `
create table if not exists resource_secrets (
  resource_id uuid primary key references resources(id) on delete cascade,
  workspace_id uuid not null references workspaces(id) on delete cascade,
  ciphertext text not null default '',
  updated_at timestamptz not null default now()
);
create index if not exists idx_resource_secrets_workspace on resource_secrets(workspace_id);

create table if not exists items (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  source_resource_id uuid references resources(id) on delete cascade,
  kind text not null,
  external_id text not null,
  title text not null default '',
  body text not null default '',
  state jsonb not null default '{}',
  tags jsonb not null default '[]',
  starts_at timestamptz,
  ends_at timestamptz,
  occurred_at timestamptz,
  synced_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(source_resource_id, kind, external_id)
);

create index if not exists idx_items_workspace on items(workspace_id, kind, updated_at desc);
create index if not exists idx_items_resource on items(source_resource_id, kind, external_id);
create index if not exists idx_items_tags on items using gin(tags);
create index if not exists idx_items_search on items using gin(to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(body,'')));

create or replace view todos as
select id, workspace_id, title, body, state, tags, created_at, updated_at
from items
where kind = 'todo.item';

create or replace view emails as
select id, workspace_id, source_resource_id as resource_id, external_id, title as subject, body, state, tags, occurred_at, synced_at, created_at, updated_at
from items
where kind = 'email.message';

create or replace view calendar_events as
select id, workspace_id, source_resource_id as resource_id, external_id, title, body, state, tags, starts_at, ends_at, synced_at, created_at, updated_at
from items
where kind = 'calendar.event';

create table if not exists external_actions (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  resource_id uuid references resources(id) on delete set null,
  item_id uuid references items(id) on delete set null,
  work_id uuid references work_items(id) on delete set null,
  run_id uuid references runs(id) on delete set null,
  action text not null,
  input jsonb not null default '{}',
  result jsonb not null default '{}',
  status text not null default 'pending' check (status in ('pending','approved','running','succeeded','failed','cancelled')),
  error text not null default '',
  created_at timestamptz not null default now(),
  completed_at timestamptz
);
create index if not exists idx_external_actions_workspace on external_actions(workspace_id, status, created_at desc);
create index if not exists idx_external_actions_item on external_actions(item_id);

create table if not exists external_sync_runs (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  resource_id uuid not null references resources(id) on delete cascade,
  run_id uuid references runs(id) on delete set null,
  status text not null default 'running',
  items_seen int not null default 0,
  items_upserted int not null default 0,
  actions_run int not null default 0,
  error text not null default '',
  started_at timestamptz not null default now(),
  completed_at timestamptz
);
create index if not exists idx_external_sync_runs_resource on external_sync_runs(resource_id, started_at desc);
`
