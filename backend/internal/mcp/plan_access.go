package mcp

import "github.com/toufiqqureshi/uplumo/backend/internal/plans"

// toolFeature is an explicit allowlist. A newly registered tool is unavailable
// on hosted billing until its entitlement is classified here.
func toolFeature(name string) (plans.Feature, bool) {
	switch name {
	case "whoami", "list_projects", "create_project", "get_project_context", "update_project_context",
		"search_serp_locations", "list_site_audits", "get_audit_status", "get_audit_pages",
		"get_audit_issues", "delete_site_audit", "run_site_audit", "list_saved_keywords",
		"save_keywords", "remove_saved_keywords", "save_report", "list_reports", "get_report",
		"delete_report", "set_report_sharing", "list_report_templates", "save_report_template",
		"delete_report_template", "list_business_categories":
		return "", true
	case "get_backlinks_overview", "get_backlinks_profile":
		return plans.Backlinks, true
	case "get_domain_overview", "get_domain_keyword_suggestions", "get_ranked_keywords", "find_serp_competitors":
		return plans.DomainResearch, true
	case "get_search_console_performance", "inspect_urls", "get_search_opportunities",
		"get_google_analytics_organic_landing_pages", "get_google_analytics_page_performance",
		"get_google_analytics_key_events", "get_google_analytics_traffic_acquisition",
		"get_google_analytics_ecommerce_performance", "get_google_analytics_site_search",
		"get_google_analytics_audience_breakdown", "get_google_analytics_organic_overview",
		"get_google_analytics_measurement_health":
		return plans.GoogleData, true
	case "create_rank_tracker", "estimate_rank_tracker_cost", "run_rank_tracker", "get_rank_tracker",
		"add_rank_tracking_keywords", "remove_rank_tracking_keywords":
		return plans.RankTracking, true
	case "research_keywords", "get_keyword_metrics", "get_serp_results", "get_local_serp_results",
		"get_google_business_questions", "search_local_businesses", "get_business_profile",
		"get_business_reviews", "get_business_updates", "get_local_rank_grid":
		return plans.Keywords, true
	default:
		return "", false
	}
}
