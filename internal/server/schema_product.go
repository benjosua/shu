package server

const schemaProduct = `
-- API-first product additions.
alter table users add column if not exists email text not null default '';
alter table users add column if not exists avatar_url text not null default '';
alter table users add column if not exists language text not null default '';
alter table users add column if not exists onboarding jsonb not null default '{}';

alter table workspaces add column if not exists description text not null default '';
alter table workspaces add column if not exists issue_prefix text not null default '';

alter table agents add column if not exists description text not null default '';
alter table agents add column if not exists avatar_url text not null default '';
alter table agents add column if not exists archived boolean not null default false;

alter table issues add column if not exists origin text not null default '';
create index if not exists idx_issues_search on issues using gin(to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(description,'')));

alter table comments add column if not exists resolved_at timestamptz;

create table if not exists labels (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  name text not null,
  color text not null default '#808080',
  description text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(workspace_id, name)
);

create table if not exists object_links (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  source_type text not null,
  source_id uuid not null,
  relation text not null,
  target_type text not null,
  target_id uuid not null,
  metadata jsonb not null default '{}',
  created_at timestamptz not null default now(),
  unique(workspace_id,source_type,source_id,relation,target_type,target_id)
);
create index if not exists idx_object_links_source on object_links(workspace_id,source_type,source_id,relation);
create index if not exists idx_object_links_target on object_links(workspace_id,target_type,target_id,relation);

create table if not exists chat_sessions (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  title text not null default 'New chat',
  agent_id uuid references agents(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create table if not exists chat_messages (
  id bigserial primary key,
  workspace_id uuid not null references workspaces(id) on delete cascade,
  session_id uuid not null references chat_sessions(id) on delete cascade,
  role text not null,
  content text not null,
  created_at timestamptz not null default now()
);

create table if not exists activity_events (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  subject_type text not null default '',
  subject_id uuid,
  type text not null,
  actor_type text not null default '',
  actor_id uuid,
  payload jsonb not null default '{}',
  created_at timestamptz not null default now()
);
create index if not exists idx_activity_workspace on activity_events(workspace_id, created_at desc);
create index if not exists idx_activity_subject on activity_events(workspace_id, subject_type, subject_id, created_at desc);

create table if not exists autopilot_triggers (
  id uuid primary key default gen_random_uuid(),
  autopilot_id uuid not null references autopilots(id) on delete cascade,
  kind text not null default 'interval',
  interval_seconds int,
  enabled boolean not null default true,
  next_run_at timestamptz,
  payload jsonb not null default '{}',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists skills (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  name text not null,
  description text not null default '',
  content text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(workspace_id,name)
);
create table if not exists skill_files (
  id uuid primary key default gen_random_uuid(),
  skill_id uuid not null references skills(id) on delete cascade,
  path text not null,
  content text not null default '',
  updated_at timestamptz not null default now(),
  unique(skill_id,path)
);
create table if not exists agent_skills (
  agent_id uuid not null references agents(id) on delete cascade,
  skill_id uuid not null references skills(id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key(agent_id,skill_id)
);
alter table squad_members add column if not exists role text not null default 'member';
alter table squads add column if not exists avatar_url text not null default '';
alter table squads add column if not exists instructions text not null default '';
alter table squads add column if not exists archived boolean not null default false;
`
