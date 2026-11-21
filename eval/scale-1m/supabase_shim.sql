-- Minimal stand-in for what a Supabase project provides before the repo's migrations run.
-- Local scale test only.

-- Roles. authenticator is the login role PostgREST uses; the API itself connects as postgres
-- (superuser here; on Supabase postgres has BYPASSRLS) and does SET LOCAL ROLE authenticated.
create role anon nologin noinherit;
create role authenticated nologin noinherit;
create role service_role nologin noinherit bypassrls;
create role authenticator login noinherit;
grant anon, authenticated, service_role to authenticator;

create schema extensions;
grant usage on schema extensions to anon, authenticated, service_role;
create extension pgcrypto schema extensions;
create extension "uuid-ossp" schema extensions;

-- pgvector is not installed in /Library/PostgreSQL/18. Stub: extensions.vector is a domain over
-- real[] with a SQL cosine-distance <=> so migrations 029/030 (adapted copies) apply and the
-- source_chunks.embedding column exists. No HNSW index. Only used when GEMINI_API_KEY is set.
create domain extensions.vector as real[];
create function extensions.cosine_distance(a real[], b real[]) returns float8
language sql immutable strict as $$
  select 1 - sum(x * y) / nullif(sqrt(sum(x * x)) * sqrt(sum(y * y)), 0)
  from unnest(a, b) as t(x, y)
$$;
create operator extensions.<=> (leftarg = real[], rightarg = real[], function = extensions.cosine_distance);

-- auth schema: the columns GoTrue's auth.users has that seeds/migrations touch.
create schema auth;
grant usage on schema auth to anon, authenticated, service_role;
create table auth.users (
  instance_id uuid,
  id uuid primary key default gen_random_uuid(),
  aud text,
  role text,
  email text,
  encrypted_password text,
  email_confirmed_at timestamptz,
  raw_app_meta_data jsonb default '{}'::jsonb,
  raw_user_meta_data jsonb default '{}'::jsonb,
  created_at timestamptz default now(),
  updated_at timestamptz default now(),
  last_sign_in_at timestamptz,
  confirmation_token text,
  email_change text,
  email_change_token_new text,
  recovery_token text
);
create table auth.identities (
  id text, user_id uuid references auth.users on delete cascade, provider_id text,
  identity_data jsonb, provider text, last_sign_in_at timestamptz,
  created_at timestamptz, updated_at timestamptz
);

-- Same definitions Supabase ships: read the claims PostgREST (or db.AsUser) sets per request.
create function auth.uid() returns uuid language sql stable as $$
  select coalesce(
    nullif(current_setting('request.jwt.claim.sub', true), ''),
    (nullif(current_setting('request.jwt.claims', true), '')::jsonb ->> 'sub')
  )::uuid
$$;
create function auth.role() returns text language sql stable as $$
  select coalesce(
    nullif(current_setting('request.jwt.claim.role', true), ''),
    (nullif(current_setting('request.jwt.claims', true), '')::jsonb ->> 'role')
  )::text
$$;
create function auth.jwt() returns jsonb language sql stable as $$
  select coalesce(
    nullif(current_setting('request.jwt.claim', true), ''),
    nullif(current_setting('request.jwt.claims', true), '')
  )::jsonb
$$;
grant execute on function auth.uid(), auth.role(), auth.jwt() to anon, authenticated, service_role;

-- storage schema: 023 inserts the private course-sources bucket.
create schema storage;
create table storage.buckets (id text primary key, name text, public boolean, created_at timestamptz default now());
create table storage.objects (id uuid primary key default gen_random_uuid(), bucket_id text references storage.buckets,
  name text, owner uuid, created_at timestamptz default now());
