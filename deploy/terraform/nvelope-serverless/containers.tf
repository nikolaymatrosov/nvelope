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
    command = "printf 'Authorization: Bearer %s\\n' \"$${YC_TOKEN:-$(yc iam create-token)}\" | curl -fsS --max-time 900 --retry 3 --retry-all-errors -X POST -H @- ${trimsuffix(yandex_serverless_container.worker.url, "/")}/admin/migrate"
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

# Every container is private. Anyone who can invoke worker can start campaigns,
# send emails, inject bounces and run migrations, so only the triggers (and the
# operator's deploy step) may invoke it. The gateway may invoke only api and
# frontend.
resource "yandex_serverless_container_iam_member" "invoker" {
  for_each = {
    api      = { container = yandex_serverless_container.api.id, sa = "gateway" }
    frontend = { container = yandex_serverless_container.frontend.id, sa = "gateway" }
    worker   = { container = yandex_serverless_container.worker.id, sa = "trigger" }
  }
  container_id = each.value.container
  role         = "serverless.containers.invoker"
  member       = "serviceAccount:${yandex_iam_service_account.sa[each.value.sa].id}"
}
