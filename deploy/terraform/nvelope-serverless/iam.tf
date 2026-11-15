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
