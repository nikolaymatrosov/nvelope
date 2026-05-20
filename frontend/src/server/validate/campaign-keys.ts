// TypeScript mirror of Go's `AllowedCampaignMergeTags` map in
// internal/campaign/domain/visualdoc.go. Whenever a new campaign-namespace
// merge tag is added on the Go side, the same key MUST be added here too —
// the drift-catcher test (campaign-keys.test.ts) reads the Go source at
// test time and fails the suite if the two diverge.

export const AllowedCampaignMergeTags: ReadonlySet<string> = new Set([
  "unsubscribe_url",
  "preference_url",
  "archive_url",
  "view_in_browser_url",
  "tenant_name",
  "current_date",
])
