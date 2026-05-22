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
alter table agents add column if not exists visibility text not null default 'workspace';
alter table agents add column if not exists mcp_config jsonb not null default '{}';
alter table agents add column if not exists concurrency int not null default 1;

alter table issues add column if not exists number bigserial;
alter table issues add column if not exists origin text not null default '';
alter table issues add column if not exists first_executed_at timestamptz;
create index if not exists idx_issues_search on issues using gin(to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(description,'')));

alter table comments add column if not exists parent_id uuid references comments(id) on delete cascade;
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
create table if not exists issue_labels (
  issue_id uuid not null references issues(id) on delete cascade,
  label_id uuid not null references labels(id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key(issue_id,label_id)
);
create table if not exists issue_subscribers (
  issue_id uuid not null references issues(id) on delete cascade,
  subscriber_type text not null,
  subscriber_id uuid not null,
  created_at timestamptz not null default now(),
  primary key(issue_id,subscriber_type,subscriber_id)
);
create table if not exists issue_reactions (
  issue_id uuid not null references issues(id) on delete cascade,
  actor_type text not null,
  actor_id uuid not null,
  emoji text not null,
  created_at timestamptz not null default now(),
  primary key(issue_id,actor_type,actor_id,emoji)
);
create table if not exists comment_reactions (
  comment_id uuid not null references comments(id) on delete cascade,
  actor_type text not null,
  actor_id uuid not null,
  emoji text not null,
  created_at timestamptz not null default now(),
  primary key(comment_id,actor_type,actor_id,emoji)
);

create table if not exists notification_preferences (
  workspace_id uuid not null references workspaces(id) on delete cascade,
  user_id uuid not null references users(id) on delete cascade,
  preferences jsonb not null default '{}',
  updated_at timestamptz not null default now(),
  primary key(workspace_id,user_id)
);

create table if not exists pinned_items (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  item_type text not null,
  item_id uuid not null,
  sort_order int not null default 0,
  created_at timestamptz not null default now(),
  unique(workspace_id,item_type,item_id)
);

create table if not exists chat_sessions (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  title text not null default 'New chat',
  agent_id uuid references agents(id) on delete set null,
  unread_since timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create table if not exists chat_messages (
  id bigserial primary key,
  workspace_id uuid not null references workspaces(id) on delete cascade,
  session_id uuid not null references chat_sessions(id) on delete cascade,
  role text not null,
  content text not null,
  failure_reason text not null default '',
  elapsed_ms bigint not null default 0,
  created_at timestamptz not null default now()
);
alter table attachments add column if not exists chat_session_id uuid references chat_sessions(id) on delete cascade;
alter table attachments add column if not exists chat_message_id bigint references chat_messages(id) on delete cascade;

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

create table if not exists feedback (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid references workspaces(id) on delete cascade,
  user_id uuid references users(id) on delete set null,
  kind text not null default 'feedback',
  body text not null,
  metadata jsonb not null default '{}',
  created_at timestamptz not null default now()
);

alter table squad_members add column if not exists role text not null default 'member';
alter table squads add column if not exists avatar_url text not null default '';
alter table squads add column if not exists instructions text not null default '';
alter table squads add column if not exists archived boolean not null default false;
`
