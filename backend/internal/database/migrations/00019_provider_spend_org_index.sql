-- +goose Up
-- The spend guard sums one workspace's spend today on every paid request.
CREATE INDEX go_provider_spend_org_created ON go_provider_spend (organization_id, created_at);

-- +goose Down
DROP INDEX go_provider_spend_org_created;
