# Serverless On-Prem — Plan C3 (packaging) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- **A Terraform module.** `deploy/terraform/nvelope-serverless/` installs a single-tenant nvelope into a customer's YC folder with one `terraform apply`. It creates:
  - serverless YDB, a feedback topic, YMQ queues with DLQs, a media bucket and a Lockbox secret;
  - the `api`, `worker` and `frontend` Serverless Containers;
  - two API gateways and five triggers.
- **Image publishing.** A tag-triggered GitHub Actions workflow publishes the `api`, `worker` and `frontend` images to the project's public Container Registry.

**Architecture:**
- **One flat module, one file per concern:**
  - `storage.tf`: the YDB database, topic, bucket and queues;
  - `secrets.tf`: the Lockbox secret;
  - `iam.tf`: service accounts and role bindings;
  - `containers.tf`: the containers, the migrate step and the invoker bindings;
  - `gateway.tf`: the gateways, certificate and DNS;
  - `triggers.tf` and `outputs.tf`.
  The module is also the root module: it configures the `yandex` provider from `var.folder_id`.
- **Plan-only tests.** `tests/module.tftest.hcl` runs with mock providers. It pins the security- and runtime-relevant wiring: routes, invoker bindings, timeouts, trigger paths.
- **Nothing applies before C4.** The worker's serverless mode, which serves `/admin/migrate`, comes in C4, so an apply would stop at the migrate step. C3's evidence is `fmt`, `validate`, `test` and a read-only `plan`.
- **No Go changes**, except one env-gated probe test that is skipped by default.

**Tech Stack:**
- Terraform ≥ 1.7 (1.16.1 installed at `~/bin/terraform`)
- `yandex-cloud/yandex` `~> 0.248` (0.248.0), `hashicorp/random` `~> 3.9` (3.9.1)
- GitHub Actions: `hashicorp/setup-terraform@v3`, plain `docker build`/`docker push` with BuildKit
- Go 1.26, `aws-sdk-go-v2/service/sqs` v1.42.27 (already in `go.mod`), testify

**Design spec:** [docs/superpowers/specs/2026-11-14-serverless-onprem-design.md](../specs/2026-11-14-serverless-onprem-design.md). The binding sections are:
- "Containers";
- "On-prem packaging → Terraform module";
- "Plan C1 planning spike → Carried to C3 and C4";
- **"Plan C3 planning spike"**, which holds every decision this plan implements;
- "Ceilings".

**Scope:** The third of four Plan C plans. Out of scope here:
- config keys: C2 adds `NVELOPE_RUNTIME`, `NVELOPE_TENANT_SLUG` and `NVELOPE_SETUP_TOKEN`; C4 adds the rest;
- the serverless modes of `cmd/api` and `cmd/worker` (C4);
- the README quickstart, the Postbox setup, DLQ alerts and the smoke test (C4);
- the setup-token signup (C2).

## Global Constraints

- **Branch.** Work in the worktree `/Users/nikthespirit/experiments/nvelope-wt/serverless-c3` on branch `feat/serverless-c3`. The main checkout and the other `nvelope-wt/*` worktrees belong to other sessions; do not touch them.
- **This worktree is outside the session's working directories, so `cd` does not persist between Bash calls.** A later call starts in the main checkout. Start **every** Bash command with `cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 && ` (or `cd …/serverless-c3/deploy/terraform/nvelope-serverless && `), and give Read, Edit and Write absolute paths under that worktree. A relative path silently reads or writes the main checkout.
- **Never `terraform apply`, and never create a real cloud resource**, except in the steps marked **"needs the user's go-ahead"**. Skip those, and report that you skipped them, unless the user has explicitly said yes in this session. `terraform plan` is read-only and allowed.
- **Providers.** `~/.terraformrc` routes `registry.terraform.io` through the YC mirror, so `terraform init` just works. `terraform providers lock` ignores that file: pass `-net-mirror=https://terraform-mirror.yandexcloud.net/`.
- **Tests are plan-only.** `terraform test` with `command = apply` runs the migrate step's `local-exec` (a real `curl` and `yc iam create-token`), even with `terraform_data.migrate` overridden. Every `run` block uses `command = plan`. Values an assertion reads at plan time are mocked with `override_during = plan`.
- **Exact values** (spec "Containers", "Plan C3 planning spike"):

  | Setting | Value |
  |---------|-------|
  | `worker` | concurrency 1, `execution_timeout` 3600 s |
  | `api`, `frontend` | concurrency 16, `execution_timeout` 30 s |
  | Job triggers | `batch_size` 1 |
  | Visibility timeout | `jobs` 120 s, `jobs-long` 3900 s |
  | `maxReceiveCount` | `jobs` 20, `jobs-long` 10 |
  | DLQ retention | 14 days |
  | Feedback trigger | 5 retries, 60 s apart, then `nvelope-feedback-dlq` |
  | Worker paths | `/jobs`, `/cron/domain-verify-sweep`, `/cron/analytics-refresh`, `/feedback`, `/admin/migrate` |
- **The env var names are a contract.** C2 fixed `NVELOPE_RUNTIME`, `NVELOPE_TENANT_SLUG` and `NVELOPE_SETUP_TOKEN`. C4 adds config keys for exactly the other new names this module sets (see "Handover to Plan C4"). Do not rename them.
- **SaaS must not change.** No existing `.go` file is edited. The only Go file is the new, env-gated `internal/platform/ymq/real_ymq_test.go`.
- **Commits.** Conventional messages. **No `Co-Authored-By` trailer** of any kind.
- **Tooling.** `terraform fmt` is the formatter. Workflow YAML is syntax-checked with the system Ruby: `ruby -ryaml -e 'ARGV.each { |f| YAML.safe_load(File.read(f)) }' <files>`. actionlint is not installed. Go tests need `export DOCKER_HOST="unix://$HOME/.orbstack/run/docker.sock" && ` only when they touch fakecloud; the probe test does not run any.

## File structure

All module paths are under `deploy/terraform/nvelope-serverless/`.

| File | Responsibility |
|------|----------------|
| `versions.tf` | Terraform and provider versions, the `yandex` provider block |
| `variables.tf` | The module inputs |
| `storage.tf` | Serverless YDB, the feedback topic, the media bucket, the job queues and DLQs |
| `secrets.tf` | Setup token, TOTP key, the Lockbox secret and its version |
| `iam.tf` | Service accounts, folder and resource role bindings, static keys |
| `containers.tf` | The three containers, the migrate step, the per-container invoker bindings |
| `gateway.tf` | The public and `bff` API gateways, the certificate and the DNS records |
| `triggers.tf` | Two Message Queue triggers, two timers, one Data Streams trigger |
| `outputs.tf` | URL, gateway domain, setup-token command, feedback topic, DLQ names |
| `tests/module.tftest.hcl` | Plan-only mock-provider tests |
| `.terraform.lock.hcl` | Provider hashes for linux_amd64, darwin_arm64, darwin_amd64, windows_amd64 |
| `.gitignore` | `.terraform/`, state files, `terraform.tfvars` |
| `.github/workflows/ci.yml` | + a `terraform` job; + a static-asset size guard in the `frontend` job |
| `.github/workflows/release.yml` | Builds and pushes the three images on a `v*` tag |
| `internal/platform/ymq/real_ymq_test.go` | The env-gated real-YMQ round trip |

---

### Task 1: The module's data layer, secrets and service accounts

**Files:**
- Create: `deploy/terraform/nvelope-serverless/{versions,variables,storage,secrets,iam}.tf`, `.gitignore`, `.terraform.lock.hcl`
- Test: `deploy/terraform/nvelope-serverless/tests/module.tftest.hcl`

**Interfaces:**
- Produces, for later tasks:
  - `yandex_iam_service_account.sa["app" | "gateway" | "trigger" | "queues"]`
  - `yandex_resourcemanager_folder_iam_member.sa`, plus `yandex_ydb_database_iam_member.app`, `yandex_lockbox_secret_iam_member.app` and `yandex_storage_bucket_iam_binding.app`
  - `yandex_ydb_database_serverless.db`, with `.ydb_full_endpoint` and `.database_path`
  - `yandex_ydb_topic.feedback`, with `.name`
  - `yandex_storage_bucket.media`, with `.bucket`
  - `yandex_message_queue.job["jobs" | "jobs-long"]` (`.id` is the queue URL, `.arn`) and `yandex_message_queue.dlq["jobs" | "jobs-long" | "feedback"]`
  - `yandex_lockbox_secret.app` and `yandex_lockbox_secret_version.app`, with the entries `setup_token`, `totp_encryption_key`, `access_key_id` and `secret_access_key`
  - every variable in `variables.tf`

- [ ] **Step 1: Write the failing test**

Create `deploy/terraform/nvelope-serverless/tests/module.tftest.hcl`. The header (mocks, overrides, variables) serves every later task; later tasks append `run` blocks.

```hcl
# Plan-only tests with mock providers. An apply would run the migrate step's
# curl, even with terraform_data.migrate overridden. Values the assertions read
# are mocked at plan time.
mock_provider "yandex" {
  mock_resource "yandex_serverless_container" {
    override_during = plan
    defaults = {
      id  = "bbacontainer"
      url = "https://bbacontainer.containers.yandexcloud.net/"
    }
  }
}
mock_provider "random" {}

override_resource {
  target          = yandex_iam_service_account.sa["gateway"]
  override_during = plan
  values          = { id = "ajegateway" }
}

override_resource {
  target          = yandex_iam_service_account.sa["trigger"]
  override_during = plan
  values          = { id = "ajetrigger" }
}

variables {
  folder_id            = "b1gtestfolder"
  domain               = "mail.example.com"
  certificate_id       = "fpqtestcert"
  image_tag            = "v0.0.0-test"
  registry_id          = "crptestregistry"
  tenant_slug          = "acme"
  system_sender_domain = "example.com"
}

run "queues" {
  command = plan

  assert {
    condition     = yandex_message_queue.job["jobs-long"].visibility_timeout_seconds > 3600
    error_message = "the long queue must hide a message for longer than the worker's 1 h timeout"
  }
  assert {
    condition     = yandex_message_queue.job["jobs"].visibility_timeout_seconds > 60
    error_message = "the short queue must hide a message for longer than a short job (60 s) can run"
  }
  assert {
    condition     = length(yandex_message_queue.dlq) == 3
    error_message = "both job queues and the feedback trigger need a dead-letter queue"
  }
}
```

- [ ] **Step 2: Run the test to verify it fails**

Write `versions.tf` first (Step 3's first file), so `terraform init` has providers to install. Then:

Run: `cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3/deploy/terraform/nvelope-serverless && terraform init -input=false && terraform test`
Expected: FAIL with `Reference to undeclared resource` for `yandex_message_queue.job`.

- [ ] **Step 3: Write the module files**

`versions.tf`:

```hcl
terraform {
  required_version = ">= 1.7"
  required_providers {
    yandex = {
      source  = "yandex-cloud/yandex"
      version = "~> 0.248"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.9"
    }
  }
}

# Credentials come from the environment (YC_TOKEN or
# YC_SERVICE_ACCOUNT_KEY_FILE), as for any yandex provider run.
provider "yandex" {
  folder_id = var.folder_id
}
```

`variables.tf`:

```hcl
variable "folder_id" {
  type        = string
  description = "The YC folder nvelope is installed into."
}

variable "domain" {
  type        = string
  description = "The FQDN nvelope is served at, e.g. mail.example.com. It must be a subdomain: the module points it at the API gateway with a CNAME."
}

variable "dns_zone_id" {
  type        = string
  default     = null
  description = "A YC DNS zone that holds var.domain. When set, the module issues a Let's Encrypt certificate and creates the DNS records. Set exactly one of dns_zone_id and certificate_id."
}

variable "certificate_id" {
  type        = string
  default     = null
  description = "A Certificate Manager certificate for var.domain, for a domain whose DNS is not in YC. You then point var.domain at the gateway_domain output yourself."
}

variable "image_tag" {
  type        = string
  description = "The nvelope release to run, e.g. v0.1.0. Upgrade by changing it and applying."
}

variable "registry_id" {
  type        = string
  description = "The public Container Registry the nvelope images are published to."
}

variable "tenant_slug" {
  type        = string
  description = "The URL slug of the install's single organization (/t/<slug>/...)."
  validation {
    condition     = can(regex("^[a-z0-9](?:[a-z0-9-]{1,61}[a-z0-9])$", var.tenant_slug))
    error_message = "tenant_slug must be 3-63 lowercase letters, digits or hyphens, starting and ending with a letter or digit."
  }
}

variable "system_sender_domain" {
  type        = string
  description = "The Postbox-verified domain account emails (registration verification) are sent from, as no-reply@<domain>."
}

variable "send_rate_per_sec" {
  type        = number
  default     = 50
  description = "The campaign send rate cap, in emails per second."
}

variable "provisioned_instances" {
  type        = number
  default     = 0
  description = "Warm instances kept for each of api and frontend, to avoid cold starts. Above 0 they are billed while idle."
}

variable "ydb_throttling_rcu_limit" {
  type        = number
  default     = 1000
  description = "The serverless YDB throughput limit, in RU/s. 0 removes the limit. YDB's own default of 10 RU/s would make imports and sends crawl."
}

variable "deletion_protection" {
  type        = bool
  default     = true
  description = "Protects the database, the secret and the media bucket from terraform destroy. Set false only for throwaway installs (the smoke test)."
}
```

`storage.tf`:

```hcl
resource "yandex_ydb_database_serverless" "db" {
  name                = "nvelope"
  deletion_protection = var.deletion_protection
  serverless_database {
    enable_throttling_rcu_limit = var.ydb_throttling_rcu_limit > 0
    throttling_rcu_limit        = var.ydb_throttling_rcu_limit
  }
}

# Postbox writes delivery feedback here (a manual configuration-set step: the
# provider has no Postbox resources); the feedback trigger reads it.
resource "yandex_ydb_topic" "feedback" {
  database_endpoint = yandex_ydb_database_serverless.db.ydb_full_endpoint
  name              = "postbox-feedback"
  # The Data Streams trigger reads through a consumer it manages itself. Without
  # this, the next apply would delete it.
  lifecycle {
    ignore_changes = [consumer]
  }
}

# One bucket, private at bucket level: only published/ copies carry a
# public-read object ACL (media library design).
resource "yandex_storage_bucket" "media" {
  folder_id     = var.folder_id
  bucket_prefix = "nvelope-media-"
  force_destroy = !var.deletion_protection
  anonymous_access_flags {
    read        = false
    list        = false
    config_read = false
  }
}

locals {
  # Visibility timeouts. The worker bounds a short job to 60 s (NVELOPE_JOB_TIMEOUT)
  # and a long one to 55 min (NVELOPE_LONG_JOB_TIMEOUT, under the container's 1 h
  # timeout), so a message stays hidden for as long as its job can run.
  #
  # Retry budget. Every failed invocation, including a 429 when the container
  # scaling quota is exhausted, consumes one receive.
  # ponytail: fixed visibility, no backoff: short jobs get about 40 min of
  # retries, long ones about 11 h, then the DLQ. River gives 25 attempts with
  # exponential backoff. Upgrade path: a per-attempt delayed re-send in the
  # dispatcher.
  queues = {
    jobs        = { visibility_timeout = 120, max_receive_count = 20 }
    "jobs-long" = { visibility_timeout = 3900, max_receive_count = 10 }
  }
}

resource "yandex_message_queue" "dlq" {
  for_each                  = toset(["jobs", "jobs-long", "feedback"])
  name                      = "nvelope-${each.key}-dlq"
  message_retention_seconds = 1209600 # 14 days, the maximum, to give time to inspect and redrive
  access_key                = yandex_iam_service_account_static_access_key.queues.access_key
  secret_key                = yandex_iam_service_account_static_access_key.queues.secret_key
  depends_on                = [yandex_resourcemanager_folder_iam_member.sa]
}

resource "yandex_message_queue" "job" {
  for_each                   = local.queues
  name                       = "nvelope-${each.key}"
  visibility_timeout_seconds = each.value.visibility_timeout
  redrive_policy = jsonencode({
    deadLetterTargetArn = yandex_message_queue.dlq[each.key].arn
    maxReceiveCount     = each.value.max_receive_count
  })
  access_key = yandex_iam_service_account_static_access_key.queues.access_key
  secret_key = yandex_iam_service_account_static_access_key.queues.secret_key
  depends_on = [yandex_resourcemanager_folder_iam_member.sa]
}
```

`secrets.tf`:

```hcl
# The first-run setup token (POST /api/platform/signup). Letters and digits
# only, so it survives copy and paste.
resource "random_password" "setup_token" {
  length  = 40
  special = false
}

# ponytail: the TOTP key lives only here and in Terraform state. Losing the state
# replaces it on the next apply, and every enrolled TOTP secret becomes
# unreadable. Upgrade path: a lifecycle ignore_changes plus a documented rotation.
resource "random_id" "totp_key" {
  byte_length = 32
}

resource "yandex_lockbox_secret" "app" {
  name                = "nvelope"
  deletion_protection = var.deletion_protection
}

resource "yandex_lockbox_secret_version" "app" {
  secret_id = yandex_lockbox_secret.app.id
  entries {
    key        = "setup_token"
    text_value = random_password.setup_token.result
  }
  entries {
    key        = "totp_encryption_key"
    text_value = random_id.totp_key.hex
  }
  entries {
    key        = "access_key_id"
    text_value = yandex_iam_service_account_static_access_key.app.access_key
  }
  entries {
    key        = "secret_access_key"
    text_value = yandex_iam_service_account_static_access_key.app.secret_key
  }
}
```

`iam.tf`. The per-container invoker bindings come in Task 2, beside the containers.

```hcl
# Service accounts. app is the runtime identity of the api and worker
# containers: both build the same YDB, YMQ, Postbox and Object Storage clients,
# so separate accounts would carry identical roles. gateway invokes only api and
# frontend; trigger invokes only worker. queues exists only so Terraform can
# create the YMQ queues, whose API accepts nothing but static keys.
resource "yandex_iam_service_account" "sa" {
  for_each = toset(["app", "gateway", "trigger", "queues"])
  name     = "nvelope-${each.key}"
}

# Folder-level roles. YMQ, Postbox and YDS roles cannot be granted on a single
# queue, address or stream.
resource "yandex_resourcemanager_folder_iam_member" "sa" {
  for_each = toset([
    "app:ymq.writer",
    "app:postbox.editor", # domain identities (sending domains) and sends
    "trigger:ymq.reader", # the job queue triggers
    "trigger:ymq.writer", # the feedback trigger's dead-letter queue
    "trigger:yds.admin",  # the feedback trigger reads the Postbox topic
    "queues:ymq.admin",
  ])
  folder_id   = var.folder_id
  role        = split(":", each.key)[1]
  member      = "serviceAccount:${yandex_iam_service_account.sa[split(":", each.key)[0]].id}"
  sleep_after = 10 # IAM bindings take a few seconds to apply
}

resource "yandex_ydb_database_iam_member" "app" {
  database_id = yandex_ydb_database_serverless.db.id
  role        = "ydb.editor"
  member      = "serviceAccount:${yandex_iam_service_account.sa["app"].id}"
}

resource "yandex_lockbox_secret_iam_member" "app" {
  secret_id = yandex_lockbox_secret.app.id
  role      = "lockbox.payloadViewer"
  member    = "serviceAccount:${yandex_iam_service_account.sa["app"].id}"
}

resource "yandex_storage_bucket_iam_binding" "app" {
  bucket  = yandex_storage_bucket.media.bucket
  role    = "storage.editor" # storage.uploader cannot set the public-read ACL on published copies
  members = ["serviceAccount:${yandex_iam_service_account.sa["app"].id}"]
}

# One static key serves YMQ, Postbox and Object Storage for both containers.
resource "yandex_iam_service_account_static_access_key" "app" {
  service_account_id = yandex_iam_service_account.sa["app"].id
}

resource "yandex_iam_service_account_static_access_key" "queues" {
  service_account_id = yandex_iam_service_account.sa["queues"].id
}
```

`.gitignore`:

```gitignore
.terraform/
*.tfstate
*.tfstate.*
terraform.tfvars
```

- [ ] **Step 4: Lock the provider hashes, then run the checks**

Run:
```bash
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3/deploy/terraform/nvelope-serverless \
  && terraform providers lock -net-mirror=https://terraform-mirror.yandexcloud.net/ \
       -platform=linux_amd64 -platform=darwin_arm64 -platform=darwin_amd64 -platform=windows_amd64 \
  && terraform fmt -check -recursive && terraform validate && terraform test
```
Expected:
- `terraform validate` prints `Success! The configuration is valid.`
- `terraform test` prints `Success! 1 passed, 0 failed.`
- `.terraform.lock.hcl` holds 4 `h1:` hashes per provider.

- [ ] **Step 5: Commit**

```bash
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 \
  && git add deploy/terraform/nvelope-serverless \
  && git commit -m "feat(deploy): add the serverless Terraform module's data layer, secrets and service accounts"
```

---

### Task 2: The worker and api containers, the migrate step and the invoker bindings

**Files:**
- Create: `deploy/terraform/nvelope-serverless/containers.tf`
- Test: append to `deploy/terraform/nvelope-serverless/tests/module.tftest.hcl`

**Interfaces:**
- Consumes: everything Task 1 produces.
- Produces:
  - `yandex_serverless_container.worker` (`.id`, `.url`) and `yandex_serverless_container.api` (`.id`)
  - `terraform_data.migrate`
  - `yandex_serverless_container_iam_member.invoker` keyed `api` and `worker`; Task 3 adds `frontend`
  - `local.env`, `local.secret_env`, `local.image`, `local.verify_interval_minutes` (15) and `local.worker_timeout_seconds` (3600)

- [ ] **Step 1: Write the failing test**

Append to `tests/module.tftest.hcl`:

```hcl

run "containers" {
  command = plan

  assert {
    condition     = yandex_serverless_container.worker.concurrency == 1
    error_message = "the worker runs one job per instance"
  }
  assert {
    condition     = yandex_serverless_container.worker.image[0].environment["NVELOPE_LONG_JOB_TIMEOUT"] == "55m"
    error_message = "long jobs must stop 5 min before the worker's 1 h timeout"
  }
  assert {
    condition     = contains([for s in yandex_serverless_container.api.secrets : s.environment_variable], "NVELOPE_SETUP_TOKEN")
    error_message = "api needs the setup token"
  }
  assert {
    condition     = yandex_serverless_container_iam_member.invoker["worker"].member == "serviceAccount:ajetrigger"
    error_message = "only the trigger account may invoke worker"
  }
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3/deploy/terraform/nvelope-serverless && terraform test`
Expected: FAIL with `Reference to undeclared resource` for `yandex_serverless_container.worker`.

- [ ] **Step 3: Write `containers.tf`**

The env maps are the contract with C2 and C4 (see "Handover to Plan C4"). `NVELOPE_SETUP_TOKEN` goes to `worker` too, so the worker does not fail config validation if C2 requires the token whenever `NVELOPE_RUNTIME=serverless`.

```hcl
locals {
  image = "cr.yandex/${var.registry_id}"

  # The domain sweep must not fire more often than the verify interval
  # (cmd/scheduler's rule), so both come from this one value.
  verify_interval_minutes = 15

  env = {
    NVELOPE_RUNTIME     = "serverless"
    NVELOPE_TENANT_SLUG = var.tenant_slug
    NVELOPE_BASE_URL    = "https://${var.domain}"

    NVELOPE_YDB_DSN            = yandex_ydb_database_serverless.db.ydb_full_endpoint
    NVELOPE_YMQ_ENDPOINT       = "https://message-queue.api.cloud.yandex.net"
    NVELOPE_YMQ_QUEUE_URL      = yandex_message_queue.job["jobs"].id
    NVELOPE_YMQ_LONG_QUEUE_URL = yandex_message_queue.job["jobs-long"].id

    NVELOPE_OBJECT_STORAGE_ENDPOINT        = "https://storage.yandexcloud.net"
    NVELOPE_OBJECT_STORAGE_BUCKET          = yandex_storage_bucket.media.bucket
    NVELOPE_OBJECT_STORAGE_PUBLIC_BASE_URL = "https://storage.yandexcloud.net/${yandex_storage_bucket.media.bucket}"
    # ponytail: API Gateway caps a request at 2.5 MB, so uploads stay under
    # 2 MiB. Upgrade path: presigned uploads straight to the bucket.
    NVELOPE_MEDIA_MAX_BYTES = "2097152"

    NVELOPE_VERIFICATION_SENDER_DOMAIN     = var.system_sender_domain
    NVELOPE_GLOBAL_SEND_RATE_LIMIT         = tostring(var.send_rate_per_sec)
    NVELOPE_DEFAULT_TENANT_SEND_RATE_LIMIT = tostring(var.send_rate_per_sec)
    NVELOPE_SENDING_DOMAIN_VERIFY_INTERVAL = "${local.verify_interval_minutes}m"
  }

  # Environment variable => Lockbox entry. One static key serves YMQ, Postbox
  # and Object Storage.
  secret_env = {
    NVELOPE_SETUP_TOKEN                      = "setup_token"
    NVELOPE_TOTP_ENCRYPTION_KEY              = "totp_encryption_key"
    NVELOPE_YMQ_ACCESS_KEY_ID                = "access_key_id"
    NVELOPE_YMQ_SECRET_ACCESS_KEY            = "secret_access_key"
    NVELOPE_POSTBOX_ACCESS_KEY_ID            = "access_key_id"
    NVELOPE_POSTBOX_SECRET_ACCESS_KEY        = "secret_access_key"
    NVELOPE_OBJECT_STORAGE_ACCESS_KEY_ID     = "access_key_id"
    NVELOPE_OBJECT_STORAGE_SECRET_ACCESS_KEY = "secret_access_key"
  }

  # The worker's execution timeout is the platform maximum (a long-lived
  # container). Long jobs stop 5 min earlier, so they end cleanly and are
  # redelivered instead of being killed.
  worker_timeout_seconds = 3600
}

resource "yandex_serverless_container" "worker" {
  name               = "nvelope-worker"
  service_account_id = yandex_iam_service_account.sa["app"].id
  memory             = 1024
  cores              = 1
  concurrency        = 1 # one job per instance; throughput scales by instances
  execution_timeout  = "${local.worker_timeout_seconds}s"
  image {
    url = "${local.image}/worker:${var.image_tag}"
    environment = merge(local.env, {
      NVELOPE_ROLE             = "worker"
      NVELOPE_JOB_TIMEOUT      = "60s"
      NVELOPE_LONG_JOB_TIMEOUT = "${local.worker_timeout_seconds / 60 - 5}m"
    })
  }
  dynamic "secrets" {
    for_each = local.secret_env
    content {
      id                   = yandex_lockbox_secret.app.id
      version_id           = yandex_lockbox_secret_version.app.id
      key                  = secrets.value
      environment_variable = secrets.key
    }
  }
  depends_on = [
    yandex_lockbox_secret_iam_member.app,
    yandex_ydb_database_iam_member.app,
    yandex_resourcemanager_folder_iam_member.sa,
    yandex_storage_bucket_iam_binding.app,
  ]
}

# Applies the YDB migrations whenever the release changes, before api runs the
# new code. /admin/migrate is idempotent. The caller needs yc and the
# serverless.containers.invoker role on worker or on the folder.
resource "terraform_data" "migrate" {
  triggers_replace = [var.image_tag]
  provisioner "local-exec" {
    command = "curl -fsS --max-time 900 --retry 3 --retry-all-errors -X POST -H \"Authorization: Bearer $(yc iam create-token)\" ${trimsuffix(yandex_serverless_container.worker.url, "/")}/admin/migrate"
  }
}

resource "yandex_serverless_container" "api" {
  name               = "nvelope-api"
  service_account_id = yandex_iam_service_account.sa["app"].id
  memory             = 512
  cores              = 1
  concurrency        = 16
  execution_timeout  = "30s"
  image {
    url         = "${local.image}/api:${var.image_tag}"
    environment = merge(local.env, { NVELOPE_ROLE = "api" })
  }
  dynamic "secrets" {
    for_each = local.secret_env
    content {
      id                   = yandex_lockbox_secret.app.id
      version_id           = yandex_lockbox_secret_version.app.id
      key                  = secrets.value
      environment_variable = secrets.key
    }
  }
  dynamic "provision_policy" {
    for_each = var.provisioned_instances > 0 ? [1] : []
    content {
      min_instances = var.provisioned_instances
    }
  }
  depends_on = [terraform_data.migrate]
}

# Every container is private. Anyone who can invoke worker can start campaigns,
# send emails, inject bounces and run migrations, so only the triggers (and the
# operator's deploy step) may invoke it. The gateway may invoke only api and
# frontend.
resource "yandex_serverless_container_iam_member" "invoker" {
  for_each = {
    api    = { container = yandex_serverless_container.api.id, sa = "gateway" }
    worker = { container = yandex_serverless_container.worker.id, sa = "trigger" }
  }
  container_id = each.value.container
  role         = "serverless.containers.invoker"
  member       = "serviceAccount:${yandex_iam_service_account.sa[each.value.sa].id}"
}
```

- [ ] **Step 4: Run the checks**

Run: `cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3/deploy/terraform/nvelope-serverless && terraform fmt -check -recursive && terraform validate && terraform test`
Expected: `Success! 2 passed, 0 failed.`

- [ ] **Step 5: Sabotage check, then restore**

In `containers.tf`, change the `worker` entry of `yandex_serverless_container_iam_member.invoker` to `sa = "gateway"`. Run `terraform test`.

Expected: FAIL with `only the trigger account may invoke worker`.

Restore `sa = "trigger"` and re-run. Expected: `Success! 2 passed, 0 failed.`

- [ ] **Step 6: Commit**

```bash
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 \
  && git add deploy/terraform/nvelope-serverless \
  && git commit -m "feat(deploy): add the worker and api containers, the migrate step and per-container invoker bindings"
```

---

### Task 3: The gateways, the frontend container and the certificate

**Files:**
- Create: `deploy/terraform/nvelope-serverless/gateway.tf`
- Modify: `deploy/terraform/nvelope-serverless/containers.tf`: add the `frontend` container and its invoker entry
- Test: append to `deploy/terraform/nvelope-serverless/tests/module.tftest.hcl`

**Interfaces:**
- Consumes: `yandex_serverless_container.api`, `yandex_iam_service_account.sa["gateway"]`, `var.domain`, `var.dns_zone_id`, `var.certificate_id`.
- Produces:
  - `yandex_api_gateway.public` (`.domain`, `.spec`) and `yandex_api_gateway.bff` (`.domain`)
  - `yandex_serverless_container.frontend`
  - `local.public_routes`, `local.bff_paths` and `local.route_parameters`

**Why two gateways** (spec "Plan C3 planning spike"):
- A direct HTTPS call to a container loses its `Cookie` header.
- The BFF forwards each visual save to `${NV_GO_API_URL}/t/:slug/api/...`, which is the same path the public gateway routes to `frontend`.
- So the BFF reaches Go through the `bff` gateway, which routes only `/t/{slug}/api/{path+}`, and only to `api`.

- [ ] **Step 1: Write the failing test**

Append to `tests/module.tftest.hcl`:

```hcl

run "gateways" {
  command = plan

  assert {
    condition     = local.public_routes["/api/platform/{path+}"] == "api" && local.public_routes["/t/{slug}/api/{path+}"] == "api"
    error_message = "the platform and tenant APIs must route to api"
  }
  assert {
    condition     = local.public_routes["/t/{slug}/api/campaigns/{id}/visual"] == "frontend"
    error_message = "visual saves must route to the BFF"
  }
  assert {
    condition     = [for p in local.route_parameters["/t/{slug}/api/{path+}"] : p.name] == ["slug", "path"]
    error_message = "greedy and plain path parameters must both be declared"
  }
  assert {
    condition     = !strcontains(yandex_api_gateway.public.spec, "/metrics")
    error_message = "/metrics must not be public"
  }
  assert {
    condition     = yandex_serverless_container_iam_member.invoker["frontend"].member == "serviceAccount:ajegateway"
    error_message = "the gateway invokes frontend"
  }
  assert {
    condition     = length(yandex_serverless_container.frontend.secrets) == 0 && yandex_serverless_container.frontend.service_account_id == null
    error_message = "the frontend gets no secrets and no cloud identity"
  }
}

run "exactly_one_certificate_source" {
  command = plan
  variables {
    certificate_id = null
  }
  expect_failures = [yandex_api_gateway.public]
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3/deploy/terraform/nvelope-serverless && terraform test`
Expected: FAIL with `Reference to undeclared local value` (`public_routes`) or `undeclared resource` (`yandex_api_gateway.public`).

- [ ] **Step 3: Write `gateway.tf`**

```hcl
locals {
  # Public routes: OpenAPI path => container. API Gateway prefers fixed
  # segments over parameters and parameters over greedy {x+} segments, and of
  # two greedy routes the longer one, so the three BFF paths win over
  # /t/{slug}/api/{path+}, which wins over /{path+}. /metrics and /healthz are
  # deliberately not routed.
  public_routes = {
    "/"                                   = "frontend"
    "/{path+}"                            = "frontend"
    "/t/{slug}/api/campaigns/{id}/visual" = "frontend"
    "/t/{slug}/api/templates/{id}/visual" = "frontend"
    "/t/{slug}/api/render-preview"        = "frontend"
    "/api/platform/{path+}"               = "api"
    "/t/{slug}/api/{path+}"               = "api"
    "/o/{campaignId}"                     = "api"
    "/l/{linkId}"                         = "api"
    "/p/{token}"                          = "api"
    "/u/{token}"                          = "api"
    "/t/{slug}/subscribe/{pageSlug}"      = "api"
    "/t/{slug}/confirm/{token}"           = "api"
    "/t/{slug}/confirm/{token}/resend"    = "api"
    "/t/{slug}/archive"                   = "api"
    "/t/{slug}/archive/{campaignId}"      = "api"
    "/t/{slug}/feed.xml"                  = "api"
  }

  # The BFF's own route to Go. Its default domain is reachable from the
  # internet, but it serves only what the public gateway already serves from
  # api, plus api's own visual-save handlers, which sanitize and validate
  # themselves.
  bff_paths = ["/t/{slug}/api/{path+}"]
}

locals {
  # Path parameters, declared per route as OpenAPI requires. No resource
  # references here, so each gateway's spec depends only on its own containers
  # (frontend depends on the bff gateway).
  route_parameters = {
    for path in setunion(keys(local.public_routes), local.bff_paths) : path => [
      for p in regexall("{([^}+]+)\\+?}", path) :
      { name = p[0], in = "path", required = true, schema = { type = "string" } }
    ]
  }
}

resource "yandex_api_gateway" "public" {
  name = "nvelope"
  spec = yamlencode({
    openapi = "3.0.0"
    info    = { title = "nvelope", version = "1.0.0" }
    paths = {
      for path, to in local.public_routes : path => {
        "x-yc-apigateway-any-method" = {
          parameters = local.route_parameters[path]
          "x-yc-apigateway-integration" = {
            type               = "serverless_containers"
            container_id       = to == "api" ? yandex_serverless_container.api.id : yandex_serverless_container.frontend.id
            service_account_id = yandex_iam_service_account.sa["gateway"].id
          }
        }
      }
    }
  })
  custom_domains {
    fqdn           = var.domain
    certificate_id = local.certificate_id
  }
  lifecycle {
    precondition {
      condition     = (var.dns_zone_id == null) != (var.certificate_id == null)
      error_message = "Set exactly one of dns_zone_id and certificate_id."
    }
  }
}

resource "yandex_api_gateway" "bff" {
  name = "nvelope-bff"
  spec = yamlencode({
    openapi = "3.0.0"
    info    = { title = "nvelope-bff", version = "1.0.0" }
    paths = {
      for path in local.bff_paths : path => {
        "x-yc-apigateway-any-method" = {
          parameters = local.route_parameters[path]
          "x-yc-apigateway-integration" = {
            type               = "serverless_containers"
            container_id       = yandex_serverless_container.api.id
            service_account_id = yandex_iam_service_account.sa["gateway"].id
          }
        }
      }
    }
  })
}

# With a YC DNS zone: a Let's Encrypt certificate validated by a CNAME
# challenge, and the domain's own CNAME to the gateway.
resource "yandex_cm_certificate" "domain" {
  count   = var.dns_zone_id == null ? 0 : 1
  name    = "nvelope"
  domains = [var.domain]
  managed {
    challenge_type = "DNS_CNAME"
  }
}

resource "yandex_dns_recordset" "acme" {
  count   = length(yandex_cm_certificate.domain)
  zone_id = var.dns_zone_id
  name    = yandex_cm_certificate.domain[0].challenges[0].dns_name
  type    = yandex_cm_certificate.domain[0].challenges[0].dns_type
  data    = [yandex_cm_certificate.domain[0].challenges[0].dns_value]
  ttl     = 60
}

data "yandex_cm_certificate" "domain" {
  count           = length(yandex_cm_certificate.domain)
  certificate_id  = yandex_cm_certificate.domain[0].id
  wait_validation = true
  depends_on      = [yandex_dns_recordset.acme]
}

locals {
  certificate_id = var.dns_zone_id == null ? var.certificate_id : data.yandex_cm_certificate.domain[0].id
}

resource "yandex_dns_recordset" "domain" {
  count   = var.dns_zone_id == null ? 0 : 1
  zone_id = var.dns_zone_id
  name    = "${var.domain}."
  type    = "CNAME"
  ttl     = 600
  data    = ["${yandex_api_gateway.public.domain}."]
}
```

- [ ] **Step 4: Add the frontend container to `containers.tf`**

Insert this resource above the `# Every container is private.` comment:

```hcl
resource "yandex_serverless_container" "frontend" {
  name              = "nvelope-frontend"
  memory            = 512
  cores             = 1
  concurrency       = 16
  execution_timeout = "30s"
  image {
    url = "${local.image}/frontend:${var.image_tag}"
    environment = {
      NVELOPE_BASE_URL = "https://${var.domain}"
      # The BFF forwards each visual save to Go on the same path the public
      # gateway routes to frontend, so it goes through its own gateway.
      NV_GO_API_URL = "https://${yandex_api_gateway.bff.domain}"
    }
  }
  dynamic "provision_policy" {
    for_each = var.provisioned_instances > 0 ? [1] : []
    content {
      min_instances = var.provisioned_instances
    }
  }
}
```

Add the `frontend` entry to the invoker map, so the map reads:

```hcl
  for_each = {
    api      = { container = yandex_serverless_container.api.id, sa = "gateway" }
    frontend = { container = yandex_serverless_container.frontend.id, sa = "gateway" }
    worker   = { container = yandex_serverless_container.worker.id, sa = "trigger" }
  }
```

- [ ] **Step 5: Run the checks**

Run: `cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3/deploy/terraform/nvelope-serverless && terraform fmt -check -recursive && terraform validate && terraform test`
Expected: `Success! 4 passed, 0 failed.`

A `Cycle:` error from `validate` means a gateway spec references a resource the other gateway's containers depend on. `frontend` depends on the `bff` gateway, so `route_parameters` must stay free of resource references, and each gateway builds its own spec inline.

- [ ] **Step 6: Commit**

```bash
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 \
  && git add deploy/terraform/nvelope-serverless \
  && git commit -m "feat(deploy): add the public and bff API gateways, the frontend container and the managed certificate"
```

---

### Task 4: The triggers and the outputs

**Files:**
- Create: `deploy/terraform/nvelope-serverless/triggers.tf`, `deploy/terraform/nvelope-serverless/outputs.tf`
- Test: append to `deploy/terraform/nvelope-serverless/tests/module.tftest.hcl`

**Interfaces:**
- Consumes:
  - `yandex_message_queue.job`, `yandex_message_queue.dlq["feedback"]`, `yandex_serverless_container.worker` and `yandex_iam_service_account.sa["trigger"]`;
  - `yandex_ydb_database_serverless.db.database_path` and `yandex_ydb_topic.feedback.name`;
  - `local.verify_interval_minutes`;
  - the invoker and folder bindings, through `depends_on`.
- Produces: the outputs `url`, `gateway_domain`, `setup_token_command`, `feedback_topic` and `dead_letter_queues` (C4's README cites them).

- [ ] **Step 1: Write the failing test**

Append to `tests/module.tftest.hcl`:

```hcl

run "triggers" {
  command = plan

  assert {
    condition     = alltrue([for t in yandex_function_trigger.jobs : t.message_queue[0].batch_size == "1" && t.container[0].path == "/jobs"])
    error_message = "the job triggers deliver one message per /jobs call"
  }
  assert {
    condition     = yandex_function_trigger.cron["domain-verify-sweep"].timer[0].cron_expression == "0/15 * * * ? *" && yandex_serverless_container.worker.image[0].environment["NVELOPE_SENDING_DOMAIN_VERIFY_INTERVAL"] == "15m"
    error_message = "the domain sweep fires once per verify interval"
  }
  assert {
    condition     = yandex_function_trigger.feedback.container[0].path == "/feedback" && length(yandex_function_trigger.feedback.dlq) == 1
    error_message = "feedback goes to /feedback, with a dead-letter queue"
  }
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3/deploy/terraform/nvelope-serverless && terraform test`
Expected: FAIL with `Reference to undeclared resource` for `yandex_function_trigger.jobs`.

- [ ] **Step 3: Write `triggers.tf`**

```hcl
# One trigger per job queue, one message per invocation: the /jobs dispatcher
# assumes batch size 1. The queue's redrive policy is the retry budget (YMQ
# triggers have no retry settings of their own), and the trigger's
# visibility_timeout is left unset so the queue's applies.
resource "yandex_function_trigger" "jobs" {
  for_each = yandex_message_queue.job
  name     = "nvelope-${each.key}"
  message_queue {
    queue_id           = each.value.arn
    service_account_id = yandex_iam_service_account.sa["trigger"].id
    batch_size         = "1"
    batch_cutoff       = "0"
  }
  container {
    id                 = yandex_serverless_container.worker.id
    service_account_id = yandex_iam_service_account.sa["trigger"].id
    path               = "/jobs"
  }
  depends_on = [yandex_resourcemanager_folder_iam_member.sa, yandex_serverless_container_iam_member.invoker]
}

locals {
  # YC cron: minutes hours day-of-month month day-of-week year, in UTC.
  # ponytail: analytics refreshes hourly, not every minute as in SaaS, so an
  # idle install costs almost nothing; dashboards lag by up to an hour.
  cron = {
    "domain-verify-sweep" = "0/${local.verify_interval_minutes} * * * ? *"
    "analytics-refresh"   = "0 * * * ? *"
  }
}

resource "yandex_function_trigger" "cron" {
  for_each = local.cron
  name     = "nvelope-${each.key}"
  timer {
    cron_expression = each.value
  }
  container {
    id                 = yandex_serverless_container.worker.id
    service_account_id = yandex_iam_service_account.sa["trigger"].id
    path               = "/cron/${each.key}"
  }
  depends_on = [yandex_serverless_container_iam_member.invoker]
}

resource "yandex_function_trigger" "feedback" {
  name = "nvelope-feedback"
  data_streams {
    database           = yandex_ydb_database_serverless.db.database_path
    stream_name        = yandex_ydb_topic.feedback.name
    service_account_id = yandex_iam_service_account.sa["trigger"].id
    batch_size         = "65536" # bytes
    batch_cutoff       = "5"
  }
  container {
    id                 = yandex_serverless_container.worker.id
    service_account_id = yandex_iam_service_account.sa["trigger"].id
    path               = "/feedback"
    retry_attempts     = "5"
    retry_interval     = "60"
  }
  dlq {
    queue_id           = yandex_message_queue.dlq["feedback"].arn
    service_account_id = yandex_iam_service_account.sa["trigger"].id
  }
  depends_on = [yandex_resourcemanager_folder_iam_member.sa, yandex_serverless_container_iam_member.invoker]
}
```

- [ ] **Step 4: Write `outputs.tf`**

```hcl
output "url" {
  value = "https://${var.domain}"
}

output "gateway_domain" {
  value       = yandex_api_gateway.public.domain
  description = "With certificate_id: point a CNAME for var.domain here."
}

output "setup_token_command" {
  value       = "yc lockbox payload get --id ${yandex_lockbox_secret.app.id} --key setup_token"
  description = "Prints the one-time setup token the first signup needs."
}

output "feedback_topic" {
  value       = "${yandex_ydb_database_serverless.db.database_path}/${yandex_ydb_topic.feedback.name}"
  description = "The topic to pick as the Postbox configuration set's event destination."
}

output "dead_letter_queues" {
  value       = [for q in yandex_message_queue.dlq : q.name]
  description = "Alert on the depth of each of these."
}
```

- [ ] **Step 5: Run the checks and a read-only plan**

Run: `cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3/deploy/terraform/nvelope-serverless && terraform fmt -check -recursive && terraform validate && terraform test`
Expected: `Success! 7 passed, 0 failed.`

Then run a read-only plan against the user's default folder. It creates nothing and writes no state:

```bash
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3/deploy/terraform/nvelope-serverless \
  && export YC_TOKEN=$(yc iam create-token) \
  && terraform plan -input=false -lock=false -out=/dev/null \
       -var folder_id=$(yc config get folder-id) -var domain=nvelope-plan.example.com \
       -var certificate_id=fpqplancert -var image_tag=v0.0.0-plan -var registry_id=crpplanregistry \
       -var tenant_slug=acme -var system_sender_domain=example.com | grep -E '^Plan:|Error'
```
Expected: `Plan: 41 to add, 0 to change, 0 to destroy.`

With `-var dns_zone_id=dnsplanzone` in place of `-var certificate_id=…`, expect `Plan: 44 to add, …`.

If `yc` has no credentials, skip this and say so in the report.

- [ ] **Step 6: Commit**

```bash
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 \
  && git add deploy/terraform/nvelope-serverless \
  && git commit -m "feat(deploy): add the job, timer and feedback triggers and the module outputs"
```

---

### Task 5: CI for the module, the static-asset guard and image publishing

**Files:**
- Modify: `.github/workflows/ci.yml`: a new `terraform` job, and a step at the end of the `frontend` job
- Create: `.github/workflows/release.yml`

**Interfaces:**
- Consumes:
  - the module directory;
  - `deploy/docker/{api,worker,frontend}.Dockerfile`, unchanged. They take `--build-arg VERSION`. The frontend's Nitro server reads `PORT`, which Serverless Containers set.
- Produces: images `cr.yandex/${{ vars.YC_REGISTRY_ID }}/{api,worker,frontend}:<tag>` for each `v*` tag. Task 6 creates the repository variable `YC_REGISTRY_ID` and the secret `YC_CR_PUSHER_KEY`.

- [ ] **Step 1: Prove the guard logic on a scratch directory**

Run:
```bash
rm -rf /tmp/c3-guard && mkdir -p /tmp/c3-guard/.output/public && cd /tmp/c3-guard \
  && dd if=/dev/zero of=.output/public/ok.js bs=1024 count=2300 2>/dev/null \
  && guard() { big=$(find .output/public -type f -size +2400k); if [ -n "$big" ]; then echo "over the cap: $big"; return 1; fi; } \
  && guard && echo "2300k: pass" \
  && dd if=/dev/zero of=.output/public/big.js bs=1024 count=2500 2>/dev/null; guard || echo "2500k: fails as it should"
```
Expected: `2300k: pass`, then `over the cap: .output/public/big.js` and `2500k: fails as it should`.

- [ ] **Step 2: Add the guard to the `frontend` job**

In `.github/workflows/ci.yml`, insert this step right after the `frontend` job's `Build` step (`run: pnpm build`), before `Build Storybook`:

```yaml
      # API Gateway caps a response at 2.5 MB, so on the serverless install a
      # bigger static asset would not load.
      - name: Static assets fit API Gateway
        run: |
          big=$(find .output/public -type f -size +2400k)
          if [ -n "$big" ]; then echo "over the API Gateway response cap (2.5 MB): $big"; exit 1; fi
```

- [ ] **Step 3: Add the `terraform` job**

Append this job at the end of `ci.yml`, under `jobs:`:

```yaml
  terraform:
    name: Terraform (fmt · validate · test)
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: deploy/terraform/nvelope-serverless
    steps:
      - uses: actions/checkout@v4

      - name: Set up Terraform
        uses: hashicorp/setup-terraform@v3
        with:
          terraform_version: "1.16.1"

      - name: Format
        run: terraform fmt -check -recursive

      - name: Init
        run: terraform init -input=false

      - name: Validate
        run: terraform validate

      # Plan-only tests with mock providers: no cloud credentials needed.
      - name: Test
        run: terraform test
```

- [ ] **Step 4: Create `.github/workflows/release.yml`**

```yaml
name: Release images

# A version tag publishes the images the Terraform module
# (deploy/terraform/nvelope-serverless) runs: cr.yandex/<registry>/<name>:<tag>.
on:
  push:
    tags: ["v*"]

permissions:
  contents: read

jobs:
  images:
    name: Build and push (api · worker · frontend)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      # YC_CR_PUSHER_KEY is an authorized key (JSON) of a service account with
      # container-registry.images.pusher on the registry. YC_REGISTRY_ID is a
      # repository variable.
      - name: Log in to Container Registry
        env:
          KEY: ${{ secrets.YC_CR_PUSHER_KEY }}
        run: printf '%s' "$KEY" | docker login --username json_key --password-stdin cr.yandex

      # Serverless Containers run only linux/amd64 images.
      - name: Build and push
        env:
          REGISTRY: cr.yandex/${{ vars.YC_REGISTRY_ID }}
          TAG: ${{ github.ref_name }}
        run: |
          for name in api worker frontend; do
            docker build --platform linux/amd64 --build-arg VERSION="$TAG" \
              -f "deploy/docker/$name.Dockerfile" -t "$REGISTRY/$name:$TAG" .
            docker push "$REGISTRY/$name:$TAG"
          done
```

- [ ] **Step 5: Check the YAML syntax**

Run: `cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 && ruby -ryaml -e 'ARGV.each { |f| YAML.safe_load(File.read(f)); puts "ok #{f}" }' .github/workflows/ci.yml .github/workflows/release.yml`
Expected: `ok .github/workflows/ci.yml` and `ok .github/workflows/release.yml`.

- [ ] **Step 6: Commit**

```bash
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 \
  && git add .github/workflows/ci.yml .github/workflows/release.yml \
  && git commit -m "ci: test the serverless Terraform module, guard static asset sizes and publish release images"
```

---

### Task 6: The public Container Registry — **needs the user's go-ahead**

This creates real, long-lived resources in the maintainers' cloud:
- a public registry;
- a release service account and its key;
- a GitHub secret and a GitHub variable.

**Do not run Step 1 without an explicit yes from the user in this session.** Ask which folder hosts the registry. Without a yes, skip the task: `registry_id` stays a required input, and the report says so.

**Files:**
- Modify: `deploy/terraform/nvelope-serverless/variables.tf` (the `registry_id` default)

- [ ] **Step 1: Create the registry, the pusher account and the CI credentials** (needs the user's go-ahead)

```bash
FOLDER=<the folder the user named>
yc container registry create --name nvelope --folder-id "$FOLDER"
yc container registry add-access-binding --name nvelope --folder-id "$FOLDER" \
  --role container-registry.images.puller --subject system:allUsers
yc iam service-account create --name nvelope-release --folder-id "$FOLDER"
yc container registry add-access-binding --name nvelope --folder-id "$FOLDER" \
  --role container-registry.images.pusher \
  --subject serviceAccount:$(yc iam service-account get nvelope-release --folder-id "$FOLDER" --format json | jq -r .id)
yc iam key create --service-account-name nvelope-release --folder-id "$FOLDER" --output /tmp/nvelope-release-key.json
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 \
  && gh variable set YC_REGISTRY_ID --body "$(yc container registry get nvelope --folder-id "$FOLDER" --format json | jq -r .id)" \
  && gh secret set YC_CR_PUSHER_KEY < /tmp/nvelope-release-key.json \
  && rm /tmp/nvelope-release-key.json
```
Expected: each command succeeds. `gh variable list` shows `YC_REGISTRY_ID`, and `gh secret list` shows `YC_CR_PUSHER_KEY`.

- [ ] **Step 2: Make the registry ID the module default**

In `variables.tf`, give `registry_id` the default printed by `gh variable get YC_REGISTRY_ID`:

```hcl
variable "registry_id" {
  type        = string
  default     = "<the value of gh variable get YC_REGISTRY_ID>"
  description = "The public Container Registry the nvelope images are published to."
}
```

Run: `cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3/deploy/terraform/nvelope-serverless && terraform fmt -check && terraform validate && terraform test`
Expected: `Success! 5 passed, 0 failed.`

- [ ] **Step 3: Commit**

```bash
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 \
  && git add deploy/terraform/nvelope-serverless/variables.tf \
  && git commit -m "feat(deploy): default the module to the public nvelope registry"
```

---

### Task 7: The real-YMQ probe

The spec's carried item: confirm before C4 that real YMQ accepts the AWS JSON protocol that `service/sqs` v1.42.27 speaks. The test is committed and skipped by default. **Running it creates a real service account and queue: it needs the user's go-ahead.**

**Files:**
- Create: `internal/platform/ymq/real_ymq_test.go`

**Interfaces:**
- Consumes, from `internal/platform/ymq` (C1):
  - `NewClient(endpoint, region, accessKeyID, secretAccessKey string) *sqs.Client`;
  - `New(client, queueURL, longQueueURL string) *Enqueuer` and `(*Enqueuer).EnqueueAnalyticsRefresh(ctx, tenantID string) error`;
  - the test helpers `createQueue(t, client, name, dlqURL string) string` and `receive(t, client, queueURL string, wait time.Duration) *types.Message` (`helpers_test.go`, package `ymq`).

- [ ] **Step 1: Write the test**

```go
package ymq

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestRealYMQRoundTrip sends, receives and deletes one message on real Yandex
// Message Queue with the production client and Enqueuer. It confirms that YMQ
// accepts the AWS JSON protocol service/sqs speaks (fakecloud cannot). It is
// skipped unless NVELOPE_TEST_YMQ_ACCESS_KEY_ID and
// NVELOPE_TEST_YMQ_SECRET_ACCESS_KEY hold a static key of a service account
// with ymq.admin. It creates its own queue and deletes it.
func TestRealYMQRoundTrip(t *testing.T) {
	keyID, secret := os.Getenv("NVELOPE_TEST_YMQ_ACCESS_KEY_ID"), os.Getenv("NVELOPE_TEST_YMQ_SECRET_ACCESS_KEY")
	if keyID == "" || secret == "" {
		t.Skip("set NVELOPE_TEST_YMQ_ACCESS_KEY_ID and NVELOPE_TEST_YMQ_SECRET_ACCESS_KEY to probe real YMQ")
	}
	client := NewClient("https://message-queue.api.cloud.yandex.net", "ru-central1", keyID, secret)
	url := createQueue(t, client, "nvelope-probe-"+uuid.NewString()[:8], "")
	t.Cleanup(func() {
		_, err := client.DeleteQueue(context.Background(), &sqs.DeleteQueueInput{QueueUrl: aws.String(url)})
		require.NoError(t, err)
	})

	require.NoError(t, New(client, url, url).EnqueueAnalyticsRefresh(t.Context(), "probe-tenant"))
	msg := receive(t, client, url, 20*time.Second)
	require.NotNil(t, msg, "the enqueued message never arrived")
	require.JSONEq(t, `{"kind":"analytics.refresh","args":{"tenant_id":"probe-tenant"}}`, *msg.Body)
	_, err := client.DeleteMessage(t.Context(), &sqs.DeleteMessageInput{QueueUrl: aws.String(url), ReceiptHandle: msg.ReceiptHandle})
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run it without credentials to verify it skips**

Run: `cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 && gofmt -l internal/platform/ymq && go vet ./internal/platform/ymq && go test ./internal/platform/ymq -run TestRealYMQRoundTrip -v -count=1`
Expected: no `gofmt` output, then `--- SKIP: TestRealYMQRoundTrip` and `ok`.

- [ ] **Step 3: Commit**

```bash
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 \
  && git add internal/platform/ymq/real_ymq_test.go \
  && git commit -m "test(ymq): add an env-gated round trip against real Yandex Message Queue"
```

- [ ] **Step 4: Run the probe against real YMQ** (needs the user's go-ahead)

This creates a temporary service account with `ymq.admin`, and one queue that the test deletes. Without an explicit yes, skip this step and report it as pending.

```bash
FOLDER=$(yc config get folder-id)
yc iam service-account create --name nvelope-ymq-probe
yc resource-manager folder add-access-binding "$FOLDER" --role ymq.admin \
  --subject serviceAccount:$(yc iam service-account get nvelope-ymq-probe --format json | jq -r .id)
sleep 15  # IAM propagation
yc iam access-key create --service-account-name nvelope-ymq-probe --format json > /tmp/nvelope-ymq-probe.json
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 \
  && NVELOPE_TEST_YMQ_ACCESS_KEY_ID=$(jq -r .access_key.key_id /tmp/nvelope-ymq-probe.json) \
     NVELOPE_TEST_YMQ_SECRET_ACCESS_KEY=$(jq -r .secret /tmp/nvelope-ymq-probe.json) \
     go test ./internal/platform/ymq -run TestRealYMQRoundTrip -v -count=1; \
  yc resource-manager folder remove-access-binding "$FOLDER" --role ymq.admin \
    --subject serviceAccount:$(yc iam service-account get nvelope-ymq-probe --format json | jq -r .id); \
  yc iam service-account delete nvelope-ymq-probe; rm -f /tmp/nvelope-ymq-probe.json
```
Expected: `--- PASS: TestRealYMQRoundTrip`. The cleanup commands run whatever the outcome.

- [ ] **Step 5: Record the result in the spec**

In `docs/superpowers/specs/2026-11-14-serverless-onprem-design.md`, under "Plan C3 planning spike", replace the bullet that starts with `**The real-YMQ probe**` with one of these:
- **On PASS:** `- **Real-YMQ probe: PASS** (<date>). SendMessage through the production Enqueuer, then ReceiveMessage and DeleteMessage with service/sqs v1.42.27, against https://message-queue.api.cloud.yandex.net. YMQ accepts the AWS JSON protocol.`
- **On FAIL:** record the error text. The fallback, per the C1 carried item, is a query-protocol `service/sqs` release built on the current core. Do not attempt it in C3; report it.

Commit:
```bash
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 \
  && git add docs/superpowers/specs/2026-11-14-serverless-onprem-design.md \
  && git commit -m "docs: record the real-YMQ probe result"
```

---

## Done criteria for Plan C3

Run:
```bash
cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3/deploy/terraform/nvelope-serverless \
  && terraform fmt -check -recursive && terraform init -input=false && terraform validate && terraform test \
  && cd /Users/nikthespirit/experiments/nvelope-wt/serverless-c3 \
  && go vet ./internal/platform/ymq && go test ./internal/platform/ymq -run TestRealYMQRoundTrip -count=1 \
  && ruby -ryaml -e 'ARGV.each { |f| YAML.safe_load(File.read(f)) }' .github/workflows/*.yml \
  && git diff main...HEAD --stat
```

Expected:
- `terraform test` prints `Success! 5 passed, 0 failed.`
- The probe test prints `ok` (skipped without credentials).
- `git diff main...HEAD --stat` lists only:
  - `deploy/terraform/nvelope-serverless/**`;
  - `.github/workflows/ci.yml` and `.github/workflows/release.yml`;
  - `internal/platform/ymq/real_ymq_test.go`;
  - the spec;
  - this plan file.
- The report names each step skipped for lack of a go-ahead (Task 6, Task 7 Step 4).

## Handover to Plan C4

- **Config keys C4 adds.** The module already sets these names:
  - `NVELOPE_ROLE`: `api` or `worker`. Worker-only routes are registered only for `worker`.
  - `NVELOPE_YDB_DSN`: `ydb_full_endpoint`, e.g. `grpcs://ydb.serverless.yandexcloud.net:2135/?database=/ru-central1/…`. The containers authenticate to YDB as their service account through the metadata service: `ydb-go-yc`'s `yc.WithMetadataCredentials()` (already in `go.mod`). No key is passed.
  - `NVELOPE_YMQ_ENDPOINT` (`https://message-queue.api.cloud.yandex.net`), `NVELOPE_YMQ_QUEUE_URL` and `NVELOPE_YMQ_LONG_QUEUE_URL`. The region is `ru-central1`, which C4 defaults; the module does not set it.
  - `NVELOPE_YMQ_ACCESS_KEY_ID` and `NVELOPE_YMQ_SECRET_ACCESS_KEY`, from Lockbox. The same key backs Postbox and Object Storage.
  - `NVELOPE_JOB_TIMEOUT` (`60s`) and `NVELOPE_LONG_JOB_TIMEOUT` (`55m`): the `ymq.NewDispatcher` timeouts.
- **C2's keys**, set on both `api` and `worker`: `NVELOPE_RUNTIME=serverless`, `NVELOPE_TENANT_SLUG` and `NVELOPE_SETUP_TOKEN` (Lockbox, 40 letters and digits).
- **Existing keys the module sets:**
  - `NVELOPE_BASE_URL` and `NVELOPE_TOTP_ENCRYPTION_KEY`;
  - `NVELOPE_POSTBOX_{ACCESS_KEY_ID,SECRET_ACCESS_KEY}`;
  - `NVELOPE_OBJECT_STORAGE_{ENDPOINT,BUCKET,ACCESS_KEY_ID,SECRET_ACCESS_KEY,PUBLIC_BASE_URL}`;
  - `NVELOPE_MEDIA_MAX_BYTES` (2 MiB) and `NVELOPE_VERIFICATION_SENDER_DOMAIN`;
  - `NVELOPE_GLOBAL_SEND_RATE_LIMIT` and `NVELOPE_DEFAULT_TENANT_SEND_RATE_LIMIT` (both `send_rate_per_sec`);
  - `NVELOPE_SENDING_DOMAIN_VERIFY_INTERVAL` (`15m`).
- **Not set: C4 must stop `Validate` from requiring them in serverless mode.** `NVELOPE_DATABASE_URL`, `NVELOPE_REDIS_URL` and the four `NVELOPE_FEEDBACK_STREAM_*` keys.
- **Listen on `$PORT`.** The platform sets `PORT`, and its value is not documented. `cmd/api` and `cmd/worker` in serverless mode must listen on `:$PORT`, not `NVELOPE_HTTP_ADDR`. The frontend already reads `PORT`.
- **The first apply works only after C4**, because `terraform_data.migrate` calls the worker's `/admin/migrate`. The apply order is the worker, then the migration, then `api` (`depends_on`), then the public gateway.
- **README items:**
  - **Postbox.** Create the address and a configuration set whose event destination is the `feedback_topic` output. Check which role Postbox needs to write to the topic.
  - **DLQ alerts.** Set up YC Monitoring alerts on the depth of each `dead_letter_queues` output. The provider has no alert resource.
  - **Quotas.** Raise `serverless.containersInstances.count` and `serverless.containersRequest.count` (10 per zone by default, shared by every container) before sending real campaigns.
  - **State.** Terraform state holds the static keys, the setup token and the TOTP key: keep it in an encrypted remote backend.
  - **The operator** needs `yc` and `serverless.containers.invoker` on `worker` or on the folder.
  - **The domain** must be a subdomain (CNAME). The setup token comes from the `setup_token_command` output.
  - **Key rotation.** The `app` static key: replace `yandex_iam_service_account_static_access_key.app`, which writes a new Lockbox version, then roll new container revisions. Maintainer note: `YC_CR_PUSHER_KEY` is a long-lived key with no rotation. It could move into a GitHub environment limited to `v*` tags (maintainer decision, not done here).
  - **Analytics refresh** is hourly, so dashboards lag by up to an hour.
  - **`provisioned_instances`** applies to both `api` and `frontend`: N needs 2N instances against the default quota.
  - **One install per dedicated folder**: the module uses folder-wide roles and fixed resource names.
  - **Postbox events.** The app never sets `ConfigurationSetName` (`internal/platform/postbox/client.go`, CreateEmailIdentity and SendEmail), so in-app sending domains send no events to `feedback_topic`. C4 needs either a config key the module sets from a new input, or a README step attaching the configuration set to every address. `system_sender_domain` must be a verified Postbox address.
- **Smoke-test checks the docs could not settle:**
  - `Cookie` and `Authorization` pass through API Gateway: a login, then an API-key `POST /t/{slug}/api/tx`;
  - the narrower trigger roles fire: `ymq.reader` instead of folder `editor`, invoker on `worker` only, `yds.admin` on the folder;
  - `batch_cutoff = "0"` is accepted;
  - whether a 429 consumes a receive count;
  - the 2.2 MB SPA chunk loads through the gateway.
  - `/metrics/` (trailing slash) and its subpaths fall to the frontend catch-all: verify they 404;
  - an anonymous GET on `jobs/<tenant>/<job>` in the media bucket returns 403.
- **CI.** The lock file's `h1:` hashes come from the YC mirror and CI has never run. If the first CI `terraform init` fails on a checksum, regenerate the lock for linux_amd64 against the public registry.
- **Fan-out under the quota.** A campaign fan-out throttled by the container quota burns receives and waits the long visibility timeout (65 min) between tries. C4 (or the campaign Plan B) should stagger `campaign.batch` enqueues with `not_before`/`DelaySeconds`, besides the quota increase.
- **Feedback recovery** (from the C1 carried list) is still C4's: a sweep that re-enqueues `feedback.process` for staged events left unprocessed.
