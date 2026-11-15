locals {
  # Public routes: OpenAPI path => container. API Gateway prefers fixed
  # segments over parameters and parameters over greedy {x+} segments, and of
  # two greedy routes the longer one, so the three BFF paths win over
  # /t/{slug}/api/{path+}, which wins over /{path+}. /metrics is answered 404
  # by the gateway itself, so the frontend catch-all cannot expose the BFF's
  # metrics; /healthz is not routed to api.
  public_routes = {
    "/"                                   = "frontend"
    "/metrics"                            = "dummy"
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
            api = {
              type               = "serverless_containers"
              container_id       = yandex_serverless_container.api.id
              service_account_id = yandex_iam_service_account.sa["gateway"].id
            }
            frontend = {
              type               = "serverless_containers"
              container_id       = yandex_serverless_container.frontend.id
              service_account_id = yandex_iam_service_account.sa["gateway"].id
            }
            dummy = {
              type      = "dummy"
              http_code = 404
              content   = { "text/plain" = "not found" }
            }
          }[to]
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
