package ranktracking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const configColumns = `id, project_id, domain, location_code, language_code, location_name, devices,
	serp_depth, schedule_interval, is_active, last_checked_at, next_check_at, last_skip_reason, created_at`

const keywordColumns = `id, config_id, keyword, match_case, search_volume, keyword_difficulty, cpc,
	metrics_fetched_at, created_at`

// Store is the Postgres repository for configs and keywords.
type Store struct{ DB *pgxpool.Pool }

type rowScanner interface{ Scan(...any) error }

func scanConfig(row rowScanner) (Config, error) {
	var c Config
	err := row.Scan(&c.ID, &c.ProjectID, &c.Domain, &c.LocationCode, &c.LanguageCode, &c.LocationName,
		&c.Devices, &c.SerpDepth, &c.ScheduleInterval, &c.IsActive, &c.LastCheckedAt, &c.NextCheckAt,
		&c.LastSkipReason, &c.CreatedAt)
	return c, err
}

func scanKeyword(row rowScanner) (Keyword, error) {
	var k Keyword
	err := row.Scan(&k.ID, &k.ConfigID, &k.Keyword, &k.MatchCase, &k.SearchVolume, &k.KeywordDifficulty,
		&k.CPC, &k.MetricsFetchedAt, &k.CreatedAt)
	return k, err
}

func isUniqueViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23505"
}

// ListActiveConfigs returns the project's active configs, oldest first.
func (s Store) ListActiveConfigs(ctx context.Context, projectID string) ([]Config, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+configColumns+` FROM go_rank_tracking_configs
		WHERE project_id = $1 AND is_active ORDER BY created_at, id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list rank tracking configs: %w", err)
	}
	configs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Config, error) { return scanConfig(r) })
	if err != nil {
		return nil, fmt.Errorf("read rank tracking configs: %w", err)
	}
	return configs, nil
}

// GetConfig returns one config of the project, active or archived.
func (s Store) GetConfig(ctx context.Context, projectID, id string) (Config, error) {
	c, err := scanConfig(s.DB.QueryRow(ctx, `SELECT `+configColumns+` FROM go_rank_tracking_configs
		WHERE id = $1 AND project_id = $2`, id, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Config{}, ErrNotFound
	}
	if err != nil {
		return Config{}, fmt.Errorf("get rank tracking config %s: %w", id, err)
	}
	return c, nil
}

// FindConfig looks a config up by its unique market key, active or archived.
// It returns ErrNotFound when none exists.
func (s Store) FindConfig(ctx context.Context, projectID, domain string, locationCode int, locationName *string) (Config, error) {
	c, err := scanConfig(s.DB.QueryRow(ctx, `SELECT `+configColumns+` FROM go_rank_tracking_configs
		WHERE project_id = $1 AND domain = $2 AND location_code = $3
		AND location_name IS NOT DISTINCT FROM $4`, projectID, domain, locationCode, locationName))
	if errors.Is(err, pgx.ErrNoRows) {
		return Config{}, ErrNotFound
	}
	if err != nil {
		return Config{}, fmt.Errorf("find rank tracking config: %w", err)
	}
	return c, nil
}

// InsertConfig stores a new config. A clash with the market key is ErrDuplicate.
func (s Store) InsertConfig(ctx context.Context, c Config) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO go_rank_tracking_configs
		(id, project_id, domain, location_code, language_code, location_name, devices, serp_depth,
		 schedule_interval, is_active, next_check_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		c.ID, c.ProjectID, c.Domain, c.LocationCode, c.LanguageCode, c.LocationName, c.Devices,
		c.SerpDepth, c.ScheduleInterval, c.IsActive, c.NextCheckAt)
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("insert rank tracking config: %w", err)
	}
	return nil
}

// UpdateConfig changes the listed columns of a project's config.
func (s Store) UpdateConfig(ctx context.Context, projectID, id string, u ConfigUpdate) error {
	var sets []string
	args := []any{id, projectID}
	set := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	if u.Domain != nil {
		set("domain", *u.Domain)
	}
	if u.LocationCode != nil {
		set("location_code", *u.LocationCode)
	}
	if u.LanguageCode != nil {
		set("language_code", *u.LanguageCode)
	}
	if u.Devices != nil {
		set("devices", *u.Devices)
	}
	if u.SerpDepth != nil {
		set("serp_depth", *u.SerpDepth)
	}
	if u.IsActive != nil {
		set("is_active", *u.IsActive)
	}
	if u.ScheduleInterval != nil {
		set("schedule_interval", string(*u.ScheduleInterval))
	}
	if u.SetLocationName {
		set("location_name", u.LocationName)
	}
	if u.SetNextCheckAt {
		set("next_check_at", u.NextCheckAt)
	}
	if u.SetSkipReason {
		set("last_skip_reason", u.LastSkipReason)
	}
	if len(sets) == 0 {
		return nil
	}
	tag, err := s.DB.Exec(ctx, `UPDATE go_rank_tracking_configs SET `+strings.Join(sets, ", ")+
		` WHERE id = $1 AND project_id = $2`, args...)
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("update rank tracking config %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Keywords lists a config's keywords, oldest first.
func (s Store) Keywords(ctx context.Context, configID string) ([]Keyword, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+keywordColumns+` FROM go_rank_tracking_keywords
		WHERE config_id = $1 ORDER BY created_at, id`, configID)
	if err != nil {
		return nil, fmt.Errorf("list rank tracking keywords: %w", err)
	}
	keywords, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Keyword, error) { return scanKeyword(r) })
	if err != nil {
		return nil, fmt.Errorf("read rank tracking keywords: %w", err)
	}
	return keywords, nil
}

// UpdateKeywordMetrics atomically updates metric fields for rows still owned
// by the requested config. A deleted or moved row is not counted as updated.
func (s Store) UpdateKeywordMetrics(ctx context.Context, configID string, updates []KeywordMetricUpdate) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}
	body, err := json.Marshal(updates)
	if err != nil {
		return 0, fmt.Errorf("encode rank tracking metrics: %w", err)
	}
	tag, err := s.DB.Exec(ctx, `WITH updates AS (
		SELECT * FROM jsonb_to_recordset($2::jsonb) AS x(
			id text, search_volume integer, keyword_difficulty integer, cpc double precision, fetched_at timestamptz))
		UPDATE go_rank_tracking_keywords AS k SET
			search_volume = u.search_volume,
			keyword_difficulty = u.keyword_difficulty,
			cpc = u.cpc,
			metrics_fetched_at = u.fetched_at
		FROM updates AS u
		WHERE k.config_id = $1 AND k.id = u.id`, configID, body)
	if err != nil {
		return 0, fmt.Errorf("update rank tracking keyword metrics: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// AddKeywords inserts keywords that are not stored yet and returns the ids it
// added. It locks the config row so two requests cannot together pass
// maxKeywords, and keeps only as many keywords as still fit.
func (s Store) AddKeywords(ctx context.Context, configID string, keywords []NewKeyword, maxKeywords int) ([]string, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin add keywords: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM go_rank_tracking_configs WHERE id = $1 FOR UPDATE`, configID).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lock rank tracking config: %w", err)
	}
	ids, texts, cases := make([]string, 0, len(keywords)), make([]string, 0, len(keywords)), make([]bool, 0, len(keywords))
	for _, k := range keywords {
		ids, texts, cases = append(ids, k.ID), append(texts, k.Keyword), append(cases, k.MatchCase)
	}
	// ON CONFLICT skips keywords another request stored first; the LIMIT keeps
	// the table at or below maxKeywords.
	rows, err := tx.Query(ctx, `INSERT INTO go_rank_tracking_keywords (id, config_id, keyword, match_case)
		SELECT t.id, $1, t.keyword, t.match_case
		FROM unnest($2::text[], $3::text[], $4::boolean[]) WITH ORDINALITY AS t(id, keyword, match_case, n)
		ORDER BY t.n
		LIMIT GREATEST($5 - (SELECT count(*) FROM go_rank_tracking_keywords WHERE config_id = $1), 0)
		ON CONFLICT (config_id, keyword) DO NOTHING
		RETURNING id`, configID, ids, texts, cases, maxKeywords)
	if err != nil {
		return nil, fmt.Errorf("insert rank tracking keywords: %w", err)
	}
	added, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("read inserted keywords: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit add keywords: %w", err)
	}
	return added, nil
}

// RemoveKeywords deletes keywords of the config and returns the ids it removed.
// History stays: snapshots do not reference keywords by foreign key.
func (s Store) RemoveKeywords(ctx context.Context, configID string, ids []string) ([]string, error) {
	rows, err := s.DB.Query(ctx, `DELETE FROM go_rank_tracking_keywords
		WHERE config_id = $1 AND id = ANY($2) RETURNING id`, configID, ids)
	if err != nil {
		return nil, fmt.Errorf("delete rank tracking keywords: %w", err)
	}
	removed, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("read deleted keywords: %w", err)
	}
	return removed, nil
}
