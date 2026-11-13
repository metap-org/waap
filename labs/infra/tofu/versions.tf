terraform {
  required_version = ">= 1.8"
  required_providers {
    incus = {
      source  = "lxc/incus"
      version = "~> 1.0"
    }
  }
}

provider "incus" {
  # Mặc định nói chuyện với Incus cục bộ (socket unix). Đổi remote ở đây nếu lab ở máy khác.
}
