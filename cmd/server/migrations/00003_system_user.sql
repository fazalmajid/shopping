-- +goose Up
-- System user: fixed all-zero ID used as invited_by for bootstrap invitations.
-- Never logs in; excluded from user counts.
INSERT INTO users(id, email, display_name)
VALUES('\x00000000000000000000000000000000', '', 'System')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM users WHERE id = '\x00000000000000000000000000000000';
