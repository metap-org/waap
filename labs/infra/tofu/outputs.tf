output "addresses" {
  description = "Địa chỉ từng VM theo vùng."
  value       = { for k, v in local.vms : k => { for n in v.nics : n.zone => n.ip } }
}
