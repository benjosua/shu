-- Unify old specialized tables into generic stores, then remove unused schema bloat.

-- Direct interval fields moved from autopilots into autopilot_triggers.
do $$
begin
  if exists (select 1 from information_schema.columns where table_schema = current_schema() and table_name = 'autopilots' and column_name = 'cron_interval_seconds') then
    insert into autopilot_triggers(autopilot_id, kind, interval_seconds, enabled, next_run_at, payload, created_at, updated_at)
    select a.id,
           'interval',
           a.cron_interval_seconds,
           a.enabled,
           coalesce(a.next_run_at, now() + make_interval(secs => a.cron_interval_seconds)),
           jsonb_build_object('source', 'schedule'),
           a.created_at,
           now()
      from autopilots a
     where a.cron_interval_seconds is not null
       and a.cron_interval_seconds > 0
       and not exists (
         select 1 from autopilot_triggers t
          where t.autopilot_id = a.id
            and t.kind = 'interval'
       );
  end if;
end $$;

-- issue_labels is now one object_links relation.
do $$
begin
  if to_regclass('issue_labels') is not null then
    insert into object_links(workspace_id, source_type, source_id, relation, target_type, target_id, created_at)
    select i.workspace_id, 'issue', il.issue_id, 'label', 'label', il.label_id, il.created_at
      from issue_labels il
      join issues i on i.id = il.issue_id
    on conflict do nothing;
  end if;
end $$;

-- external_items is now generic items with source_resource_id.
do $$
begin
  if to_regclass('external_items') is not null and to_regclass('items') is not null then
    if exists (select 1 from information_schema.columns where table_schema = current_schema() and table_name = 'external_items' and column_name = 'resource_id') then
      insert into items(id, workspace_id, source_resource_id, kind, external_id, title, body, state, tags, starts_at, ends_at, occurred_at, synced_at, created_at, updated_at)
      select id, workspace_id, resource_id, kind, external_id, title, body, state, tags, starts_at, ends_at, occurred_at, synced_at, created_at, updated_at
        from external_items
      on conflict (source_resource_id, kind, external_id) do update set
        title = excluded.title,
        body = excluded.body,
        state = excluded.state,
        tags = excluded.tags,
        starts_at = excluded.starts_at,
        ends_at = excluded.ends_at,
        occurred_at = excluded.occurred_at,
        synced_at = excluded.synced_at,
        updated_at = excluded.updated_at;
    elsif exists (select 1 from information_schema.columns where table_schema = current_schema() and table_name = 'external_items' and column_name = 'source_resource_id') then
      insert into items(id, workspace_id, source_resource_id, kind, external_id, title, body, state, tags, starts_at, ends_at, occurred_at, synced_at, created_at, updated_at)
      select id, workspace_id, source_resource_id, kind, external_id, title, body, state, tags, starts_at, ends_at, occurred_at, synced_at, created_at, updated_at
        from external_items
      on conflict (source_resource_id, kind, external_id) do update set
        title = excluded.title,
        body = excluded.body,
        state = excluded.state,
        tags = excluded.tags,
        starts_at = excluded.starts_at,
        ends_at = excluded.ends_at,
        occurred_at = excluded.occurred_at,
        synced_at = excluded.synced_at,
        updated_at = excluded.updated_at;
    end if;
  end if;
end $$;

-- Existing work/actions/sync rows get run records.
with created as (
  insert into runs(workspace_id, kind, subject_type, subject_id, status, input, result, error, started_at, completed_at, created_at, updated_at)
  select wi.workspace_id,
         'agent.work',
         'work',
         wi.id,
         wi.status,
         jsonb_build_object('kind', wi.kind, 'title', wi.title, 'resource_id', wi.resource_id, 'agent_id', wi.agent_id),
         case when wi.result <> '' then jsonb_build_object('text', wi.result) else '{}'::jsonb end,
         wi.error,
         wi.started_at,
         wi.completed_at,
         wi.created_at,
         coalesce(wi.completed_at, wi.started_at, wi.created_at)
    from work_items wi
   where wi.run_id is null
  returning id, subject_id
)
update work_items wi
   set run_id = created.id
  from created
 where wi.id = created.subject_id;

with created as (
  insert into runs(workspace_id, kind, subject_type, subject_id, status, input, result, error, started_at, completed_at, created_at, updated_at)
  select ea.workspace_id,
         'external.action',
         'external_action',
         ea.id,
         case when ea.status in ('pending', 'approved') then 'queued' else ea.status end,
         ea.input,
         ea.result,
         ea.error,
         case when ea.status in ('running', 'succeeded', 'failed', 'cancelled') then ea.created_at else null end,
         ea.completed_at,
         ea.created_at,
         coalesce(ea.completed_at, ea.created_at)
    from external_actions ea
   where ea.run_id is null
  returning id, subject_id
)
update external_actions ea
   set run_id = created.id
  from created
 where ea.id = created.subject_id;

with created as (
  insert into runs(workspace_id, kind, subject_type, subject_id, status, input, result, error, started_at, completed_at, created_at, updated_at)
  select sr.workspace_id,
         'external.sync',
         'external_sync_run',
         sr.id,
         sr.status,
         jsonb_build_object('resource_id', sr.resource_id),
         jsonb_build_object('items_seen', sr.items_seen, 'items_upserted', sr.items_upserted, 'actions_run', sr.actions_run),
         sr.error,
         sr.started_at,
         sr.completed_at,
         sr.started_at,
         coalesce(sr.completed_at, sr.started_at)
    from external_sync_runs sr
   where sr.run_id is null
  returning id, subject_id
)
update external_sync_runs sr
   set run_id = created.id
  from created
 where sr.id = created.subject_id;

update autopilot_runs ar
   set run_id = wi.run_id
  from work_items wi
 where ar.work_id = wi.id
   and ar.run_id is null;

with created as (
  insert into runs(workspace_id, kind, subject_type, subject_id, status, input, result, started_at, completed_at, created_at, updated_at)
  select ar.workspace_id,
         'agent.autopilot',
         'autopilot_run',
         ar.id,
         ar.status,
         jsonb_build_object('autopilot_id', ar.autopilot_id, 'trigger_payload', ar.trigger_payload),
         '{}',
         ar.started_at,
         ar.completed_at,
         ar.started_at,
         coalesce(ar.completed_at, ar.started_at)
    from autopilot_runs ar
   where ar.run_id is null
  returning id, subject_id
)
update autopilot_runs ar
   set run_id = created.id
  from created
 where ar.id = created.subject_id;

-- Repoint legacy external_actions.item_id foreign key to items before dropping external_items.
do $$
declare
  constraint_name text;
begin
  if to_regclass('external_items') is not null and to_regclass('external_actions') is not null then
    for constraint_name in
      select c.conname
        from pg_constraint c
        join pg_class t on t.oid = c.conrelid
       where t.relname = 'external_actions'
         and c.contype = 'f'
         and c.confrelid = 'external_items'::regclass
    loop
      execute format('alter table external_actions drop constraint %I', constraint_name);
    end loop;
  end if;

  if to_regclass('external_actions') is not null and to_regclass('items') is not null then
    if not exists (
      select 1
        from pg_constraint c
       where c.conrelid = 'external_actions'::regclass
         and c.contype = 'f'
         and c.confrelid = 'items'::regclass
    ) then
      alter table external_actions add constraint external_actions_item_id_fkey foreign key (item_id) references items(id) on delete set null;
    end if;
  end if;
end $$;

-- Drop dead tables/features with no product path.
drop table if exists access_grants cascade;
drop table if exists comment_reactions cascade;
drop table if exists issue_reactions cascade;
drop table if exists issue_subscribers cascade;
drop table if exists notification_preferences cascade;
drop table if exists pinned_items cascade;
drop table if exists workspace_tabs cascade;
drop table if exists feedback cascade;
drop table if exists issue_labels cascade;
drop table if exists external_items cascade;

-- Drop unused columns. Data already lives in active tables where needed.
alter table issues drop column if exists number;
alter table issues drop column if exists due_date;
alter table issues drop column if exists first_executed_at;
alter table comments drop column if exists parent_id;
alter table chat_sessions drop column if exists unread_since;
alter table chat_messages drop column if exists failure_reason;
alter table chat_messages drop column if exists elapsed_ms;
alter table attachments drop column if exists chat_session_id;
alter table attachments drop column if exists chat_message_id;
alter table agents drop column if exists visibility;
alter table agents drop column if exists mcp_config;
alter table agents drop column if exists concurrency;
alter table executors drop column if exists capabilities;
alter table executors drop column if exists metadata;
alter table autopilots drop column if exists trigger_type;
alter table autopilots drop column if exists cron_interval_seconds;
alter table autopilots drop column if exists next_run_at;
alter table inbox_items drop column if exists recipient_type;
alter table inbox_items drop column if exists recipient_id;
