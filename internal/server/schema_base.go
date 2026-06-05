package server

const schemaBase = `
create extension if not exists "pgcrypto";

create table if not exists users (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  token_hash text unique,
  created_at timestamptz not null default now()
);
alter table users add column if not exists token_hash text unique;

create table if not exists personal_access_tokens (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  name text not null,
  token_hash text unique not null,
  expires_at timestamptz,
  last_used_at timestamptz,
  created_at timestamptz not null default now()
);
create index if not exists idx_personal_access_tokens_user on personal_access_tokens(user_id);

create table if not exists workspaces (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  slug text unique not null,
  created_at timestamptz not null default now()
);

create table if not exists workspace_members (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  user_id uuid references users(id) on delete cascade,
  role text not null default 'member',
  created_at timestamptz not null default now(),
  unique(workspace_id, user_id)
);
create index if not exists idx_workspace_members_workspace on workspace_members(workspace_id);

create table if not exists agents (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  name text not null,
  provider text not null,
  model text not null default '',
  instructions text not null default '',
  custom_env jsonb not null default '{}',
  custom_args jsonb not null default '[]',
  created_at timestamptz not null default now(),
  unique(workspace_id, name)
);

create table if not exists issues (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  title text not null,
  description text not null default '',
  status text not null default 'backlog',
  priority text not null default 'none',
  assignee_type text not null default '',
  assignee_id uuid,
  parent_issue_id uuid references issues(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists idx_issues_workspace on issues(workspace_id, status, created_at desc);
create index if not exists idx_issues_assignee on issues(assignee_type, assignee_id);

create table if not exists comments (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  issue_id uuid not null references issues(id) on delete cascade,
  author_user_id uuid references users(id) on delete set null,
  body text not null,
  resolved boolean not null default false,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists idx_comments_issue on comments(issue_id, created_at);

create table if not exists attachments (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  issue_id uuid references issues(id) on delete cascade,
  comment_id uuid references comments(id) on delete cascade,
  file_name text not null,
  content_type text not null default 'application/octet-stream',
  size_bytes bigint not null default 0,
  storage_path text not null,
  created_at timestamptz not null default now()
);
create index if not exists idx_attachments_issue on attachments(issue_id, created_at);
create index if not exists idx_attachments_comment on attachments(comment_id, created_at);

create table if not exists inbox_items (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  type text not null,
  severity text not null default 'info',
  title text not null,
  body text not null default '',
  read boolean not null default false,
  archived boolean not null default false,
  created_at timestamptz not null default now()
);

create table if not exists squads (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  name text not null,
  leader_agent_id uuid not null references agents(id) on delete cascade,
  created_at timestamptz not null default now(),
  unique(workspace_id, name)
);
create table if not exists squad_members (
  squad_id uuid not null references squads(id) on delete cascade,
  member_type text not null,
  member_id uuid not null,
  primary key(squad_id, member_type, member_id)
);

create table if not exists autopilots (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  name text not null,
  prompt text not null,
  assignee_type text not null default 'agent',
  assignee_id uuid not null,
  enabled boolean not null default true,
  created_at timestamptz not null default now()
);
create table if not exists autopilot_runs (
  id uuid primary key default gen_random_uuid(),
  autopilot_id uuid not null references autopilots(id) on delete cascade,
  workspace_id uuid not null references workspaces(id) on delete cascade,
  status text not null default 'queued',
  trigger_payload jsonb not null default '{}',
  started_at timestamptz not null default now(),
  completed_at timestamptz
);
`
