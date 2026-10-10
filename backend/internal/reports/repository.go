package reports

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	List(context.Context, string, int, int) ([]Metadata, error)
	Count(context.Context, string) (int, error)
	Get(context.Context, string, string) (Metadata, error)
	GetHTML(context.Context, string, string) (Metadata, string, error)
	FindTitle(context.Context, string, string) (string, error)
	OrganizationBytes(context.Context, string) (int64, error)
	Insert(context.Context, Metadata, string) error
	Update(context.Context, Metadata, string) error
	Delete(context.Context, string, string) (bool, error)
	SetShare(context.Context, string, string, *string, *string) (bool, error)
	GetShared(context.Context, string) (SharedReport, error)
	ListTemplates(context.Context, string) ([]Template, error)
	GetTemplate(context.Context, string, string) (Template, error)
	InsertTemplate(context.Context, Template) error
	UpdateTemplate(context.Context, Template) (bool, error)
	DeleteTemplate(context.Context, string, string) (bool, error)
}

type Repository struct{ DB *pgxpool.Pool }

const metadataColumns = `id, project_id, title, summary, skill, template_id, created_by, created_by_user_id, size_bytes, share_token, shared_at, created_at, updated_at`

func (r Repository) List(ctx context.Context, projectID string, limit, offset int) ([]Metadata, error) {
	rows, err := r.DB.Query(ctx, `SELECT `+metadataColumns+` FROM reports WHERE project_id=$1 ORDER BY updated_at DESC, id DESC LIMIT $2 OFFSET $3`, projectID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	defer rows.Close()
	result := make([]Metadata, 0)
	for rows.Next() {
		var item Metadata
		if err := scanMetadata(rows, &item); err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reports: %w", err)
	}
	return result, nil
}

func (r Repository) Count(ctx context.Context, projectID string) (int, error) {
	var count int
	if err := r.DB.QueryRow(ctx, `SELECT count(*) FROM reports WHERE project_id=$1`, projectID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count reports: %w", err)
	}
	return count, nil
}

func (r Repository) Get(ctx context.Context, projectID, id string) (Metadata, error) {
	var item Metadata
	err := scanMetadata(r.DB.QueryRow(ctx, `SELECT `+metadataColumns+` FROM reports WHERE project_id=$1 AND id=$2`, projectID, id), &item)
	if errors.Is(err, pgx.ErrNoRows) {
		return Metadata{}, ErrNotFound
	}
	if err != nil {
		return Metadata{}, fmt.Errorf("get report: %w", err)
	}
	return item, nil
}

func (r Repository) GetHTML(ctx context.Context, projectID, id string) (Metadata, string, error) {
	var item Metadata
	var html string
	err := scanMetadataWithHTML(r.DB.QueryRow(ctx, `SELECT `+metadataColumns+`, html FROM reports WHERE project_id=$1 AND id=$2`, projectID, id), &item, &html)
	if errors.Is(err, pgx.ErrNoRows) {
		return Metadata{}, "", ErrNotFound
	}
	if err != nil {
		return Metadata{}, "", fmt.Errorf("get report HTML: %w", err)
	}
	return item, html, nil
}

func (r Repository) FindTitle(ctx context.Context, projectID, title string) (string, error) {
	var id string
	err := r.DB.QueryRow(ctx, `SELECT id FROM reports WHERE project_id=$1 AND title=$2 LIMIT 1`, projectID, title).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find report title: %w", err)
	}
	return id, nil
}

func (r Repository) OrganizationBytes(ctx context.Context, organizationID string) (int64, error) {
	var total int64
	err := r.DB.QueryRow(ctx, `SELECT COALESCE(sum(r.size_bytes), 0)::bigint FROM reports r JOIN projects p ON p.id=r.project_id WHERE p.organization_id=$1`, organizationID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("sum organization report bytes: %w", err)
	}
	return total, nil
}

func (r Repository) Insert(ctx context.Context, item Metadata, html string) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO reports (`+metadataColumns+`, html) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, item.ID, item.ProjectID, item.Title, item.Summary, item.Skill, item.TemplateID, item.CreatedBy, item.CreatedByUserID, item.SizeBytes, item.ShareToken, item.SharedAt, item.CreatedAt, item.UpdatedAt, html)
	if err != nil {
		return fmt.Errorf("insert report: %w", err)
	}
	return nil
}

func (r Repository) Update(ctx context.Context, item Metadata, html string) error {
	tag, err := r.DB.Exec(ctx, `UPDATE reports SET title=$3, summary=$4, html=$5, skill=$6, template_id=$7, size_bytes=$8, updated_at=$9 WHERE project_id=$1 AND id=$2`, item.ProjectID, item.ID, item.Title, item.Summary, html, item.Skill, item.TemplateID, item.SizeBytes, item.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update report: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r Repository) Delete(ctx context.Context, projectID, id string) (bool, error) {
	tag, err := r.DB.Exec(ctx, `DELETE FROM reports WHERE project_id=$1 AND id=$2`, projectID, id)
	if err != nil {
		return false, fmt.Errorf("delete report: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r Repository) SetShare(ctx context.Context, projectID, id string, token, sharedAt *string) (bool, error) {
	query := `UPDATE reports SET share_token=$3, shared_at=$4 WHERE project_id=$1 AND id=$2`
	if token != nil {
		query += ` AND share_token IS NULL`
	}
	tag, err := r.DB.Exec(ctx, query, projectID, id, token, sharedAt)
	if err != nil {
		return false, fmt.Errorf("set report sharing: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r Repository) GetShared(ctx context.Context, token string) (SharedReport, error) {
	var item SharedReport
	err := r.DB.QueryRow(ctx, `SELECT r.id,r.project_id,r.title,r.summary,r.skill,r.template_id,r.created_by,r.created_by_user_id,r.size_bytes,r.share_token,r.shared_at,r.created_at,r.updated_at,r.html,p.organization_id,p.domain,(p.archived_at IS NOT NULL) FROM reports r JOIN projects p ON p.id=r.project_id WHERE r.share_token=$1`, token).Scan(
		&item.ID, &item.ProjectID, &item.Title, &item.Summary, &item.Skill, &item.TemplateID, &item.CreatedBy, &item.CreatedByUserID, &item.SizeBytes, &item.ShareToken, &item.SharedAt, &item.CreatedAt, &item.UpdatedAt, &item.HTML, &item.OrganizationID, &item.ProjectDomain, &item.Archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return SharedReport{}, ErrNotFound
	}
	if err != nil {
		return SharedReport{}, fmt.Errorf("get shared report: %w", err)
	}
	return item, nil
}

func (r Repository) ListTemplates(ctx context.Context, projectID string) ([]Template, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,project_id,name,description,instructions,created_by,created_by_user_id,created_at,updated_at FROM report_templates WHERE project_id=$1 ORDER BY name ASC,id ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list report templates: %w", err)
	}
	defer rows.Close()
	result := make([]Template, 0)
	for rows.Next() {
		var item Template
		if err := scanTemplate(rows, &item); err != nil {
			return nil, fmt.Errorf("scan report template: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report templates: %w", err)
	}
	return result, nil
}

func (r Repository) GetTemplate(ctx context.Context, projectID, id string) (Template, error) {
	var item Template
	err := scanTemplate(r.DB.QueryRow(ctx, `SELECT id,project_id,name,description,instructions,created_by,created_by_user_id,created_at,updated_at FROM report_templates WHERE project_id=$1 AND id=$2`, projectID, id), &item)
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("get report template: %w", err)
	}
	return item, nil
}

func (r Repository) InsertTemplate(ctx context.Context, item Template) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO report_templates (id,project_id,name,description,instructions,created_by,created_by_user_id,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, item.ID, item.ProjectID, item.Name, item.Description, item.Instructions, item.CreatedBy, item.CreatedByUserID, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert report template: %w", err)
	}
	return nil
}

func (r Repository) UpdateTemplate(ctx context.Context, item Template) (bool, error) {
	tag, err := r.DB.Exec(ctx, `UPDATE report_templates SET name=$3,description=$4,instructions=$5,updated_at=$6 WHERE project_id=$1 AND id=$2`, item.ProjectID, item.ID, item.Name, item.Description, item.Instructions, item.UpdatedAt)
	if err != nil {
		return false, fmt.Errorf("update report template: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r Repository) DeleteTemplate(ctx context.Context, projectID, id string) (bool, error) {
	tag, err := r.DB.Exec(ctx, `DELETE FROM report_templates WHERE project_id=$1 AND id=$2`, projectID, id)
	if err != nil {
		return false, fmt.Errorf("delete report template: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

type scanner interface{ Scan(...any) error }

func scanMetadata(row scanner, item *Metadata) error {
	return row.Scan(&item.ID, &item.ProjectID, &item.Title, &item.Summary, &item.Skill, &item.TemplateID, &item.CreatedBy, &item.CreatedByUserID, &item.SizeBytes, &item.ShareToken, &item.SharedAt, &item.CreatedAt, &item.UpdatedAt)
}
func scanMetadataWithHTML(row scanner, item *Metadata, html *string) error {
	return row.Scan(&item.ID, &item.ProjectID, &item.Title, &item.Summary, &item.Skill, &item.TemplateID, &item.CreatedBy, &item.CreatedByUserID, &item.SizeBytes, &item.ShareToken, &item.SharedAt, &item.CreatedAt, &item.UpdatedAt, html)
}
func scanTemplate(row scanner, item *Template) error {
	return row.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Description, &item.Instructions, &item.CreatedBy, &item.CreatedByUserID, &item.CreatedAt, &item.UpdatedAt)
}
