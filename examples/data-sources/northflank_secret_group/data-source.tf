data "northflank_secret_group" "shared" {
  project_id = "my-project"
  id         = "shared-secrets"
}

output "db_host" {
  value     = data.northflank_secret_group.shared.variables["DB_HOST"]
  sensitive = true
}
