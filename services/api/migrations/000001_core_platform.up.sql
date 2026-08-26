CREATE TABLE companies (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (btrim(name) <> ''),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL CHECK (email = lower(email) AND btrim(email) <> ''),
    password_hash text NOT NULL CHECK (btrim(password_hash) <> ''),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_email_unique ON users (lower(email));

CREATE TABLE company_users (
    company_id uuid NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (company_id, user_id)
);

CREATE INDEX company_users_user_id_idx ON company_users (user_id);

CREATE TABLE employees (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    user_id uuid,
    display_name text NOT NULL CHECK (btrim(display_name) <> ''),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT employees_company_user_fk
        FOREIGN KEY (company_id, user_id)
        REFERENCES company_users (company_id, user_id)
        ON DELETE RESTRICT,
    UNIQUE (company_id, user_id)
);

CREATE INDEX employees_company_id_idx ON employees (company_id);

CREATE TABLE permissions (
    key text PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$'),
    description text NOT NULL CHECK (btrim(description) <> ''),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    name text NOT NULL CHECK (btrim(name) <> ''),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (company_id, name),
    UNIQUE (company_id, id)
);

CREATE INDEX roles_company_id_idx ON roles (company_id);

CREATE TABLE role_permissions (
    company_id uuid NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    role_id uuid NOT NULL,
    permission_key text NOT NULL REFERENCES permissions (key) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_key),
    CONSTRAINT role_permissions_company_role_fk
        FOREIGN KEY (company_id, role_id)
        REFERENCES roles (company_id, id)
        ON DELETE CASCADE
);

CREATE INDEX role_permissions_company_id_idx ON role_permissions (company_id);

CREATE TABLE company_user_roles (
    company_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (company_id, user_id, role_id),
    CONSTRAINT company_user_roles_access_fk
        FOREIGN KEY (company_id, user_id)
        REFERENCES company_users (company_id, user_id)
        ON DELETE CASCADE,
    CONSTRAINT company_user_roles_role_fk
        FOREIGN KEY (company_id, role_id)
        REFERENCES roles (company_id, id)
        ON DELETE CASCADE
);

CREATE INDEX company_user_roles_role_id_idx ON company_user_roles (role_id);

CREATE TABLE module_entitlements (
    company_id uuid NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    module_key text NOT NULL CHECK (module_key ~ '^[a-z][a-z0-9_]*$'),
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (company_id, module_key)
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);

CREATE INDEX sessions_active_user_idx ON sessions (user_id, expires_at)
    WHERE revoked_at IS NULL;

CREATE TABLE audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    actor_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    action text NOT NULL CHECK (btrim(action) <> ''),
    target_type text NOT NULL CHECK (btrim(target_type) <> ''),
    target_id text NOT NULL CHECK (btrim(target_id) <> ''),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_company_occurred_at_idx
    ON audit_logs (company_id, occurred_at DESC);

INSERT INTO permissions (key, description) VALUES
    ('employees.view', 'View employees'),
    ('employees.manage', 'Create and update employees'),
    ('roles.view', 'View roles and permission assignments'),
    ('roles.manage', 'Create roles and manage permission assignments'),
    ('settings.manage', 'Manage company settings and module entitlements');
