package plans

import "testing"

func TestCatalogDefinesRequestedTiersAndLimits(t *testing.T) {
	want := map[string]struct {
		price, sites int
		paidSEO, ai  bool
	}{
		Basic:  {price: 899, sites: 1},
		Pro:    {price: 1899, sites: 5, paidSEO: true, ai: true},
		Agency: {price: 4999, sites: 25, paidSEO: true, ai: true},
	}
	for id, expected := range want {
		got, ok := Get(id)
		if !ok || got.PriceINR != expected.price || got.Sites != expected.sites || got.PaidSEOData != expected.paidSEO || got.AIVisibility != expected.ai {
			t.Errorf("Get(%q) = %+v, %v; want %+v", id, got, ok, expected)
		}
	}
	if PaidProviderAccess(Basic) || !PaidProviderAccess(Pro) || !PaidProviderAccess(Agency) {
		t.Fatal("paid provider access must be off for Basic and on for Pro/Agency")
	}
}

func TestGetReturnsIndependentFeatureSlices(t *testing.T) {
	first, _ := Get(Pro)
	first.Features[0] = "mutated"
	second, _ := Get(Pro)
	if second.Features[0] == "mutated" {
		t.Fatal("caller mutation changed the shared plan catalog")
	}
}
