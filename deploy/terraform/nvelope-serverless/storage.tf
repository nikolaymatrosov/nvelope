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
