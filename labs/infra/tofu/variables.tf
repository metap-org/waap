variable "profile" {
  description = "Profile tài nguyên: tên tệp trong labs/infra/profiles (compact, ...)."
  type        = string
  default     = "compact"
}

variable "ssh_public_key" {
  description = "Khoá công khai cho tài khoản ops (chỉ dùng cho bootstrap; sau P2 chuyển sang cert SSH ngắn hạn do OpenBao ký)."
  type        = string
}

variable "image" {
  description = "Ảnh gốc, Ubuntu 24.04 LTS theo ADR-0048."
  type        = string
  default     = "images:ubuntu/24.04/cloud"
}

variable "storage_pool" {
  type    = string
  default = "default"
}

variable "generated_dir" {
  description = "Thư mục do `lab gen` ghi (ruleset.nft). Chạy gen_nft.py trước khi plan."
  type        = string
  default     = "../generated"
}
