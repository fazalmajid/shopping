-- +goose Up

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE sections (
    id         SERIAL PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    sort_order INT  NOT NULL DEFAULT 0
);

CREATE TABLE users (
    id           BYTEA PRIMARY KEY,
    email        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE webauthn_credentials (
    id           TEXT PRIMARY KEY,   -- base64url-encoded credential ID
    user_id      BYTEA NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    data         TEXT NOT NULL,      -- JSON-encoded webauthn.Credential
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);
CREATE INDEX ON webauthn_credentials(user_id);

-- Short-lived: stores go-webauthn SessionData between begin/finish ceremonies
CREATE TABLE webauthn_sessions (
    id         TEXT PRIMARY KEY,
    data       TEXT NOT NULL,        -- JSON-encoded webauthn.SessionData
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

-- Long-lived: authenticated user sessions (random hex token in cookie)
CREATE TABLE app_sessions (
    id         TEXT PRIMARY KEY,
    user_id    BYTEA NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX ON app_sessions(user_id);

CREATE TABLE items (
    id         BIGSERIAL PRIMARY KEY,
    text       TEXT NOT NULL,
    section_id INT REFERENCES sections(id),  -- NULL while classifying
    checked    BOOLEAN NOT NULL DEFAULT false,
    added_by   BYTEA REFERENCES users(id),
    added_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    checked_at TIMESTAMPTZ,
    checked_by BYTEA REFERENCES users(id)
);
CREATE INDEX ON items(section_id);
CREATE INDEX ON items(checked);

CREATE TABLE invitations (
    token      TEXT PRIMARY KEY,
    email      TEXT NOT NULL,
    invited_by BYTEA NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ
);

-- Lookup cache: normalized item text → section assignment
-- Manual choices (source='manual') are never overwritten by LLM results.
CREATE TABLE item_sections (
    item_text  TEXT PRIMARY KEY,
    section_id INT NOT NULL REFERENCES sections(id),
    source     TEXT NOT NULL DEFAULT 'llm',  -- 'llm' | 'manual'
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down

DROP TABLE IF EXISTS item_sections;
DROP TABLE IF EXISTS invitations;
DROP TABLE IF EXISTS items;
DROP TABLE IF EXISTS app_sessions;
DROP TABLE IF EXISTS webauthn_sessions;
DROP TABLE IF EXISTS webauthn_credentials;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS sections;
