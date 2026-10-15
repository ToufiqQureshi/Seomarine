// Package sitepublish connects a project to the customer's website and
// publishes content-editor drafts to it. WordPress is the only platform today.
package sitepublish

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/toufiqqureshi/uplumo/backend/internal/contenteditor"
)

// MaxImagesPerArticle caps how many images one publish copies to the site,
// so a single request stays well inside its time budget.
const MaxImagesPerArticle = 10

// Errors the handler turns into answers a customer can act on.
var (
	ErrNotConnected      = errors.New("no website is connected to this project")
	ErrNotHTTPS          = errors.New("the website address must start with https://")
	ErrSiteUnreachable   = errors.New("the website did not answer")
	ErrNotWordPress      = errors.New("no WordPress REST API found at this address")
	ErrSiteBlocked       = errors.New("the website's firewall blocked the request")
	ErrAppPasswordsOff   = errors.New("application passwords are turned off on this site")
	ErrBadCredentials    = errors.New("WordPress refused the username or application password")
	ErrAuthHeaderDropped = errors.New("the website's hosting removed the login header")
	ErrMissingPermission = errors.New("this WordPress user cannot publish posts and upload images")
	ErrRemoteMissing     = errors.New("the published post no longer exists on the site")
	ErrKeywordStuffing   = errors.New("the draft repeats its target topic too often to publish")
)

// ValidationError is input the customer must correct.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

// WordPressPlatform is the only platform a project can connect today.
const WordPressPlatform = "wordpress"

// Publish statuses a customer can choose.
const (
	StatusDraft   = "draft"
	StatusPublish = "publish"
)

// Connection is a project's website, with the secret still encrypted.
type Connection struct {
	ProjectID        string
	Platform         string
	SiteURL          string
	APIBase          string
	Username         string
	SecretCiphertext string
	SiteName         string
	ConnectedBy      string
	CreatedAt        time.Time
}

// Publication records where a draft was published.
type Publication struct {
	DraftID      string
	ProjectID    string
	SiteURL      string
	RemoteID     string
	RemoteURL    string
	RemoteStatus string
	Media        map[string]Media
	PublishedAt  time.Time
	UpdatedAt    time.Time
}

// Store persists connections and publications.
type Store interface {
	Connection(ctx context.Context, projectID string) (Connection, error)
	SaveConnection(ctx context.Context, connection Connection) error
	DeleteConnection(ctx context.Context, projectID string) error
	Publication(ctx context.Context, projectID, draftID string) (Publication, error)
	SavePublication(ctx context.Context, publication Publication) error
}

// Drafts reads content-editor drafts. contenteditor.Repository satisfies it.
type Drafts interface {
	Get(ctx context.Context, projectID, id string) (contenteditor.Draft, error)
}

// Cipher encrypts the site secret at rest. google.TokenCipher satisfies it.
type Cipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// Service holds the rules for connecting and publishing.
type Service struct {
	Store     Store
	Drafts    Drafts
	Cipher    Cipher
	WordPress WordPress
	// Authors supplies the author profile a byline and Article schema are
	// built from; nil means drafts are published without an author.
	Authors AuthorStore
}

// AuthorStore loads one project's author profile.
type AuthorStore interface {
	Author(ctx context.Context, projectID, authorID string) (AuthorInfo, error)
}

// publishedAuthor loads the author a draft names, never leaking another
// project's profile (the store is project-scoped).
func (s *Service) publishedAuthor(ctx context.Context, projectID, authorID string) (AuthorInfo, bool) {
	if s.Authors == nil || authorID == "" {
		return AuthorInfo{}, false
	}
	info, err := s.Authors.Author(ctx, projectID, authorID)
	if err != nil {
		// A deleted author cannot block a publish: the byline dies, not the
		// article.
		return AuthorInfo{}, false
	}
	return info, info.Name != ""
}

// ConnectInput is what the customer types to connect a WordPress site.
type ConnectInput struct {
	Platform            string `json:"platform"`
	SiteURL             string `json:"siteUrl"`
	Username            string `json:"username"`
	ApplicationPassword string `json:"applicationPassword"`
}

// Connect checks the site and the credentials, then stores them. Nothing is
// stored unless WordPress accepted the password and the user may publish.
func (s *Service) Connect(ctx context.Context, projectID, userID string, input ConnectInput) (Connection, error) {
	if input.Platform != WordPressPlatform {
		return Connection{}, &ValidationError{Message: "Choose WordPress."}
	}
	siteURL, err := normalizeSiteURL(input.SiteURL)
	if err != nil {
		return Connection{}, err
	}
	username := strings.TrimSpace(input.Username)
	// WordPress shows application passwords in groups of four with spaces;
	// it accepts them with or without.
	password := strings.Join(strings.Fields(input.ApplicationPassword), "")
	if username == "" || len(username) > 200 || password == "" || len(password) > 200 {
		return Connection{}, &ValidationError{Message: "Enter the WordPress username and the application password."}
	}
	site, err := s.WordPress.Discover(ctx, siteURL)
	if err != nil {
		return Connection{}, err
	}
	if _, err := s.WordPress.Verify(ctx, Credentials{APIBase: site.APIBase, Username: username, Password: password}); err != nil {
		return Connection{}, err
	}
	ciphertext, err := s.Cipher.Encrypt(password)
	if err != nil {
		return Connection{}, fmt.Errorf("encrypt site password: %w", err)
	}
	connection := Connection{
		ProjectID: projectID, Platform: WordPressPlatform, SiteURL: site.SiteURL, APIBase: site.APIBase,
		Username: username, SecretCiphertext: ciphertext, SiteName: site.Name, ConnectedBy: userID,
	}
	if err := s.Store.SaveConnection(ctx, connection); err != nil {
		return Connection{}, err
	}
	return s.Store.Connection(ctx, projectID)
}

// Connection returns the project's website, or ErrNotConnected.
func (s *Service) Connection(ctx context.Context, projectID string) (Connection, error) {
	return s.Store.Connection(ctx, projectID)
}

// Disconnect forgets the project's website and its password. Posts already
// published stay on the site.
func (s *Service) Disconnect(ctx context.Context, projectID string) error {
	return s.Store.DeleteConnection(ctx, projectID)
}

// Publication returns where a draft was published, or ErrNotFound.
func (s *Service) Publication(ctx context.Context, projectID, draftID string) (Publication, error) {
	return s.Store.Publication(ctx, projectID, draftID)
}

// Publish sends a draft to the project's website as a post with the given
// status, adding the chosen JSON-LD schema when WordPress allows it.
// Publishing the same draft again updates that post; its images are copied
// into the site's media library once.
func (s *Service) Publish(ctx context.Context, projectID, draftID, status string, options SchemaOptions) (Publication, PublicationSchema, error) {
	if status != StatusDraft && status != StatusPublish {
		return Publication{}, PublicationSchema{}, &ValidationError{Message: "Choose draft or publish."}
	}
	connection, err := s.Store.Connection(ctx, projectID)
	if err != nil {
		return Publication{}, PublicationSchema{}, err
	}
	draft, err := s.Drafts.Get(ctx, projectID, draftID)
	if err != nil {
		return Publication{}, PublicationSchema{}, err
	}
	if strings.TrimSpace(draft.Title) == "" || strings.TrimSpace(draft.Content) == "" {
		return Publication{}, PublicationSchema{}, &ValidationError{Message: "Add a title and some content before publishing."}
	}
	// A WordPress draft is private, so only going live is refused.
	if status == StatusPublish && keywordStuffed(draft.TargetKeyword, draft.Content) {
		return Publication{}, PublicationSchema{}, ErrKeywordStuffing
	}
	password, err := s.Cipher.Decrypt(connection.SecretCiphertext)
	if err != nil {
		return Publication{}, PublicationSchema{}, fmt.Errorf("decrypt site password: %w", err)
	}
	creds := Credentials{APIBase: connection.APIBase, Username: connection.Username, Password: password}

	previous, err := s.Store.Publication(ctx, projectID, draftID)
	switch {
	case errors.Is(err, ErrNotFound):
		previous = Publication{}
	case err != nil:
		return Publication{}, PublicationSchema{}, err
	case previous.SiteURL != connection.SiteURL:
		// Connected to another site since: start fresh there.
		previous = Publication{}
	}

	media, replace, featured, err := s.copyImages(ctx, creds, connection.SiteURL, draft.Content, previous.Media)
	if err != nil {
		return Publication{}, PublicationSchema{}, err
	}
	author, hasAuthor := s.publishedAuthor(ctx, projectID, draft.AuthorID)
	schema, err := schemaPass(draft, author, hasAuthor, media, options)
	if err != nil {
		return Publication{}, PublicationSchema{}, err
	}
	// The byline is Markdown added to the article before rendering, while
	// the audit-linked copy backs it with dateModified and the same date.
	markup, err := renderHTML(draft.Content+schema.Byline, replace)
	if err != nil {
		return Publication{}, PublicationSchema{}, err
	}
	post := Post{
		Title: strings.TrimSpace(draft.Title), Content: markup, Excerpt: strings.TrimSpace(draft.MetaDescription),
		Slug: strings.TrimSpace(draft.Slug), Status: status, FeaturedMedia: featured,
	}
	saved, err := s.WordPress.SavePost(ctx, creds, previous.RemoteID, post)
	if errors.Is(err, ErrRemoteMissing) {
		// Deleted on the site: publish it as a new post.
		saved, err = s.WordPress.SavePost(ctx, creds, "", post)
	}
	if err != nil {
		return Publication{}, PublicationSchema{}, err
	}
	schemaInfo := s.addSchema(ctx, creds, saved, post, markup, schema)
	publication := Publication{
		DraftID: draftID, ProjectID: projectID, SiteURL: connection.SiteURL,
		RemoteID: saved.ID, RemoteURL: saved.Link, RemoteStatus: saved.Status, Media: media,
	}
	if err := s.Store.SavePublication(ctx, publication); err != nil {
		return Publication{}, PublicationSchema{}, err
	}
	final, err := s.Store.Publication(ctx, projectID, draftID)
	if err != nil {
		return Publication{}, PublicationSchema{}, err
	}
	return final, schemaInfo, nil
}

// PublicationSchema reports what a publish did with the requested JSON-LD.
type PublicationSchema struct {
	// Added is true when at least one block reached the post.
	Added bool `json:"added"`
	// Note explains why a requested block was left off; empty when all went
	// out.
	Note string `json:"note,omitempty"`
}

// schemaPass builds the per-publish schema plan from the saved images and
// the draft's author and review date. The first image's site address feeds
// the Article schema, so use its copied URL when one exists.
func schemaPass(draft contenteditor.Draft, author AuthorInfo, hasAuthor bool, media map[string]Media, options SchemaOptions) (SchemaBuild, error) {
	image := ""
	for _, entry := range media {
		if entry.URL != "" {
			image = entry.URL
			break
		}
	}
	if image == "" {
		urls := imageURLs(draft.Content)
		if len(urls) > 0 {
			image = urls[0]
		}
	}
	return buildSchemas(schemaInput{
		Title: strings.TrimSpace(draft.Title), Meta: draft.MetaDescription, Content: draft.Content,
		Image: image, Author: author, HasAuthor: hasAuthor,
		Reviewed: draft.LastReviewedAt,
	}, options), nil
}

// addSchema appends the built JSON-LD to the saved post, and only when the
// WordPress user can keep scripts in post content. A schema problem never
// fails the publish: the post from the first save stands, and the writer
// is told what could not be added.
func (s *Service) addSchema(ctx context.Context, creds Credentials, saved Saved, post Post, markup string, schema SchemaBuild) PublicationSchema {
	info := PublicationSchema{}
	if len(schema.Blocks) == 0 {
		info.Note = schema.Skipped
		return info
	}
	allowed, err := s.WordPress.ViewRawHTML(ctx, creds)
	switch {
	case err != nil:
		info.Note = schema.Skipped + " The schema is left off because checking this WordPress user failed."
		return info
	case !allowed:
		info.Note = schema.Skipped + " This WordPress user cannot keep scripts in post content, so the schema is not sent here."
		return info
	}
	var scripts strings.Builder
	for _, block := range schema.Blocks {
		scripts.WriteString("\n" + schemaScriptTag(block))
	}
	post.Content = markup + scripts.String()
	if _, err := s.WordPress.SavePost(ctx, creds, saved.ID, post); err != nil {
		info.Note = schema.Skipped + " The schema could not be saved on the site, but the article itself is published."
		return info
	}
	info.Added = true
	return info
}

// copyImages uploads the draft's images to the site's media library, reusing
// those copied by an earlier publish. It returns every copied image, the
// address each draft image now points to, and the first image's media id for
// the featured image.
func (s *Service) copyImages(ctx context.Context, creds Credentials, siteURL, content string, known map[string]Media) (map[string]Media, map[string]string, int64, error) {
	media := map[string]Media{}
	replace := map[string]string{}
	var featured int64
	urls := imageURLs(content)
	if len(urls) > MaxImagesPerArticle {
		return nil, nil, 0, &ValidationError{Message: fmt.Sprintf("An article can carry at most %d images when it is published.", MaxImagesPerArticle)}
	}
	for _, source := range urls {
		copied, ok := known[source]
		if !ok {
			if onSite(source, siteURL) {
				continue
			}
			data, contentType, err := s.WordPress.FetchImage(ctx, source)
			if err != nil {
				return nil, nil, 0, &ValidationError{Message: fmt.Sprintf("Could not copy the image %s: %v", source, err)}
			}
			copied, err = s.WordPress.UploadImage(ctx, creds, imageFilename(source, contentType), contentType, data)
			if err != nil {
				return nil, nil, 0, err
			}
		}
		media[source] = copied
		replace[source] = copied.URL
		if featured == 0 {
			featured = copied.ID
		}
	}
	return media, replace, featured, nil
}

// onSite reports whether an image is already served by the site itself.
func onSite(rawURL, siteURL string) bool {
	image, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	site, err := url.Parse(siteURL)
	return err == nil && strings.EqualFold(image.Host, site.Host)
}

func imageFilename(rawURL, contentType string) string {
	name := "image"
	if parsed, err := url.Parse(rawURL); err == nil {
		if base := path.Base(parsed.Path); base != "." && base != "/" && base != "" {
			name = strings.TrimSuffix(base, path.Ext(base))
		}
	}
	extension := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}[contentType]
	if extension == "" {
		extension = ".img"
	}
	return name + extension
}

// normalizeSiteURL accepts "example.com" or "https://example.com/" and
// returns "https://example.com". A path is kept for sites in a subfolder.
func normalizeSiteURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 300 {
		return "", &ValidationError{Message: "Enter your website address, like example.com."}
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", &ValidationError{Message: "Enter your website address, like example.com."}
	}
	if parsed.Scheme != "https" {
		return "", ErrNotHTTPS
	}
	return "https://" + strings.ToLower(parsed.Host) + strings.TrimRight(parsed.EscapedPath(), "/"), nil
}
