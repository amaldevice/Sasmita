-- Ticket #20: full-text search over canonical conversation content, plus a
-- sentinel project for conversations that have no project (and therefore no
-- folder to open). FTS5 and JSON1 were verified against modernc.org/sqlite in
-- internal/persistence/sqlite/fts_test.go.
CREATE VIRTUAL TABLE conversation_fts USING fts5(
    conversation_id UNINDEXED,
    kind            UNINDEXED,
    message_id      UNINDEXED,
    text,
    tokenize = 'unicode61 remove_diacritics 2'
);

-- Backfill rows created by earlier schema versions.
INSERT INTO conversation_fts(conversation_id, kind, text)
    SELECT id, 'title', title FROM conversations;
INSERT INTO conversation_fts(conversation_id, kind, message_id, text)
    SELECT conversation_id, 'message', id,
           COALESCE(json_extract(content_json, '$[0].text'), content_json)
    FROM messages;

CREATE TRIGGER conversation_fts_ai AFTER INSERT ON conversations BEGIN
    INSERT INTO conversation_fts(conversation_id, kind, text)
    VALUES (new.id, 'title', new.title);
END;
CREATE TRIGGER conversation_fts_ad AFTER DELETE ON conversations BEGIN
    DELETE FROM conversation_fts WHERE conversation_id = old.id;
END;
CREATE TRIGGER conversation_fts_au AFTER UPDATE OF title ON conversations BEGIN
    DELETE FROM conversation_fts WHERE conversation_id = old.id AND kind = 'title';
    INSERT INTO conversation_fts(conversation_id, kind, text)
    VALUES (new.id, 'title', new.title);
END;

CREATE TRIGGER message_fts_ai AFTER INSERT ON messages BEGIN
    INSERT INTO conversation_fts(conversation_id, kind, message_id, text)
    VALUES (new.conversation_id, 'message', new.id,
            COALESCE(json_extract(new.content_json, '$[0].text'), new.content_json));
END;
CREATE TRIGGER message_fts_ad AFTER DELETE ON messages BEGIN
    DELETE FROM conversation_fts WHERE message_id = old.id;
END;
CREATE TRIGGER message_fts_au AFTER UPDATE OF content_json ON messages BEGIN
    DELETE FROM conversation_fts WHERE message_id = old.id;
    INSERT INTO conversation_fts(conversation_id, kind, message_id, text)
    VALUES (new.conversation_id, 'message', new.id,
            COALESCE(json_extract(new.content_json, '$[0].text'), new.content_json));
END;

-- A conversation with no project points at this row. Its empty root_uri is the
-- data-model signal that folder actions must not be offered in the UI.
INSERT OR IGNORE INTO projects(id, name, root_uri, created_at, updated_at)
VALUES ('', 'No project', '', '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z');
