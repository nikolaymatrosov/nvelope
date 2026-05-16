# nvelope — User Stories

User stories for the multi-tenant newsletter SaaS, grouped by epic.

**Roles:**
- *Visitor* — unauthenticated person on the marketing/signup pages.
- *Tenant owner* — platform user who created/owns a tenant; handles billing & team.
- *Tenant user* — a member of a tenant with role-based permissions (listmonk-style RBAC).
- *Tenant developer* — a tenant user (or API key) integrating via the API.
- *Subscriber* — an email recipient interacting with public pages.
- *Reader* — anonymous viewer of public campaign archives.
- *Platform admin* — internal staff operating the SaaS across tenants.

Format: *As a {role}, I want {capability} so that {value}.*

---

## Epic A — Tenant Onboarding & Platform

- As a **visitor**, I want to sign up and create a tenant with a unique slug, so that I can start
  using the platform.
- As a **tenant owner**, I want to invite team members and assign them roles, so that my team can
  collaborate with appropriate access.
- As a **tenant owner**, I want to see my plan, quotas, and current usage, so that I know where I
  stand.
- As a **platform admin**, I want to view, suspend, and support tenants with every action
  audit-logged, so that I can operate the service safely and accountably.
- As a **tenant**, I want my data fully isolated from every other tenant, so that my subscribers
  and campaigns stay private.

## Epic B — Billing & Usage

- As a **tenant owner**, I want to choose a plan and pay via Stripe, so that I can use paid
  capacity.
- As a **tenant owner**, I want to see usage (emails sent, subscribers) against my quota, so that
  I can plan ahead.
- As a **tenant owner**, I want to be warned before hitting limits and blocked from over-sending,
  so that I avoid surprise overages.
- As a **tenant owner**, I want to view and download invoices, so that I can manage accounting.
- As the **platform**, I want to suspend sending on payment failure without deleting tenant data,
  so that tenants can recover after paying.

## Epic C — Sending Domains & Deliverability

- As a **tenant user**, I want to add a sending domain and receive DKIM/SPF/DMARC DNS records, so
  that I can send from my own branded domain.
- As a **tenant user**, I want to see my domain's verification status and re-trigger a check, so
  that I know when it is ready.
- As a **tenant user**, I want to only be able to send campaigns from a verified domain I own, so
  that deliverability and reputation are protected.

## Epic D — Lists & Subscribers

- As a **tenant user**, I want to create public/private lists with single or double opt-in, so
  that I can manage audiences with the right consent model.
- As a **tenant user**, I want to add, edit, and remove subscribers with custom JSON attributes,
  so that I can store flexible profile data.
- As a **tenant user**, I want to import subscribers from CSV/ZIP with upsert, so that I can
  migrate or bulk-load audiences.
- As a **tenant user**, I want to export subscribers, so that I can back up or move data.
- As a **tenant user**, I want to build query/segment-based selections on attributes, so that I
  can target specific audiences.
- As a **tenant user**, I want to blocklist subscribers and manage the suppression list, so that
  I respect opt-outs and bounces.

## Epic E — Campaigns & Templates

- As a **tenant user**, I want to create campaigns in rich text, HTML, plain text, markdown, or a
  visual editor, so that I can author in my preferred format.
- As a **tenant user**, I want to create and reuse templates (campaign and transactional), so
  that I keep consistent branding.
- As a **tenant user**, I want to target campaigns to one or more lists/segments, so that the
  right people receive each message.
- As a **tenant user**, I want to schedule, run, pause, resume, and cancel campaigns, so that I
  control delivery timing.
- As a **tenant user**, I want to run A/B test variants, so that I can optimize subject lines and
  content.
- As a **tenant user**, I want to archive campaigns to a public page with RSS, so that content is
  publicly discoverable.
- As a **tenant developer**, I want to send transactional email via an API key and tx templates,
  so that my app can send password resets, receipts, etc.

## Epic F — Sending & Analytics

- As a **tenant**, I want my campaigns to send through Yandex Postbox within my plan's rate
  limit, so that delivery is reliable and compliant.
- As a **tenant user**, I want to see opens, clicks, bounces, and complaints per campaign, so
  that I can measure performance.
- As a **tenant user**, I want a dashboard of subscriber/list/campaign stats and charts, so that
  I get an at-a-glance overview.
- As a **tenant**, I want hard bounces and complaints automatically suppressed, so that my
  sender reputation stays healthy.

## Epic G — Public Pages

- As a **subscriber**, I want to subscribe via a tenant's public subscription page, so that I can
  opt in to their lists.
- As a **subscriber**, I want to receive and confirm double-opt-in emails, so that my consent is
  verified.
- As a **subscriber**, I want to manage preferences and unsubscribe, so that I control what I
  receive.
- As a **reader**, I want to view a tenant's public campaign archive and RSS feed, so that I can
  browse past content.

## Epic H — Media & Settings

- As a **tenant user**, I want to upload and manage media stored in object storage and scoped to
  my tenant, so that I can use images in campaigns.
- As a **tenant user**, I want to configure tenant settings, branding, and privacy options, so
  that the experience matches my brand and policies.

---

## Traceability — Epics to Implementation Phases

| Epic | Primary phases (see implementation-plan.md) |
|---|---|
| A — Tenant Onboarding & Platform | Phase 1, Phase 7 |
| B — Billing & Usage | Phase 5 |
| C — Sending Domains & Deliverability | Phase 3 |
| D — Lists & Subscribers | Phase 2 |
| E — Campaigns & Templates | Phase 3, Phase 7 |
| F — Sending & Analytics | Phase 3, Phase 4 |
| G — Public Pages | Phase 6 |
| H — Media & Settings | Phase 2, Phase 6 |
