package keywords

import "testing"

func TestRankSerpLocations(t *testing.T) {
	locations := []SerpLocation{
		{LocationName: "South Portland,Maine,United States", LocationType: "City"},
		{LocationName: "Portland, OR,Oregon,United States", LocationType: "DMA Region"},
		{LocationName: "Portland,Oregon,United States", LocationType: "City"},
		{LocationName: "Portland,Maine,United States", LocationType: "City"},
		{LocationName: "Portland,Texas,United States", LocationType: "City"},
		{LocationName: "Catonsville,Maryland,United States", LocationType: "City"},
		{LocationName: "New City,New York,United States", LocationType: "City"},
		{LocationName: "New York,New York,United States", LocationType: "City"},
		{LocationName: "La Crosse,Wisconsin,United States", LocationType: "City"},
	}
	got := RankSerpLocations("Portland", locations, "us")
	if len(got) != 5 || got[0].LocationName != "Portland,Maine,United States" || got[1].LocationName != "Portland,Oregon,United States" || got[4].LocationName != "South Portland,Maine,United States" {
		t.Fatalf("Portland ranking = %+v", got)
	}
	got = RankSerpLocations("Catonsville MD", locations, "us")
	if len(got) != 1 || got[0].LocationName != "Catonsville,Maryland,United States" {
		t.Fatalf("abbreviation = %+v", got)
	}
	got = RankSerpLocations("New York", locations, "us")
	if len(got) == 0 || got[0].LocationName != "New York,New York,United States" {
		t.Fatalf("exact place = %+v", got)
	}
	got = RankSerpLocations("La Crosse", locations, "us")
	if len(got) != 1 || got[0].LocationName != "La Crosse,Wisconsin,United States" {
		t.Fatalf("first-token abbreviation = %+v", got)
	}
	if got := RankSerpLocations(" , ", locations, "us"); len(got) != 0 {
		t.Fatalf("blank query = %+v", got)
	}
	if got := FoldLocationText("  São  PAULO "); got != "sao paulo" {
		t.Fatalf("folded = %q", got)
	}
	arlingtons := make([]SerpLocation, 60)
	for i := range arlingtons {
		arlingtons[i] = SerpLocation{LocationName: "Arlington,State,United States", LocationType: "City"}
	}
	if got := RankSerpLocations("Arlington", arlingtons, "us"); len(got) != 50 {
		t.Fatalf("limit = %d", len(got))
	}
}
