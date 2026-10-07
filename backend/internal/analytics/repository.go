package analytics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// topLimit bounds the ranked lists in a summary.
const topLimit = 50

type repository struct {
	db *pgxpool.Pool
}

// ensureSite returns the site key of projectID's site, creating the site
// with newKey when there is none. The no-op update makes RETURNING yield the
// existing row on conflict, so concurrent callers all get the same key.
func (r repository) ensureSite(ctx context.Context, projectID, newKey string) (string, error) {
	var key string
	err := r.db.QueryRow(ctx, `
		INSERT INTO go_analytics_sites (project_id, site_key) VALUES ($1, $2)
		ON CONFLICT (project_id) DO UPDATE SET project_id = EXCLUDED.project_id
		RETURNING site_key`,
		projectID, newKey,
	).Scan(&key)
	if err != nil {
		return "", fmt.Errorf("ensure analytics site: %w", err)
	}
	return key, nil
}

// site is a tracked site and its project's domain ("" when unset).
type site struct {
	ID     int64
	Domain string
}

// siteByKey returns the site with siteKey whose project is active, or
// ErrUnknownSite.
func (r repository) siteByKey(ctx context.Context, siteKey string) (site, error) {
	var s site
	err := r.db.QueryRow(ctx, `
		SELECT s.id, coalesce(p.domain, '')
		FROM go_analytics_sites s
		JOIN projects p ON p.id = s.project_id
		WHERE s.site_key = $1 AND p.archived_at IS NULL`,
		siteKey,
	).Scan(&s.ID, &s.Domain)
	if errors.Is(err, pgx.ErrNoRows) {
		return site{}, ErrUnknownSite
	}
	if err != nil {
		return site{}, fmt.Errorf("load analytics site: %w", err)
	}
	return s, nil
}

type event struct {
	SiteID      int64
	OccurredAt  time.Time
	Path        string
	Source      source
	Device      string
	VisitorHash []byte
}

func (r repository) insertEvent(ctx context.Context, e event) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO go_analytics_events
			(site_id, occurred_at, path, referrer_host, channel, ai_source, device, visitor_hash)
		VALUES ($1, $2, $3, nullif($4, ''), $5, nullif($6, ''), $7, $8)`,
		e.SiteID, e.OccurredAt, e.Path, e.Source.ReferrerHost, e.Source.Channel, e.Source.AISource, e.Device, e.VisitorHash,
	)
	if err != nil {
		return fmt.Errorf("insert analytics event: %w", err)
	}
	return nil
}

// eventsInRange selects the project's events from $2 (inclusive) to $3
// (exclusive). A project without a site matches no events.
const eventsInRange = `
	FROM go_analytics_events
	WHERE site_id = (SELECT id FROM go_analytics_sites WHERE project_id = $1)
		AND occurred_at >= $2 AND occurred_at < $3`

// summary aggregates the project's events in [start, end), whole UTC days,
// with every query sent in one round trip.
func (r repository) summary(ctx context.Context, projectID string, start, end time.Time) (Summary, error) {
	s := Summary{}
	args := []any{projectID, start, end}
	b := &pgx.Batch{}

	b.Queue(`SELECT count(DISTINCT visitor_hash), count(*)`+eventsInRange, args...).
		QueryRow(func(row pgx.Row) error { return row.Scan(&s.Visitors, &s.Pageviews) })

	b.Queue(`
		WITH daily AS (
			SELECT (occurred_at AT TIME ZONE 'UTC')::date AS day,
				count(DISTINCT visitor_hash) AS visitors, count(*) AS pageviews`+eventsInRange+`
			GROUP BY 1
		)
		SELECT to_char(d, 'YYYY-MM-DD'), coalesce(daily.visitors, 0), coalesce(daily.pageviews, 0)
		FROM generate_series($2 AT TIME ZONE 'UTC', $3 AT TIME ZONE 'UTC' - interval '1 day', interval '1 day') AS d
		LEFT JOIN daily ON daily.day = d::date
		ORDER BY d`, args...).
		Query(collectInto(&s.Series, pgx.RowToStructByPos[DayCount]))

	b.Queue(`SELECT path, count(*)`+eventsInRange+`
		GROUP BY path ORDER BY 2 DESC, path LIMIT $4`, append(args, topLimit)...).
		Query(collectInto(&s.TopPages, pgx.RowToStructByPos[PageCount]))

	b.Queue(`SELECT channel, count(DISTINCT visitor_hash)`+eventsInRange+`
		AND channel <> $4
		GROUP BY channel ORDER BY 2 DESC, channel`, append(args, channelInternal)...).
		Query(collectInto(&s.Channels, pgx.RowToStructByPos[ChannelCount]))

	b.Queue(`SELECT ai_source, count(DISTINCT visitor_hash)`+eventsInRange+`
		AND ai_source IS NOT NULL
		GROUP BY ai_source ORDER BY 2 DESC, ai_source`, args...).
		Query(collectInto(&s.AISources, pgx.RowToStructByPos[AISourceCount]))

	b.Queue(`SELECT referrer_host, count(DISTINCT visitor_hash)`+eventsInRange+`
		AND referrer_host IS NOT NULL
		GROUP BY referrer_host ORDER BY 2 DESC, referrer_host LIMIT $4`, append(args, topLimit)...).
		Query(collectInto(&s.Referrers, pgx.RowToStructByPos[ReferrerCount]))

	b.Queue(`SELECT device, count(DISTINCT visitor_hash)`+eventsInRange+`
		GROUP BY device ORDER BY 2 DESC, device`, args...).
		Query(collectInto(&s.Devices, pgx.RowToStructByPos[DeviceCount]))

	if err := r.db.SendBatch(ctx, b).Close(); err != nil {
		return Summary{}, fmt.Errorf("query analytics summary: %w", err)
	}
	return s, nil
}

// collectInto returns a batch callback that collects every row into dst.
func collectInto[T any](dst *[]T, fn pgx.RowToFunc[T]) func(pgx.Rows) error {
	return func(rows pgx.Rows) error {
		var err error
		*dst, err = pgx.CollectRows(rows, fn)
		return err
	}
}
