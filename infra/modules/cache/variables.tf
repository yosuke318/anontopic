variable "name_prefix" {
  description = "リソース名の接頭辞（例: anontopic-dev）。レプリケーショングループの ID にもなる。"
  type        = string
}

variable "subnet_ids" {
  description = "キャッシュサブネットグループに入れるプライベートサブネットの ID。"
  type        = list(string)
}

variable "security_group_id" {
  description = "ノードに付けるセキュリティグループの ID。"
  type        = string
}

variable "node_type" {
  description = "ノードタイプ（例: cache.t4g.micro）。"
  type        = string
}
