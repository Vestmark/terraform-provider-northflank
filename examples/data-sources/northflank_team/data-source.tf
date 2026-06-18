data "northflank_team" "main" {
  name = "Acme Engineering"
}

output "team_id" {
  value = data.northflank_team.main.id
}
