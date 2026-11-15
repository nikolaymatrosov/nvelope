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
    # ponytail: step must stay below 60 minutes; for longer intervals switch to an hourly "0 0/N" form.
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
