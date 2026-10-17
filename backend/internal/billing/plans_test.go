package billing

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/plans"
)

// Not testPlanID, which a test below uses as a plan nobody configured.
var tierPlanIDs = map[string]string{PlanBasic: "plan_tier_basic", PlanPro: "plan_tier_pro", PlanAgency: "plan_tier_agency"}

// sellTiers makes the fixture sell all three plans.
func (f *fixture) sellTiers() {
	f.svc = NewPlanService(f.pool, f.svc.razorpay, tierPlanIDs, testWebhookSecret)
}

// eventOnPlan is f.event for a subscription to plan.
func (f *fixture) eventOnPlan(plan, name, subID, status string) []byte {
	return bytes.Replace(f.event(name, subID, status, t1), []byte(testPlanID), []byte(tierPlanIDs[plan]), 1)
}

func TestPlanDecidesFeaturesAndLimits(t *testing.T) {
	ctx := context.Background()
	all := []plans.Feature{
		plans.GoogleData, plans.Keywords, plans.RankTracking, plans.FullAudit, plans.Publishing, plans.AIWriting,
		plans.Backlinks, plans.DomainResearch, plans.AIVisibility, plans.WhiteLabel,
	}
	for _, test := range []struct {
		plan                     string
		allowed                  int // the first n of all
		sites, keywords, members int
	}{
		{PlanFree, 0, 1, 0, 1},
		{PlanBasic, 6, 1, 100, 1},
		{PlanPro, 9, 2, 500, 3},
		{PlanAgency, 10, 10, 1500, 10},
	} {
		t.Run(test.plan, func(t *testing.T) {
			f := newFixture(ctx, t)
			f.sellTiers()
			if test.plan != PlanFree {
				f.deliver(ctx, t, f.eventOnPlan(test.plan, "subscription.activated", "sub_"+test.plan, "active"))
			}
			for i, feature := range all {
				got, err := f.svc.Gate(feature).HasPaidPlan(ctx, f.org)
				if err != nil || got != (i < test.allowed) {
					t.Errorf("%s includes %s = %v, %v; want %v", test.plan, feature, got, err, i < test.allowed)
				}
			}
			sites, err1 := f.svc.SiteLimit(ctx, f.org)
			keywords, err2 := f.svc.KeywordLimit(ctx, f.org)
			members, err3 := f.svc.MemberLimit(ctx, f.org)
			if err := errors.Join(err1, err2, err3); err != nil {
				t.Fatal(err)
			}
			if sites != test.sites || keywords != test.keywords || members != test.members {
				t.Errorf("limits = %d sites, %d keywords, %d members; want %d, %d, %d",
					sites, keywords, members, test.sites, test.keywords, test.members)
			}
		})
	}
}

func TestAPlanThatStoppedPayingLosesItsFeatures(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.sellTiers()
	gate := f.svc.Gate(plans.Backlinks)

	f.deliver(ctx, t, f.eventOnPlan(PlanPro, "subscription.activated", "sub_1", "active"))
	if ok, err := gate.HasPaidPlan(ctx, f.org); err != nil || !ok {
		t.Fatalf("active Pro includes backlinks = %v, %v; want true", ok, err)
	}
	body := bytes.Replace(f.event("subscription.halted", "sub_1", "halted", t2), []byte(testPlanID), []byte(tierPlanIDs[PlanPro]), 1)
	f.deliver(ctx, t, body)
	if ok, err := gate.HasPaidPlan(ctx, f.org); err != nil || ok {
		t.Fatalf("halted Pro includes backlinks = %v, %v; want false", ok, err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := gate.HasPaidPlan(canceled, f.org); err == nil {
		t.Error("a gate on a failing database answered, want an error")
	}
}

func TestThePlanComesFromRazorpayNotFromCheckout(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.sellTiers()

	// The browser asks for Basic; Razorpay then reports an Agency subscription.
	checkout, err := f.svc.CheckoutPlan(ctx, f.org, PlanBasic)
	if err != nil {
		t.Fatal(err)
	}
	f.deliver(ctx, t, f.eventOnPlan(PlanAgency, "subscription.activated", checkout.SubscriptionID, "active"))
	f.assertStatus(ctx, t, Status{Plan: PlanAgency, Status: "active"})
}

func TestCheckoutRefusesAPlanThatIsNotSold(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.svc = NewPlanService(f.pool, f.svc.razorpay, map[string]string{PlanPro: "plan_tier_pro", PlanAgency: ""}, testWebhookSecret)

	for _, plan := range []string{PlanBasic, PlanAgency, PlanFree, "enterprise", ""} {
		if _, err := f.svc.CheckoutPlan(ctx, f.org, plan); !errors.Is(err, ErrPlanUnavailable) {
			t.Errorf("CheckoutPlan(%q) error = %v, want ErrPlanUnavailable", plan, err)
		}
	}
	if n := f.calls.Load(); n != 0 {
		t.Errorf("Razorpay was called %d times for plans that are not sold", n)
	}
	if got := f.svc.AvailablePlans(); len(got) != 1 || got[0] != PlanPro {
		t.Errorf("AvailablePlans() = %v, want [pro]", got)
	}
}

func TestWebhookForAnUnknownPlanFailsAndChangesNothing(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.sellTiers()

	// testPlanID is not one of the three configured plans.
	body := f.event("subscription.activated", "sub_stray", "active", t1)
	_, err := f.svc.HandleWebhook(ctx, body, sign(body), "evt_"+rand.Text())
	if !errors.Is(err, ErrUnknownPlan) || errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("HandleWebhook() error = %v, want ErrUnknownPlan and not ErrInvalidEvent", err)
	}
	f.assertStatus(ctx, t, Status{Plan: PlanFree, Status: "none"})
}

func TestEachPlanGrantsItsOwnCredits(t *testing.T) {
	ctx := context.Background()
	// Basic grants credits too: its keyword research and rank tracking spend them.
	for plan, want := range map[string]int64{PlanBasic: 4000, PlanPro: 10000, PlanAgency: 25000} {
		t.Run(plan, func(t *testing.T) {
			f := newFixture(ctx, t)
			f.sellTiers()
			f.deliver(ctx, t, f.eventOnPlan(plan, "subscription.charged", "sub_1", "active"))

			got, err := NewCreditLedgerService(f.pool).GetBalance(ctx, f.org)
			if err != nil || got != want {
				t.Errorf("balance after one %s charge = %d, %v; want %d", plan, got, err, want)
			}
		})
	}
}

// addMember makes a user with email a member of f.org with role.
func (f *fixture) addMember(ctx context.Context, t *testing.T, email, role string, verified bool) {
	t.Helper()
	id := "user-" + rand.Text()
	if _, err := f.pool.Exec(ctx, `INSERT INTO "user" (id, name, email, email_verified) VALUES ($1, 'Someone', $2, $3)`, id, email, verified); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		if _, err := f.pool.Exec(context.WithoutCancel(ctx), `DELETE FROM "user" WHERE id = $1`, id); err != nil {
			t.Errorf("delete user: %v", err)
		}
	})
	if _, err := f.pool.Exec(ctx, `INSERT INTO member (id, organization_id, user_id, role, created_at) VALUES ($1, $2, $1, $3, now())`, id, f.org, role); err != nil {
		t.Fatalf("create member: %v", err)
	}
}

func TestStaffWorkspaceIsNeverLimited(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.sellTiers()
	staff := "Owner+" + rand.Text() + "@Uplumo.test"
	f.svc.SetStaffEmails([]string{"  " + staff + " ", ""})
	f.addMember(ctx, t, staff, "owner", true)

	status, err := f.svc.Status(ctx, f.org)
	if err != nil || !status.Unlimited || status.Plan != PlanAgency || status.Status != "none" {
		t.Fatalf("Status() = %+v, %v; want an unlimited Agency workspace with no subscription", status, err)
	}
	if ok, err := f.svc.Gate(plans.WhiteLabel).HasPaidPlan(ctx, f.org); err != nil || !ok {
		t.Errorf("staff workspace includes white-label = %v, %v; want true", ok, err)
	}
	sites, _ := f.svc.SiteLimit(ctx, f.org)
	keywords, _ := f.svc.KeywordLimit(ctx, f.org)
	members, _ := f.svc.MemberLimit(ctx, f.org)
	if sites != 0 || keywords != 0 || members != 0 {
		t.Errorf("staff limits = %d, %d, %d; want none", sites, keywords, members)
	}
	if unlimited, err := f.svc.IsUnlimited(ctx, f.org); err != nil || !unlimited {
		t.Errorf("IsUnlimited() = %v, %v; want true", unlimited, err)
	}
}

func TestOnlyAVerifiedOwnerMakesAWorkspaceStaff(t *testing.T) {
	ctx := context.Background()
	for name, member := range map[string]struct {
		role     string
		verified bool
		listed   bool
	}{
		"a staff member who is not the owner":  {"admin", true, true},
		"an owner whose address is unverified": {"owner", false, true},
		"an owner who is not staff":            {"owner", true, false},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(ctx, t)
			email := "person-" + rand.Text() + "@uplumo.test"
			if member.listed {
				f.svc.SetStaffEmails([]string{email})
			} else {
				f.svc.SetStaffEmails([]string{"someone-else@uplumo.test"})
			}
			f.addMember(ctx, t, email, member.role, member.verified)

			status, err := f.svc.Status(ctx, f.org)
			if err != nil || status.Unlimited || status.Plan != PlanFree {
				t.Fatalf("Status() = %+v, %v; want an ordinary free workspace", status, err)
			}
		})
	}
}

func TestStaffWorkspaceIsToppedUpInsteadOfRefused(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	ledger := NewCreditLedgerService(f.pool)

	// An ordinary workspace with no credits is refused.
	if _, err := ledger.Reserve(ctx, f.org, 500, ""); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("Reserve() without credits error = %v, want ErrInsufficientBalance", err)
	}

	staff := "owner-" + rand.Text() + "@uplumo.test"
	f.svc.SetStaffEmails([]string{staff})
	f.addMember(ctx, t, staff, "owner", true)
	ledger.Unlimited = f.svc.IsUnlimited

	if _, err := ledger.Reserve(ctx, f.org, 500, ""); err != nil {
		t.Fatalf("Reserve() for a staff workspace error = %v", err)
	}
	// Topped up by the minimum, then charged: the spend is still counted.
	if got, err := ledger.GetBalance(ctx, f.org); err != nil || got != staffTopUpCredits-500 {
		t.Errorf("balance = %d, %v; want %d", got, err, staffTopUpCredits-500)
	}
	// With enough balance nothing is granted.
	if _, err := ledger.Reserve(ctx, f.org, 1, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := ledger.GetBalance(ctx, f.org); got != staffTopUpCredits-501 {
		t.Errorf("balance after a covered reservation = %d, want %d", got, staffTopUpCredits-501)
	}
	// A reservation for exactly the balance is covered without a grant.
	if _, err := ledger.Reserve(ctx, f.org, staffTopUpCredits-501, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := ledger.GetBalance(ctx, f.org); got != 0 {
		t.Errorf("balance after spending exactly what was left = %d, want 0", got)
	}
	// A reservation larger than the minimum top-up is covered exactly.
	if _, err := ledger.Reserve(ctx, f.org, 3*staffTopUpCredits, ""); err != nil {
		t.Errorf("large Reserve() for a staff workspace error = %v", err)
	}
	if got, _ := ledger.GetBalance(ctx, f.org); got != 0 {
		t.Errorf("balance after a reservation above the top-up = %d, want 0", got)
	}
}

func TestUnusedCreditsRollOverUpToTwoMonths(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.sellTiers()
	ledger := NewCreditLedgerService(f.pool)
	balance := func() int64 {
		t.Helper()
		got, err := ledger.GetBalance(ctx, f.org)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	charge := func() { f.deliver(ctx, t, f.eventOnPlan(PlanPro, "subscription.charged", "sub_1", "active")) }

	charge()
	charge()
	if got := balance(); got != 20000 {
		t.Fatalf("after two unused Pro months balance = %d, want 20000", got)
	}
	charge()
	if got := balance(); got != 20000 {
		t.Fatalf("after a third unused month balance = %d, want it capped at two months (20000)", got)
	}
	// Spend some: the next charge tops up to the cap, not past it.
	if _, err := f.pool.Exec(ctx, `UPDATE go_billing_credits SET balance_credits = 15000 WHERE organization_id = $1`, f.org); err != nil {
		t.Fatal(err)
	}
	charge()
	if got := balance(); got != 20000 {
		t.Fatalf("after a top-up from 15000 balance = %d, want 20000", got)
	}
}

func TestWorkspaceDailyLimitIsAThirdOfTheMonth(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.sellTiers()
	got, err := f.svc.WorkspaceDailyLimitUSD(ctx, f.org)
	if err != nil || got != 0 {
		t.Fatalf("free workspace daily limit = %v, %v; want 0 (it has no credits to spend)", got, err)
	}
	f.deliver(ctx, t, f.eventOnPlan(PlanPro, "subscription.charged", "sub_1", "active"))
	got, err = f.svc.WorkspaceDailyLimitUSD(ctx, f.org)
	if want := 10000.0 / 3 / 1280; err != nil || got < want-1e-9 || got > want+1e-9 {
		t.Fatalf("Pro daily limit = %v, %v; want %v", got, err, want)
	}
}

func TestDowngradeKeepsDataAndMarksWorkspaceOverLimit(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.sellTiers()
	f.deliver(ctx, t, f.eventOnPlan(PlanPro, "subscription.activated", "sub_1", "active"))
	if _, err := f.pool.Exec(ctx, `INSERT INTO projects (id, organization_id, name) VALUES ($1,$3,'One'),($2,$3,'Two')`, "project-a-"+rand.Text(), "project-b-"+rand.Text(), f.org); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE go_billing_subscriptions SET plan=$2 WHERE organization_id=$1`, f.org, PlanBasic); err != nil {
		t.Fatal(err)
	}

	usage, err := f.svc.UsageAgainstPlan(ctx, f.org)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Sites.Used != 2 || usage.Sites.Limit != 1 || !usage.Sites.Exceeded {
		t.Fatalf("site usage = %+v, want 2 retained over a limit of 1", usage.Sites)
	}
	if within, err := f.svc.WithinPlanLimits(ctx, f.org); err != nil || within {
		t.Fatalf("WithinPlanLimits() = %v, %v; want false", within, err)
	}
	var retained int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM projects WHERE organization_id=$1`, f.org).Scan(&retained); err != nil || retained != 2 {
		t.Fatalf("retained projects = %d, %v; want 2", retained, err)
	}
}
