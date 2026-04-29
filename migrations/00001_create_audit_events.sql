-- +goose Up
-- +goose StatementBegin
CREATE TABLE audit_events (
    id            BIGSERIAL   PRIMARY KEY,
    proposal_hash TEXT        NOT NULL,
    kind          TEXT        NOT NULL,
    payload       JSONB       NOT NULL,
    prev_hash     TEXT        NOT NULL DEFAULT '',
    hash          TEXT        NOT NULL,
    at            TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_audit_events_proposal_hash
    ON audit_events(proposal_hash);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS audit_events;
-- +goose StatementEnd
