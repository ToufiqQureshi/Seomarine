package keywords

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
)

// SavedRepository is the Postgres store of saved keywords, tags and metrics.
type SavedRepository struct{ DB *pgxpool.Pool }

const metricJoin = `LEFT JOIN go_keyword_metrics m ON m.project_id = k.project_id AND m.keyword = k.keyword
	AND m.location_code = k.location_code AND m.language_code = k.language_code`

// Save stores metrics, keywords and tags in one transaction, so a failure
// never leaves metrics saved for keywords that were not.
func (r SavedRepository) Save(ctx context.Context, in SaveInput) ([]string, error) {
	var saved []string
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		if err := upsertMetrics(ctx, tx, in.ProjectID, in.LocationCode, in.LanguageCode, in.Metrics); err != nil {
			return err
		}
		newIDs := make([]string, len(in.Keywords))
		for i := range newIDs {
			id, err := ids.New()
			if err != nil {
				return err
			}
			newIDs[i] = id
		}
		if _, err := tx.Exec(ctx, `INSERT INTO go_saved_keywords (id, project_id, keyword, location_code, language_code)
			SELECT t.id, $1, t.keyword, $2, $3 FROM unnest($4::text[], $5::text[]) AS t(id, keyword)
			ON CONFLICT (project_id, keyword, location_code, language_code) DO NOTHING`,
			in.ProjectID, in.LocationCode, in.LanguageCode, newIDs, in.Keywords); err != nil {
			return fmt.Errorf("save keywords: %w", err)
		}
		rows, err := tx.Query(ctx, `SELECT keyword, id FROM go_saved_keywords
			WHERE project_id = $1 AND location_code = $2 AND language_code = $3 AND keyword = ANY($4)`,
			in.ProjectID, in.LocationCode, in.LanguageCode, in.Keywords)
		if err != nil {
			return fmt.Errorf("read saved keyword ids: %w", err)
		}
		byKeyword := map[string]string{}
		for rows.Next() {
			var keyword, id string
			if err := rows.Scan(&keyword, &id); err != nil {
				return fmt.Errorf("scan saved keyword id: %w", err)
			}
			byKeyword[keyword] = id
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read saved keyword ids: %w", err)
		}
		for _, k := range in.Keywords {
			saved = append(saved, byKeyword[k])
		}
		if len(in.Tags) == 0 {
			return nil
		}
		assigned, err := addTags(ctx, tx, in.ProjectID, saved, in.Tags)
		if err != nil {
			return err
		}
		if in.ReplaceTags {
			keep := make([]string, len(assigned.Tags))
			for i, t := range assigned.Tags {
				keep[i] = t.ID
			}
			if _, err := tx.Exec(ctx, `DELETE FROM go_saved_keyword_tag_assignments
				WHERE saved_keyword_id = ANY($1) AND NOT (tag_id = ANY($2))`, saved, keep); err != nil {
				return fmt.Errorf("replace keyword tags: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

func nonNilMonths(m []MonthlySearch) []MonthlySearch {
	if m == nil {
		return []MonthlySearch{}
	}
	return m
}

func (r SavedRepository) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin keyword transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit keyword transaction: %w", err)
	}
	return nil
}

// addTags creates missing tags and labels the project's saved keywords with
// them. Ids that are not the project's saved keywords are ignored.
func addTags(ctx context.Context, tx pgx.Tx, projectID string, savedIDs, tagNames []string) (TagAssignment, error) {
	existing, err := ownedKeywordIDs(ctx, tx, projectID, savedIDs)
	if err != nil || len(existing) == 0 {
		return TagAssignment{}, err
	}
	names, normalized := make([]string, len(tagNames)), make([]string, len(tagNames))
	for i, n := range tagNames {
		name, norm, _ := NormalizeTag(n)
		names[i], normalized[i] = name, norm
	}
	newIDs := make([]string, len(names))
	for i := range newIDs {
		if newIDs[i], err = ids.New(); err != nil {
			return TagAssignment{}, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO go_saved_keyword_tags (id, project_id, name, normalized_name)
		SELECT t.id, $1, t.name, t.normalized FROM unnest($2::text[], $3::text[], $4::text[]) AS t(id, name, normalized)
		ON CONFLICT (project_id, normalized_name) DO NOTHING`, projectID, newIDs, names, normalized); err != nil {
		return TagAssignment{}, fmt.Errorf("create tags: %w", err)
	}
	tags, err := queryTags(ctx, tx, `SELECT id, name, normalized_name, color FROM go_saved_keyword_tags
		WHERE project_id = $1 AND normalized_name = ANY($2) ORDER BY normalized_name`, projectID, normalized)
	if err != nil {
		return TagAssignment{}, err
	}
	tagIDs := make([]string, len(tags))
	for i, t := range tags {
		tagIDs[i] = t.ID
	}
	if _, err := tx.Exec(ctx, `INSERT INTO go_saved_keyword_tag_assignments (saved_keyword_id, tag_id)
		SELECT s.id, t.id FROM unnest($1::text[]) AS s(id) CROSS JOIN unnest($2::text[]) AS t(id)
		ON CONFLICT DO NOTHING`, existing, tagIDs); err != nil {
		return TagAssignment{}, fmt.Errorf("assign tags: %w", err)
	}
	return TagAssignment{SavedKeywordCount: len(existing), Tags: tags}, nil
}

func ownedKeywordIDs(ctx context.Context, q pgx.Tx, projectID string, savedIDs []string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT id FROM go_saved_keywords WHERE project_id = $1 AND id = ANY($2)`, projectID, savedIDs)
	if err != nil {
		return nil, fmt.Errorf("find saved keywords: %w", err)
	}
	owned, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("read saved keywords: %w", err)
	}
	return owned, nil
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func queryTags(ctx context.Context, q querier, sql string, args ...any) ([]Tag, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	tags, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Tag, error) {
		var t Tag
		err := r.Scan(&t.ID, &t.Name, &t.NormalizedName, &t.Color)
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("read tags: %w", err)
	}
	return tags, nil
}

// AddTags labels saved keywords with the named tags, creating any that are new.
func (r SavedRepository) AddTags(ctx context.Context, projectID string, savedIDs, tagNames []string) (TagAssignment, error) {
	var out TagAssignment
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = addTags(ctx, tx, projectID, savedIDs, tagNames)
		return err
	})
	return out, err
}

// RemoveTags takes tags off saved keywords. Tags and keywords of other
// projects are ignored.
func (r SavedRepository) RemoveTags(ctx context.Context, projectID string, savedIDs, tagIDs []string) (TagAssignment, error) {
	var out TagAssignment
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		owned, err := ownedKeywordIDs(ctx, tx, projectID, savedIDs)
		if err != nil {
			return err
		}
		tags, err := queryTags(ctx, tx, `SELECT id, name, normalized_name, color FROM go_saved_keyword_tags
			WHERE project_id = $1 AND id = ANY($2) ORDER BY normalized_name`, projectID, tagIDs)
		if err != nil {
			return err
		}
		ownedTagIDs := make([]string, len(tags))
		for i, t := range tags {
			ownedTagIDs[i] = t.ID
		}
		tag, err := tx.Exec(ctx, `DELETE FROM go_saved_keyword_tag_assignments WHERE saved_keyword_id = ANY($1) AND tag_id = ANY($2)`, owned, ownedTagIDs)
		if err != nil {
			return fmt.Errorf("remove tags: %w", err)
		}
		out = TagAssignment{SavedKeywordCount: len(owned), Tags: tags, RemovedCount: int(tag.RowsAffected())}
		return nil
	})
	return out, err
}

// UpdateTag renames or recolors a tag of the project. It returns nil when the
// tag does not exist. Renaming onto another tag's name is ErrTagExists.
func (r SavedRepository) UpdateTag(ctx context.Context, projectID, tagID string, name *string, color **string) (*Tag, error) {
	sets, args := []string{}, []any{projectID, tagID}
	if name != nil {
		display, normalized, _ := NormalizeTag(*name)
		args = append(args, display, normalized)
		sets = append(sets, fmt.Sprintf("name = $%d, normalized_name = $%d", len(args)-1, len(args)))
	}
	if color != nil {
		args = append(args, *color)
		sets = append(sets, fmt.Sprintf("color = $%d", len(args)))
	}
	tags, err := queryTags(ctx, r.DB, `UPDATE go_saved_keyword_tags SET `+strings.Join(sets, ", ")+
		` WHERE project_id = $1 AND id = $2 RETURNING id, name, normalized_name, color`, args...)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
		return nil, ErrTagExists
	}
	if err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return nil, nil
	}
	return &tags[0], nil
}

// DeleteTag removes a tag nothing uses. The tag row is locked while it is
// counted, because adding an assignment takes a share lock on it: a keyword
// cannot be labelled between the count and the delete.
func (r SavedRepository) DeleteTag(ctx context.Context, projectID, tagID string) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `SELECT id FROM go_saved_keyword_tags WHERE project_id = $1 AND id = $2 FOR UPDATE`, projectID, tagID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTagNotFound
		}
		if err != nil {
			return fmt.Errorf("lock tag: %w", err)
		}
		var inUse int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM go_saved_keyword_tag_assignments WHERE tag_id = $1`, tagID).Scan(&inUse); err != nil {
			return fmt.Errorf("count tag assignments: %w", err)
		}
		if inUse > 0 {
			return &TagInUseError{AssignmentCount: inUse}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM go_saved_keyword_tags WHERE id = $1`, tagID); err != nil {
			return fmt.Errorf("delete tag: %w", err)
		}
		return nil
	})
}

// Remove deletes saved keywords of the project. Their tag assignments go with
// them; metrics stay as a cache.
func (r SavedRepository) Remove(ctx context.Context, projectID string, savedIDs []string) (int, error) {
	tag, err := r.DB.Exec(ctx, `DELETE FROM go_saved_keywords WHERE project_id = $1 AND id = ANY($2)`, projectID, savedIDs)
	if err != nil {
		return 0, fmt.Errorf("remove saved keywords: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

var sortColumns = map[string]string{
	"createdAt": "k.created_at", "keyword": "k.keyword", "searchVolume": "m.search_volume", "cpc": "m.cpc",
	"competition": "m.competition", "keywordDifficulty": "m.keyword_difficulty", "fetchedAt": "m.fetched_at",
}

// List returns the project's saved keywords with metrics and tags, plus every
// tag of the project with its keyword count. Missing metrics sort last in both
// directions, so unmeasured keywords never crowd the top of a sorted table.
func (r SavedRepository) List(ctx context.Context, q ListQuery) (ListResult, error) {
	tags, err := r.projectTags(ctx, q.ProjectID)
	if err != nil {
		return ListResult{}, err
	}
	tagIDs := slicesClone(q.TagIDs)
	if len(q.TagNames) > 0 {
		var normalized []string
		for _, n := range NormalizeTags(q.TagNames) {
			_, norm, _ := NormalizeTag(n)
			normalized = append(normalized, norm)
		}
		rows, err := r.DB.Query(ctx, `SELECT id FROM go_saved_keyword_tags WHERE project_id = $1 AND normalized_name = ANY($2)`, q.ProjectID, normalized)
		if err != nil {
			return ListResult{}, fmt.Errorf("resolve tag names: %w", err)
		}
		named, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return ListResult{}, fmt.Errorf("read tag names: %w", err)
		}
		if len(normalized) > 0 && len(named) == 0 && len(q.TagIDs) == 0 {
			return ListResult{Rows: []SavedKeyword{}, Tags: tags}, nil // names that match no tag match no keyword
		}
		tagIDs = append(tagIDs, named...)
	}

	where, args := []string{"k.project_id = $1"}, []any{q.ProjectID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if q.Search != "" {
		add(`lower(k.keyword) LIKE $%d ESCAPE '\'`, "%"+escapeLike(strings.ToLower(q.Search))+"%")
	}
	for _, t := range q.IncludeTerms {
		add(`lower(k.keyword) LIKE $%d ESCAPE '\'`, "%"+escapeLike(strings.ToLower(t))+"%")
	}
	for _, t := range q.ExcludeTerms {
		add(`lower(k.keyword) NOT LIKE $%d ESCAPE '\'`, "%"+escapeLike(strings.ToLower(t))+"%")
	}
	if q.MinVolume != nil {
		add("m.search_volume >= $%d", *q.MinVolume)
	}
	if q.MaxVolume != nil {
		add("m.search_volume <= $%d", *q.MaxVolume)
	}
	if q.MinCPC != nil {
		add("m.cpc >= $%d", *q.MinCPC)
	}
	if q.MaxCPC != nil {
		add("m.cpc <= $%d", *q.MaxCPC)
	}
	if q.MinDifficulty != nil {
		add("m.keyword_difficulty >= $%d", *q.MinDifficulty)
	}
	if q.MaxDifficulty != nil {
		add("m.keyword_difficulty <= $%d", *q.MaxDifficulty)
	}
	if len(tagIDs) > 0 {
		add(`EXISTS (SELECT 1 FROM go_saved_keyword_tag_assignments a WHERE a.saved_keyword_id = k.id AND a.tag_id = ANY($%d))`, tagIDs)
	}
	filter := strings.Join(where, " AND ")

	var total int
	if err := r.DB.QueryRow(ctx, `SELECT count(*) FROM go_saved_keywords k `+metricJoin+` WHERE `+filter, args...).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("count saved keywords: %w", err)
	}
	direction := "ASC"
	if q.Descending {
		direction = "DESC"
	}
	sql := `SELECT k.id, k.project_id, k.keyword, k.location_code, k.language_code, k.created_at,
			m.search_volume, m.cpc, m.competition, m.keyword_difficulty, m.intent, m.monthly_searches, m.fetched_at
		FROM go_saved_keywords k ` + metricJoin + ` WHERE ` + filter +
		` ORDER BY ` + sortColumns[q.Sort] + ` ` + direction + ` NULLS LAST, k.id`
	if q.PageSize > 0 {
		page := max(q.Page, 1)
		args = append(args, q.PageSize, (page-1)*q.PageSize)
		sql += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	}
	rows, err := r.DB.Query(ctx, sql, args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list saved keywords: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SavedKeyword, error) {
		var k SavedKeyword
		var created time.Time
		var fetched *time.Time
		var months []byte
		err := row.Scan(&k.ID, &k.ProjectID, &k.Keyword, &k.LocationCode, &k.LanguageCode, &created, &k.SearchVolume, &k.CPC,
			&k.Competition, &k.KeywordDifficulty, &k.Intent, &months, &fetched)
		k.CreatedAt = created.UTC().Format(time.RFC3339Nano)
		if fetched != nil {
			s := fetched.UTC().Format(time.RFC3339Nano)
			k.FetchedAt = &s
		}
		// A malformed value reads as no monthly data instead of failing the list.
		if json.Unmarshal(months, &k.MonthlySearches) != nil || k.MonthlySearches == nil {
			k.MonthlySearches = []MonthlySearch{}
		}
		k.Tags = []Tag{}
		return k, err
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("read saved keywords: %w", err)
	}
	if err := r.attachTags(ctx, q.ProjectID, list); err != nil {
		return ListResult{}, err
	}
	return ListResult{Rows: list, TotalCount: total, Tags: tags}, nil
}

func slicesClone(s []string) []string { return append([]string(nil), s...) }

func (r SavedRepository) projectTags(ctx context.Context, projectID string) ([]TagSummary, error) {
	rows, err := r.DB.Query(ctx, `SELECT t.id, t.name, t.normalized_name, t.color, count(a.saved_keyword_id)::int
		FROM go_saved_keyword_tags t LEFT JOIN go_saved_keyword_tag_assignments a ON a.tag_id = t.id
		WHERE t.project_id = $1 GROUP BY t.id ORDER BY t.normalized_name`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	tags, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (TagSummary, error) {
		var t TagSummary
		err := row.Scan(&t.ID, &t.Name, &t.NormalizedName, &t.Color, &t.KeywordCount)
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("read tags: %w", err)
	}
	return tags, nil
}

func (r SavedRepository) attachTags(ctx context.Context, projectID string, list []SavedKeyword) error {
	if len(list) == 0 {
		return nil
	}
	index := make(map[string]int, len(list))
	savedIDs := make([]string, len(list))
	for i, k := range list {
		index[k.ID], savedIDs[i] = i, k.ID
	}
	rows, err := r.DB.Query(ctx, `SELECT a.saved_keyword_id, t.id, t.name, t.normalized_name, t.color
		FROM go_saved_keyword_tag_assignments a JOIN go_saved_keyword_tags t ON t.id = a.tag_id
		WHERE t.project_id = $1 AND a.saved_keyword_id = ANY($2) ORDER BY t.normalized_name`, projectID, savedIDs)
	if err != nil {
		return fmt.Errorf("list keyword tags: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var keywordID string
		var t Tag
		if err := rows.Scan(&keywordID, &t.ID, &t.Name, &t.NormalizedName, &t.Color); err != nil {
			return fmt.Errorf("read keyword tags: %w", err)
		}
		list[index[keywordID]].Tags = append(list[index[keywordID]].Tags, t)
	}
	return rows.Err()
}

type batcher interface {
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

const upsertMetricSQL = `INSERT INTO go_keyword_metrics
		(project_id, keyword, location_code, language_code, search_volume, cpc, competition, keyword_difficulty, intent, monthly_searches)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	ON CONFLICT (project_id, keyword, location_code, language_code) DO UPDATE SET
		search_volume = EXCLUDED.search_volume, cpc = EXCLUDED.cpc, competition = EXCLUDED.competition,
		keyword_difficulty = EXCLUDED.keyword_difficulty, intent = EXCLUDED.intent,
		monthly_searches = EXCLUDED.monthly_searches, fetched_at = now()`

func upsertMetrics(ctx context.Context, db batcher, projectID string, locationCode int, languageCode string, metrics []Metric) error {
	if len(metrics) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, m := range metrics {
		months, err := json.Marshal(nonNilMonths(m.MonthlySearches))
		if err != nil {
			return fmt.Errorf("encode monthly searches: %w", err)
		}
		batch.Queue(upsertMetricSQL, projectID, m.Keyword, locationCode, languageCode, m.SearchVolume, m.CPC, m.Competition,
			m.KeywordDifficulty, m.Intent, months)
	}
	results := db.SendBatch(ctx, batch)
	for range metrics {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("upsert keyword metric: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("upsert keyword metrics: %w", err)
	}
	return nil
}

// UpsertMetrics stores the latest metrics of keywords in a market, all or none.
func (r SavedRepository) UpsertMetrics(ctx context.Context, projectID string, locationCode int, languageCode string, rows []Metric) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		return upsertMetrics(ctx, tx, projectID, locationCode, languageCode, rows)
	})
}
