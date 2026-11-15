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
