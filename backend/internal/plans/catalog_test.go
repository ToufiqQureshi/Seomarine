package plans

import (
	"slices"
	"testing"
)

func TestCatalogMatchesTheOwnersPlanTable(t *testing.T) {
	want := []struct {
		id                              string
		price, sites, keywords, members int
		credits                         int64
	}{
		{Free, 0, 1, 0, 1, 0},
		{Basic, 999, 1, 100, 1, 4000},
		{Pro, 2499, 2, 500, 3, 10000},
		{Agency, 5999, 10, 2000, 10, 25000},
	}
	got := Catalog()
	if len(got) != len(want) {
		t.Fatalf("catalog has %d plans, want %d", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.ID != w.id || g.PriceINR != w.price || g.Sites != w.sites || g.TrackedKeywords != w.keywords || g.Members != w.members || g.MonthlyCredits != w.credits {
			t.Errorf("plan %d = %s ₹%d, %d sites, %d keywords, %d members, %d credits; want %+v",
				i, g.ID, g.PriceINR, g.Sites, g.TrackedKeywords, g.Members, g.MonthlyCredits, w)
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
	all := []Feature{GoogleData, Keywords, RankTracking, FullAudit, Publishing, Backlinks, DomainResearch, AIVisibility, AIWriting, WhiteLabel}
	want := map[string][]Feature{
		Free:   nil,
		Basic:  {GoogleData, Keywords, RankTracking, FullAudit, Publishing, AIWriting},
		Pro:    {GoogleData, Keywords, RankTracking, FullAudit, Publishing, AIWriting, Backlinks, DomainResearch, AIVisibility},
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

// Credits are worth 1/1280 of a US dollar of provider cost (1,000 per dollar
// with a 1.28 markup). A plan's grant is the most one workspace can cost
// Uplumo in a month, so it must stay well under the price.
func TestNoPlanCanCostMoreThanAThirdOfItsPrice(t *testing.T) {
	const inrPerUSD = 90.0 // a rupee that has weakened, to keep a margin
	for _, plan := range Catalog() {
		providerINR := float64(plan.MonthlyCredits) / 1280 * inrPerUSD
		if providerINR > float64(plan.PriceINR)/3 {
			t.Errorf("%s: credits can cost ₹%.0f a month at the provider, more than a third of ₹%d", plan.ID, providerINR, plan.PriceINR)
		}
	}
}

// A plan's tracked keywords, checked weekly on desktop at its deepest
// allowed depth with the scheduled (queued) price, must fit in its monthly
// credits, or the rank tracking it sells runs out of credits mid-month.
func TestWeeklyRankTrackingFitsInTheMonthlyCredits(t *testing.T) {
	const checksPerMonth = 4.35
	for _, plan := range Catalog() {
		if plan.TrackedKeywords == 0 {
			continue
		}
		if plan.MaxSerpDepth < 10 || plan.MaxSerpDepth > 100 || plan.MaxSerpDepth%10 != 0 {
			t.Fatalf("%s: MaxSerpDepth %d is not 10 to 100 in steps of 10", plan.ID, plan.MaxSerpDepth)
		}
		perCheckUSD := 0.0006 + float64(plan.MaxSerpDepth/10-1)*0.00045
		credits := float64(plan.TrackedKeywords) * checksPerMonth * perCheckUSD * 1280
		if credits > float64(plan.MonthlyCredits)*0.9 {
			t.Errorf("%s: weekly rank tracking costs %.0f credits a month, over 90%% of the %d it grants", plan.ID, credits, plan.MonthlyCredits)
		}
	}
}
