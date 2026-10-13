package plans

import (
	"slices"
	"testing"
)

func TestCatalogMatchesTheOwnersPlanTable(t *testing.T) {
	want := []struct {
		id                              string
		price, sites, keywords, members int
	}{
		{Free, 0, 1, 0, 1},
		{Basic, 999, 1, 100, 1},
		{Pro, 2499, 2, 500, 3},
		{Agency, 5999, 10, 2000, 10},
	}
	got := Catalog()
	if len(got) != len(want) {
		t.Fatalf("catalog has %d plans, want %d", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.ID != w.id || g.PriceINR != w.price || g.Sites != w.sites || g.TrackedKeywords != w.keywords || g.Members != w.members {
			t.Errorf("plan %d = %s ₹%d, %d sites, %d keywords, %d members; want %+v",
				i, g.ID, g.PriceINR, g.Sites, g.TrackedKeywords, g.Members, w)
		}
		if g.Name == "" || g.Summary == "" || len(g.Highlights) == 0 {
			t.Errorf("plan %s has no name, summary or highlights to show a customer", g.ID)
		}
	}
	if !slices.Equal(Paid(), []string{Basic, Pro, Agency}) {
		t.Errorf("Paid() = %v", Paid())
	}
}

func TestEachPlanIncludesExactlyItsFeatures(t *testing.T) {
	all := []Feature{GoogleData, Keywords, RankTracking, FullAudit, Backlinks, DomainResearch, AIVisibility, AIWriting, WhiteLabel}
	want := map[string][]Feature{
		Free:   nil,
		Basic:  {GoogleData, Keywords, RankTracking, FullAudit},
		Pro:    {GoogleData, Keywords, RankTracking, FullAudit, Backlinks, DomainResearch, AIVisibility, AIWriting},
		Agency: all,
	}
	for id, included := range want {
		plan, ok := Get(id)
		if !ok {
			t.Fatalf("Get(%q) not found", id)
		}
		for _, feature := range all {
			if got, want := plan.Allows(feature), slices.Contains(included, feature); got != want {
				t.Errorf("%s allows %s = %v, want %v", id, feature, got, want)
			}
		}
	}
	if _, ok := Get("enterprise"); ok {
		t.Error("Get found a plan that is not in the catalog")
	}
}

func TestUnlimitedAllowsEverythingAndHasNoLimits(t *testing.T) {
	for _, feature := range agencyFeatures {
		if !Unlimited.Allows(feature) {
			t.Errorf("Unlimited does not allow %s", feature)
		}
	}
	if Unlimited.Sites != 0 || Unlimited.TrackedKeywords != 0 || Unlimited.Members != 0 {
		t.Errorf("Unlimited carries limits: %+v", Unlimited)
	}
}

func TestCallersCannotChangeTheCatalog(t *testing.T) {
	first, _ := Get(Pro)
	first.Highlights[0] = "mutated"
	first.features[0] = "mutated"
	Catalog()[2].Highlights[1] = "mutated"

	second, _ := Get(Pro)
	if second.Highlights[0] == "mutated" || second.Highlights[1] == "mutated" || !second.Allows(GoogleData) {
		t.Fatal("a caller's change reached the shared catalog")
	}
	// Basic's list is the base of Pro's and Agency's; they must not share it.
	if basic, _ := Get(Basic); basic.Allows(Backlinks) || basic.Allows(WhiteLabel) {
		t.Fatal("Basic picked up a feature of a higher plan")
	}
}
