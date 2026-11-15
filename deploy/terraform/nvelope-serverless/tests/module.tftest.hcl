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
