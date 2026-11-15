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
    condition     = yamldecode(yandex_api_gateway.public.spec).paths["/metrics"]["x-yc-apigateway-any-method"]["x-yc-apigateway-integration"] == { type = "dummy", http_code = 404, content = { "text/plain" = "not found" } }
    error_message = "/metrics must be answered 404 by the gateway, not routed to a container"
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
