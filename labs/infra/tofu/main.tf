locals {
  zones   = yamldecode(file("${path.module}/../zones.yaml"))
  profile = yamldecode(file("${path.module}/../profiles/${var.profile}.yaml"))

  zone_ids = { for z, _ in local.zones.zones : z => tonumber(substr(z, 1, 1)) }

  # Mỗi VM: danh sách NIC {zone, ip, mac, ifname}. fw-1 có thêm NIC uplink.
  vms = { for v in local.profile.vms : v.name => merge(v, {
    nics = [for z in v.zones : {
      zone   = z
      net    = "lab-${lower(z)}"
      cidr   = local.zones.zones[z].cidr
      ip     = cidrhost(local.zones.zones[z].cidr, v.host)
      gw     = cidrhost(local.zones.zones[z].cidr, 1)
      mac    = format("00:16:3e:10:%02x:%02x", local.zone_ids[z], v.host)
      ifname = v.name == "fw-1" ? lower(z) : "lab0"
    }]
  }) }

  fw      = local.vms["fw-1"]
  others  = { for k, v in local.vms : k => v if k != "fw-1" }
  up_cidr = local.zones.uplink.cidr
}

# Một cầu ảo cho mỗi vùng. Máy chủ không có IP trên cầu (ipv4.address = none): không có đường định tuyến
# giữa các vùng ngoài fw-1.
resource "incus_network" "zone" {
  for_each = local.zones.zones
  name     = "lab-${lower(each.key)}"
  type     = "bridge"
  config = {
    "ipv4.address" = "none"
    "ipv6.address" = "none"
  }
}

# Cầu NAT cho fw-1 ra Internet thật. Chỉ fw-1 gắn vào đây.
resource "incus_network" "uplink" {
  name = "lab-up"
  type = "bridge"
  config = {
    "ipv4.address" = "${cidrhost(local.up_cidr, 1)}/24"
    "ipv4.nat"     = "true"
    "ipv6.address" = "none"
  }
}

resource "incus_instance" "fw" {
  name  = "fw-1"
  type  = "virtual-machine"
  image = var.image

  config = {
    "limits.cpu"    = local.fw.cpu
    "limits.memory" = local.fw.mem
    "cloud-init.user-data" = templatefile("${path.module}/../cloud-init/fw.yaml.tftpl", {
      ssh_key = var.ssh_public_key
      ruleset = file("${var.generated_dir}/${var.profile}/ruleset.nft")
    })
    "cloud-init.network-config" = templatefile("${path.module}/../cloud-init/network-fw.yaml.tftpl", {
      nics      = local.fw.nics
      up_mac    = format("00:16:3e:10:fe:%02x", local.zones.uplink.fw_host)
      up_ip     = cidrhost(local.up_cidr, local.zones.uplink.fw_host)
      up_gw     = cidrhost(local.up_cidr, 1)
    })
    "security.secureboot" = "true"
  }

  device {
    name = "root"
    type = "disk"
    properties = {
      path = "/"
      pool = var.storage_pool
      size = local.fw.disk
    }
  }

  dynamic "device" {
    for_each = local.fw.nics
    content {
      name = "nic-${lower(device.value.zone)}"
      type = "nic"
      properties = {
        network = incus_network.zone[device.value.zone].name
        hwaddr  = device.value.mac
        "security.mac_filtering" = "true"
      }
    }
  }

  device {
    name = "nic-up"
    type = "nic"
    properties = {
      network = incus_network.uplink.name
      hwaddr  = format("00:16:3e:10:fe:%02x", local.zones.uplink.fw_host)
      "security.mac_filtering" = "true"
    }
  }
}

resource "incus_instance" "vm" {
  for_each = local.others
  name     = each.key
  type     = "virtual-machine"
  image    = var.image

  # Dựng sau fw-1: lab up cũng chờ fw-1 xong cloud-init trước khi apply phần này.
  depends_on = [incus_instance.fw]

  config = {
    "limits.cpu"    = each.value.cpu
    "limits.memory" = each.value.mem
    "cloud-init.user-data" = templatefile("${path.module}/../cloud-init/base.yaml.tftpl", {
      hostname = each.key
      ssh_key  = var.ssh_public_key
      roles    = each.value.roles
    })
    "cloud-init.network-config" = templatefile("${path.module}/../cloud-init/network-vm.yaml.tftpl", {
      nic = each.value.nics[0]
      dns = cidrhost(local.up_cidr, 1)
    })
    "security.secureboot" = "true"
  }

  device {
    name = "root"
    type = "disk"
    properties = {
      path = "/"
      pool = var.storage_pool
      size = each.value.disk
    }
  }

  device {
    name = "nic-${lower(each.value.nics[0].zone)}"
    type = "nic"
    properties = {
      network = incus_network.zone[each.value.nics[0].zone].name
      hwaddr  = each.value.nics[0].mac
      "security.mac_filtering" = "true"
    }
  }
}
