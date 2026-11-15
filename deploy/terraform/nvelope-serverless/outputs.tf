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
