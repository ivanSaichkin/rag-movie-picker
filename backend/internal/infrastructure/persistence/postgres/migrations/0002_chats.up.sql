CREATE TABLE chats (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    UNIQUE (user_id, id)
);

INSERT INTO chats (id, user_id) SELECT id, user_id FROM search_history;
ALTER TABLE search_history ADD COLUMN chat_id uuid;
UPDATE search_history SET chat_id = id;
ALTER TABLE search_history ALTER COLUMN chat_id SET NOT NULL;
ALTER TABLE search_history ADD CONSTRAINT search_history_chat_owner_fk
    FOREIGN KEY (user_id, chat_id) REFERENCES chats (user_id, id) ON DELETE CASCADE;
CREATE INDEX search_history_chat_turn_idx ON search_history (user_id, chat_id, created_at, id);
