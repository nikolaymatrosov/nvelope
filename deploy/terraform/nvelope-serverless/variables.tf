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
