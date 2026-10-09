package domain

import "github.com/toufiqqureshi/seomarine/backend/internal/aisearch"

// Scope matches the host and path scoping used by the legacy domain overview
// feature: exact URL, subfolder, entire domain or all subdomains.
type Scope = aisearch.Scope

const (
	ScopeExactURL   = aisearch.ScopeExactURL
	ScopeSubfolder  = aisearch.ScopeSubfolder
	ScopeDomain     = aisearch.ScopeDomain
	ScopeSubdomains = aisearch.ScopeSubdomains
)

// ResearchTarget is the normalized domain or URL returned by the shared
// validator. The Go implementation reuses the AI search validator to avoid
// drifting from a single source of truth.
type ResearchTarget = aisearch.ResearchTarget

func parseResearchTarget(input string, requested Scope) (ResearchTarget, error) {
	return aisearch.ResolveResearchTarget(input, requested)
}
