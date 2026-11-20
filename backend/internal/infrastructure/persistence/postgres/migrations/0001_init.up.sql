-- 0001_init creates the user-owned data of the Go service.
CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));

CREATE TABLE refresh_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL,
    family_id uuid NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT refresh_tokens_token_hash_key UNIQUE (token_hash)
);

CREATE INDEX refresh_tokens_user_id_idx ON refresh_tokens (user_id);
CREATE INDEX refresh_tokens_expires_at_idx ON refresh_tokens (expires_at);

CREATE TABLE search_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    query text NOT NULL,
    filters jsonb NOT NULL,
    requested_limit integer NOT NULL,
    response jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT search_history_query_check CHECK (length(btrim(query)) > 0),
    CONSTRAINT search_history_filters_object CHECK (jsonb_typeof(filters) = 'object'),
    CONSTRAINT search_history_response_object CHECK (jsonb_typeof(response) = 'object'),
    CONSTRAINT search_history_requested_limit_check CHECK (requested_limit > 0)
);

CREATE INDEX search_history_user_created_idx
    ON search_history (user_id, created_at DESC, id DESC);

CREATE TABLE user_movies (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    movie_id bigint NOT NULL,
    status text NOT NULL,
    liked boolean,
    movie_snapshot jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, movie_id),
    CONSTRAINT user_movies_status_check CHECK (status IN ('want_to_watch', 'watched')),
    CONSTRAINT user_movies_liked_check CHECK (liked IS NULL OR status = 'watched'),
    CONSTRAINT user_movies_snapshot_object CHECK (jsonb_typeof(movie_snapshot) = 'object')
);

CREATE INDEX user_movies_user_status_updated_idx
    ON user_movies (user_id, status, updated_at DESC, movie_id);
