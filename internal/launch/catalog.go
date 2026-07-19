package launch

// Entry is one curated launchpad row.
type Entry struct {
	// Label shown in the hub menu.
	Label string
	// Detail is a one-line hint under or beside the label.
	Detail string
	// Spec is what to run when the user selects this entry.
	// Dir is left empty: Run fills campaign root when WantsCampaignRoot(spec).
	Spec Spec
}

// Catalog is the default launchpad list (Ring 3 hub surface).
// Keep this curated and small; data-driven config can come later.
//
// Campaign-scoped tools (wi, intent, list, watch) run with cwd = campaign root
// when DetectCampaignRoot finds a .campaign ancestor; version checks inherit cwd.
func Catalog() []Entry {
	return []Entry{
		{
			Label:  "Campaign work items",
			Detail: "camp wi — unified work browser (campaign root when known)",
			Spec:   Spec{Tool: "camp", Args: []string{"wi"}, Title: "camp wi"},
		},
		{
			Label:  "Intent explore",
			Detail: "camp intent explore — triage the inbox (campaign root when known)",
			Spec:   Spec{Tool: "camp", Args: []string{"intent", "explore"}, Title: "camp intent explore"},
		},
		{
			Label:  "List festivals",
			Detail: "fest list — festivals by status (campaign root when known)",
			Spec:   Spec{Tool: "fest", Args: []string{"list"}, Title: "fest list"},
		},
		{
			Label:  "Watch festival progress",
			Detail: "fest watch — live progress (q to return here)",
			Spec:   Spec{Tool: "fest", Args: []string{"watch"}, Title: "fest watch"},
		},
		{
			Label:  "Camp version",
			Detail: "camp version — quick sanity check",
			Spec:   Spec{Tool: "camp", Args: []string{"version"}, Title: "camp version"},
		},
		{
			Label:  "Fest version",
			Detail: "fest version — quick sanity check",
			Spec:   Spec{Tool: "fest", Args: []string{"version"}, Title: "fest version"},
		},
	}
}
