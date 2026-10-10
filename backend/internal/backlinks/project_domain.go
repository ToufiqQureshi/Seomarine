package backlinks

// NormalizeProjectDomain canonicalizes a project website to a lowercase bare
// hostname using the same IDNA and public-suffix checks as paid backlink APIs.
func NormalizeProjectDomain(input string) (string, error) {
	target, err := normalizeTarget(input, string(ScopeDomain))
	if err != nil {
		return "", err
	}
	return target.APITarget, nil
}
