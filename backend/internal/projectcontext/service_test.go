package projectcontext

import (
	"encoding/json"
	"testing"
)

func TestResolveTypedSectionRequiresFullStrictInput(t *testing.T) {
	custom, domains, urls := map[string]bool{}, map[string]bool{}, map[string]bool{}
	if _, err := resolve(json.RawMessage(`{"section":"current_goal","content":" Grow "}`), custom, domains, urls); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(json.RawMessage(`{"section":"current_goal"}`), custom, domains, urls); err == nil {
		t.Fatal("missing required content accepted")
	}
	if _, err := resolve(json.RawMessage(`{"section":"current_goal","content":"x","extra":true}`), custom, domains, urls); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := resolve(json.RawMessage(`{"section":"current_goal","content":"x","customSection":"bad"}`), custom, domains, urls); err == nil {
		t.Fatal("two operations accepted")
	}
}

func TestResolveNormalizesAndDeduplicatesCompetitors(t *testing.T) {
	custom, domains, urls := map[string]bool{}, map[string]bool{}, map[string]bool{}
	raw := json.RawMessage(`{"addCompetitors":[{"domain":"https://WWW.Acme.com/pricing"},{"domain":"acme.com","notes":"latest"}]}`)
	op, err := resolve(raw, custom, domains, urls)
	if err != nil {
		t.Fatal(err)
	}
	if len(op.competitors) != 1 || op.competitors[0].domain != "acme.com" || op.competitors[0].notes == nil || *op.competitors[0].notes != "latest" {
		t.Fatalf("resolved=%+v", op)
	}
}

func TestResolveCountsBatchAgainstCaps(t *testing.T) {
	custom, domains, urls := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := 0; i < 100; i++ {
		domains[string(rune('a'+i%26))+string(rune('a'+(i/26)%26))+".com"] = true
	}
	if _, err := resolve(json.RawMessage(`{"addCompetitors":[{"domain":"newcomer.example"}]}`), custom, domains, urls); err == nil {
		t.Fatal("101st competitor accepted")
	}
}

func TestNormalizePageURL(t *testing.T) {
	got, err := normalizePageURL("http://WWW.Acme.com/pricing?plan=pro#faq")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://acme.com/pricing?plan=pro" {
		t.Fatalf("url=%q", got)
	}
	if _, err := normalizePageURL("javascript:alert(1)"); err == nil {
		t.Fatal("javascript URL accepted")
	}
}
