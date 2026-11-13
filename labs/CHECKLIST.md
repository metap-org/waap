# Checklist chạy lab từng bước

Sinh tự động bởi `labs/infra/tools/steps.py doc` từ `labs/infra/steps.yaml` và bảng tuning ở `labs/ansible/roles/os_tuning/defaults/main.yml`. **Không sửa tay.**

Cách dùng (trên máy lab): `labs/bin/lab step list`, rồi từng bước `show` (đọc), `run` (chạy), `verify` (kiểm). `run` từ chối nếu bước phụ thuộc chưa `verify` đạt.

Giá trị trong cột **Phải là** là giá trị mục tiêu của profile `compact` (ước lượng, chưa đo trên máy thật). Mục **THỦ CÔNG** cần người đọc.

## P0. Máy chủ lab sẵn sàng

### P0.1 Máy chủ lab đủ điều kiện

**Vì sao:** Phát hiện sớm thiếu RAM, đĩa, ảo hoá lồng hay công cụ, trước khi dựng bất cứ thứ gì.

**Phụ thuộc:** không  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab check
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Incus đã cài | `host` | lệnh thành công (rc=0) | `incus --version` |
| KVM có (ảo hoá lồng bật) | `host` | lệnh thành công (rc=0) | `test -e /dev/kvm` |
| RAM máy chủ (GB) | `host` | ≥ 50 | `free -g \| awk '/Mem:/{print $2}'` |
| CPU luồng | `host` | ≥ 8 | `nproc` |
| Đĩa trống cho Incus (GB) | `host` | ≥ 200 | `df --output=avail -BG /var/lib/incus 2>/dev/null \| tail -1 \| tr -dc 0-9` |
| OpenTofu | `host` | lệnh thành công (rc=0) | `tofu --version` |
| Ansible | `host` | lệnh thành công (rc=0) | `ansible-playbook --version` |
| Ma trận luồng và luật sinh ra khớp (T07) | `host` | lệnh thành công (rc=0) | `python3 labs/infra/tools/gen_nft.py check compact` |

**Hoàn tác:** Không có gì để hoàn tác.

### P0.2 Khoá và bí mật của lab

**Vì sao:** SSH CA, mật khẩu ngẫu nhiên cho database. Nằm ngoài Git (generated/), quyền 0600.

**Phụ thuộc:** P0.1  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab ssh-ca
$ labs/bin/lab secrets
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Khoá riêng CA SSH quyền 600 | `host` | = 600 | `stat -c %a labs/infra/generated/ssh_user_ca` |
| secrets.yml quyền 600 | `host` | = 600 | `stat -c %a labs/infra/generated/secrets.yml` |
| secrets.yml không nằm trong Git | `host` | lệnh thành công (rc=0) | `git check-ignore -q labs/infra/generated/secrets.yml` |

**Hoàn tác:** Xoá labs/infra/generated/ (sinh lại được).

## P1. Mạng ảo và fw-1

### P1.1 Dựng 8 cầu ảo (7 vùng và uplink)

**Vì sao:** Mỗi vùng một cầu riêng, máy chủ không có IP trên cầu, nên không có đường định tuyến giữa các vùng ngoài fw-1.

**Phụ thuộc:** P0.2  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab tf net
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Đủ 8 cầu lab-* | `host` | = 8 | `incus network list -f csv \| grep -c '^lab-'` |
| Z0 không IP trên máy chủ | `host` | = none | `incus network get lab-z0 ipv4.address` |
| Z1 không IP trên máy chủ | `host` | = none | `incus network get lab-z1 ipv4.address` |
| Z2 không IP trên máy chủ | `host` | = none | `incus network get lab-z2 ipv4.address` |
| Z3 không IP trên máy chủ | `host` | = none | `incus network get lab-z3 ipv4.address` |
| Z4 không IP trên máy chủ | `host` | = none | `incus network get lab-z4 ipv4.address` |
| Z5 không IP trên máy chủ | `host` | = none | `incus network get lab-z5 ipv4.address` |
| Z6 không IP trên máy chủ | `host` | = none | `incus network get lab-z6 ipv4.address` |
| Uplink có NAT | `host` | = true | `incus network get lab-up ipv4.nat` |
| Uplink dải riêng | `host` | = 10.10.254.1/24 | `incus network get lab-up ipv4.address` |

**Hoàn tác:** labs/bin/lab destroy (xoá cả lab) hoặc tofu destroy -target=incus_network.zone

### P1.2 Boot fw-1 (router duy nhất giữa các vùng)

**Vì sao:** fw-1 có một card mạng ở mỗi vùng cộng uplink. Nó là router L3 duy nhất; không VM nào khác có hai card.

**Phụ thuộc:** P1.1  ·  **Thời gian dự kiến:** 3 phút

```
$ labs/bin/lab tf fw
$ incus exec fw-1 -- cloud-init status --wait
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| fw-1 đang chạy | `host` | = RUNNING | `incus list fw-1 -c s -f csv` |
| Secure Boot tắt (ảnh images:* không ký; ghi là rủi ro ADR-0045) | `host` | = false | `incus config get fw-1 security.secureboot` |
| RAM của fw-1 | `host` | = 1GiB | `incus config get fw-1 limits.memory` |
| vCPU của fw-1 | `host` | = 2 | `incus config get fw-1 limits.cpu` |
| cloud-init xong | `fw-1` | = done | `cloud-init status \| awk '{print $2}'` |
| Đủ 8 card mạng (z0 đến z6 và uplink) | `fw-1` | = 8 | `ls /sys/class/net \| grep -cE '^(z[0-6]\|uplink)$'` |
| IP z0 | `fw-1` | = 10.10.0.1/24 | `ip -4 -o addr show z0 \| awk '{print $4}'` |
| IP z1 | `fw-1` | = 10.10.10.1/24 | `ip -4 -o addr show z1 \| awk '{print $4}'` |
| IP z2 | `fw-1` | = 10.10.20.1/24 | `ip -4 -o addr show z2 \| awk '{print $4}'` |
| IP z3 | `fw-1` | = 10.10.30.1/24 | `ip -4 -o addr show z3 \| awk '{print $4}'` |
| IP z4 | `fw-1` | = 10.10.40.1/24 | `ip -4 -o addr show z4 \| awk '{print $4}'` |
| IP z5 | `fw-1` | = 10.10.50.1/24 | `ip -4 -o addr show z5 \| awk '{print $4}'` |
| IP z6 | `fw-1` | = 10.10.60.1/24 | `ip -4 -o addr show z6 \| awk '{print $4}'` |
| Định tuyến IP bật | `fw-1` | = 1 | `sysctl -n net.ipv4.ip_forward` |
| Ra Internet qua uplink | `fw-1` | lệnh thành công (rc=0) | `ping -c1 -W3 1.1.1.1 >/dev/null` |

**Hoàn tác:** incus delete -f fw-1 (tofu dựng lại được)

### P1.3 Luật nftables của fw-1 (chế độ bootstrap)

**Vì sao:** Default-deny, chỉ các luồng trong ma trận 7.6 được chuyển tiếp; bootstrap tạm cho các vùng ra Internet để cài gói (gỡ ở P9.1).

**Phụ thuộc:** P1.2  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab fw bootstrap
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| forward policy drop | `fw-1` | = 1 | `nft list chain inet lab forward \| grep -c 'policy drop'` |
| input policy drop | `fw-1` | = 1 | `nft list chain inet lab input \| grep -c 'policy drop'` |
| Số luật chuyển tiếp khớp ma trận ({fw_rules} luật) | `fw-1` | ≥ {fw_rules} | `nft list chain inet lab forward \| grep -c ' accept comment '` |
| Có luật bootstrap (tạm) | `fw-1` | ≥ 2 | `nft list chain inet lab forward \| grep -c BOOTSTRAP` |
| NAT ra uplink | `fw-1` | = 1 | `nft list table ip labnat \| grep -c masquerade` |
| SSH vào fw-1 chỉ từ bastion (10.10.60.11) | `fw-1` | = 1 | `nft list chain inet lab input \| grep -c '10.10.60.11'` |

**Hoàn tác:** labs/bin/lab fw bootstrap để nạp lại; xem luật bằng `incus exec fw-1 -- nft list ruleset`

## P2. Boot VM theo từng vùng

### P2.1 Boot Z6 Vận hành (bastion, ops-1)

**Vì sao:** Boot một vùng một lần, kiểm trước khi sang vùng sau. Vận hành đứng đầu vì bastion là cửa quản trị.

**Phụ thuộc:** P1.3  ·  **Thời gian dự kiến:** 3 phút

```
$ labs/bin/lab tf vm bastion ops-1
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| bastion đang chạy | `host` | = RUNNING | `incus list bastion -c s -f csv` |
| bastion RAM cấp theo profile | `host` | = 512MiB | `incus config get bastion limits.memory` |
| bastion số vCPU theo profile | `host` | = 1 | `incus config get bastion limits.cpu` |
| cloud-init xong | `bastion` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z6 | `bastion` | = 10.10.60.11/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `bastion` | = 10.10.60.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `bastion` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.60.1 >/dev/null` |
| tên máy đúng | `bastion` | = bastion | `hostname` |
| ops-1 đang chạy | `host` | = RUNNING | `incus list ops-1 -c s -f csv` |
| ops-1 RAM cấp theo profile | `host` | = 4GiB | `incus config get ops-1 limits.memory` |
| ops-1 số vCPU theo profile | `host` | = 2 | `incus config get ops-1 limits.cpu` |
| cloud-init xong | `ops-1` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z6 | `ops-1` | = 10.10.60.12/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `ops-1` | = 10.10.60.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `ops-1` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.60.1 >/dev/null` |
| tên máy đúng | `ops-1` | = ops-1 | `hostname` |

**Hoàn tác:** incus delete -f <vm>

### P2.2 Boot Z5 Secret (bao-1)

**Vì sao:** Vùng nhạy cảm nhất; chỉ một VM.

**Phụ thuộc:** P2.1  ·  **Thời gian dự kiến:** 2 phút

```
$ labs/bin/lab tf vm bao-1
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| bao-1 đang chạy | `host` | = RUNNING | `incus list bao-1 -c s -f csv` |
| bao-1 RAM cấp theo profile | `host` | = 1GiB | `incus config get bao-1 limits.memory` |
| bao-1 số vCPU theo profile | `host` | = 1 | `incus config get bao-1 limits.cpu` |
| cloud-init xong | `bao-1` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z5 | `bao-1` | = 10.10.50.11/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `bao-1` | = 10.10.50.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `bao-1` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.50.1 >/dev/null` |
| tên máy đúng | `bao-1` | = bao-1 | `hostname` |

**Hoàn tác:** incus delete -f bao-1

### P2.3 Boot Z4 Dữ liệu (data-1, ch-1, kafka-1)

**Vì sao:** Đĩa phụ được tạo cùng VM (xem P6 để gắn).

**Phụ thuộc:** P2.2  ·  **Thời gian dự kiến:** 4 phút

```
$ labs/bin/lab tf vm data-1 ch-1 kafka-1
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| data-1 đang chạy | `host` | = RUNNING | `incus list data-1 -c s -f csv` |
| data-1 RAM cấp theo profile | `host` | = 5GiB | `incus config get data-1 limits.memory` |
| data-1 số vCPU theo profile | `host` | = 2 | `incus config get data-1 limits.cpu` |
| cloud-init xong | `data-1` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z4 | `data-1` | = 10.10.40.11/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `data-1` | = 10.10.40.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `data-1` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.40.1 >/dev/null` |
| tên máy đúng | `data-1` | = data-1 | `hostname` |
| ch-1 đang chạy | `host` | = RUNNING | `incus list ch-1 -c s -f csv` |
| ch-1 RAM cấp theo profile | `host` | = 4GiB | `incus config get ch-1 limits.memory` |
| ch-1 số vCPU theo profile | `host` | = 2 | `incus config get ch-1 limits.cpu` |
| cloud-init xong | `ch-1` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z4 | `ch-1` | = 10.10.40.12/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `ch-1` | = 10.10.40.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `ch-1` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.40.1 >/dev/null` |
| tên máy đúng | `ch-1` | = ch-1 | `hostname` |
| kafka-1 đang chạy | `host` | = RUNNING | `incus list kafka-1 -c s -f csv` |
| kafka-1 RAM cấp theo profile | `host` | = 3GiB | `incus config get kafka-1 limits.memory` |
| kafka-1 số vCPU theo profile | `host` | = 2 | `incus config get kafka-1 limits.cpu` |
| cloud-init xong | `kafka-1` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z4 | `kafka-1` | = 10.10.40.13/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `kafka-1` | = 10.10.40.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `kafka-1` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.40.1 >/dev/null` |
| tên máy đúng | `kafka-1` | = kafka-1 | `hostname` |
| Thấy đĩa phụ data và wal | `data-1` | ≥ 2 | `ls /dev/disk/by-id \| grep -cE 'incus_(data\|wal)'` |
| Thấy đĩa phụ data | `ch-1` | ≥ 1 | `ls /dev/disk/by-id \| grep -c 'incus_data'` |
| Thấy đĩa phụ data | `kafka-1` | ≥ 1 | `ls /dev/disk/by-id \| grep -c 'incus_data'` |

**Hoàn tác:** incus delete -f <vm>; incus storage volume delete default <vm>-<đĩa>

### P2.4 Boot Z3 Ứng dụng (k8s-cp-1, k8s-wk-1, k8s-wk-2)

**Vì sao:** Ba node cho K8s; chỉ boot, chưa cài K8s.

**Phụ thuộc:** P2.3  ·  **Thời gian dự kiến:** 5 phút

```
$ labs/bin/lab tf vm k8s-cp-1 k8s-wk-1 k8s-wk-2
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| k8s-cp-1 đang chạy | `host` | = RUNNING | `incus list k8s-cp-1 -c s -f csv` |
| k8s-cp-1 RAM cấp theo profile | `host` | = 3GiB | `incus config get k8s-cp-1 limits.memory` |
| k8s-cp-1 số vCPU theo profile | `host` | = 2 | `incus config get k8s-cp-1 limits.cpu` |
| cloud-init xong | `k8s-cp-1` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z3 | `k8s-cp-1` | = 10.10.30.11/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `k8s-cp-1` | = 10.10.30.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `k8s-cp-1` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.30.1 >/dev/null` |
| tên máy đúng | `k8s-cp-1` | = k8s-cp-1 | `hostname` |
| k8s-wk-1 đang chạy | `host` | = RUNNING | `incus list k8s-wk-1 -c s -f csv` |
| k8s-wk-1 RAM cấp theo profile | `host` | = 6GiB | `incus config get k8s-wk-1 limits.memory` |
| k8s-wk-1 số vCPU theo profile | `host` | = 3 | `incus config get k8s-wk-1 limits.cpu` |
| cloud-init xong | `k8s-wk-1` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z3 | `k8s-wk-1` | = 10.10.30.21/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `k8s-wk-1` | = 10.10.30.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `k8s-wk-1` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.30.1 >/dev/null` |
| tên máy đúng | `k8s-wk-1` | = k8s-wk-1 | `hostname` |
| k8s-wk-2 đang chạy | `host` | = RUNNING | `incus list k8s-wk-2 -c s -f csv` |
| k8s-wk-2 RAM cấp theo profile | `host` | = 6GiB | `incus config get k8s-wk-2 limits.memory` |
| k8s-wk-2 số vCPU theo profile | `host` | = 3 | `incus config get k8s-wk-2 limits.cpu` |
| cloud-init xong | `k8s-wk-2` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z3 | `k8s-wk-2` | = 10.10.30.22/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `k8s-wk-2` | = 10.10.30.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `k8s-wk-2` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.30.1 >/dev/null` |
| tên máy đúng | `k8s-wk-2` | = k8s-wk-2 | `hostname` |

**Hoàn tác:** incus delete -f <vm>

### P2.5 Boot Z2 Biên (perimeter-1)

**Vì sao:** Một VM gộp ingress-1, egw-1 và egress-1.

**Phụ thuộc:** P2.4  ·  **Thời gian dự kiến:** 2 phút

```
$ labs/bin/lab tf vm perimeter-1
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| perimeter-1 đang chạy | `host` | = RUNNING | `incus list perimeter-1 -c s -f csv` |
| perimeter-1 RAM cấp theo profile | `host` | = 2GiB | `incus config get perimeter-1 limits.memory` |
| perimeter-1 số vCPU theo profile | `host` | = 2 | `incus config get perimeter-1 limits.cpu` |
| cloud-init xong | `perimeter-1` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z2 | `perimeter-1` | = 10.10.20.11/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `perimeter-1` | = 10.10.20.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `perimeter-1` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.20.1 >/dev/null` |
| tên máy đúng | `perimeter-1` | = perimeter-1 | `hostname` |

**Hoàn tác:** incus delete -f perimeter-1

### P2.6 Boot Z1 Edge DMZ (edge-1, dns-1)

**Vì sao:** Vùng tin thấp nhất trong số các vùng nội bộ.

**Phụ thuộc:** P2.5  ·  **Thời gian dự kiến:** 3 phút

```
$ labs/bin/lab tf vm edge-1 dns-1
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| edge-1 đang chạy | `host` | = RUNNING | `incus list edge-1 -c s -f csv` |
| edge-1 RAM cấp theo profile | `host` | = 3GiB | `incus config get edge-1 limits.memory` |
| edge-1 số vCPU theo profile | `host` | = 2 | `incus config get edge-1 limits.cpu` |
| cloud-init xong | `edge-1` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z1 | `edge-1` | = 10.10.10.11/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `edge-1` | = 10.10.10.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `edge-1` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.10.1 >/dev/null` |
| tên máy đúng | `edge-1` | = edge-1 | `hostname` |
| dns-1 đang chạy | `host` | = RUNNING | `incus list dns-1 -c s -f csv` |
| dns-1 RAM cấp theo profile | `host` | = 512MiB | `incus config get dns-1 limits.memory` |
| dns-1 số vCPU theo profile | `host` | = 1 | `incus config get dns-1 limits.cpu` |
| cloud-init xong | `dns-1` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z1 | `dns-1` | = 10.10.10.12/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `dns-1` | = 10.10.10.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `dns-1` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.10.1 >/dev/null` |
| tên máy đúng | `dns-1` | = dns-1 | `hostname` |
| Thấy đĩa cache | `edge-1` | ≥ 1 | `ls /dev/disk/by-id \| grep -c 'incus_cache'` |

**Hoàn tác:** incus delete -f <vm>

### P2.7 Boot Z0 Internet giả (origin, loadgen, dns-mock)

**Vì sao:** Các VM đóng vai bên ngoài; chưa cài gì lên chúng.

**Phụ thuộc:** P2.6  ·  **Thời gian dự kiến:** 3 phút

```
$ labs/bin/lab tf vm origin loadgen dns-mock
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| origin đang chạy | `host` | = RUNNING | `incus list origin -c s -f csv` |
| origin RAM cấp theo profile | `host` | = 512MiB | `incus config get origin limits.memory` |
| origin số vCPU theo profile | `host` | = 1 | `incus config get origin limits.cpu` |
| cloud-init xong | `origin` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z0 | `origin` | = 10.10.0.11/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `origin` | = 10.10.0.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `origin` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.0.1 >/dev/null` |
| tên máy đúng | `origin` | = origin | `hostname` |
| loadgen đang chạy | `host` | = RUNNING | `incus list loadgen -c s -f csv` |
| loadgen RAM cấp theo profile | `host` | = 1GiB | `incus config get loadgen limits.memory` |
| loadgen số vCPU theo profile | `host` | = 2 | `incus config get loadgen limits.cpu` |
| cloud-init xong | `loadgen` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z0 | `loadgen` | = 10.10.0.12/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `loadgen` | = 10.10.0.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `loadgen` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.0.1 >/dev/null` |
| tên máy đúng | `loadgen` | = loadgen | `hostname` |
| dns-mock đang chạy | `host` | = RUNNING | `incus list dns-mock -c s -f csv` |
| dns-mock RAM cấp theo profile | `host` | = 512MiB | `incus config get dns-mock limits.memory` |
| dns-mock số vCPU theo profile | `host` | = 1 | `incus config get dns-mock limits.cpu` |
| cloud-init xong | `dns-mock` | = done | `cloud-init status \| awk '{print $2}'` |
| IP đúng vùng Z0 | `dns-mock` | = 10.10.0.13/24 | `ip -4 -o addr show lab0 \| awk '{print $4}'` |
| gateway là fw-1 | `dns-mock` | = 10.10.0.1 | `ip route \| awk '/^default/{print $3}'` |
| ping được fw-1 | `dns-mock` | lệnh thành công (rc=0) | `ping -c1 -W2 10.10.0.1 >/dev/null` |
| tên máy đúng | `dns-mock` | = dns-mock | `hostname` |

**Hoàn tác:** incus delete -f <vm>

## P3. Hardening hệ điều hành từng lớp

### P3.1 Gói bắt buộc và gỡ trình biên dịch

**Vì sao:** ADR-0048, "Dịch vụ": chỉ gói cần thiết, không trình biên dịch, không công cụ mạng cũ.

**Phụ thuộc:** P2.7  ·  **Thời gian dự kiến:** 6 phút (16 máy cài song song theo ansible forks)

```
$ labs/bin/lab provision --tags packages
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| auditd đã cài | `all` | = 1 | `dpkg -s auditd \| grep -c 'Status: install ok installed'` |
| aide đã cài | `all` | = 1 | `dpkg -s aide \| grep -c 'Status: install ok installed'` |
| chrony đã cài | `all` | = 1 | `dpkg -s chrony \| grep -c 'Status: install ok installed'` |
| nftables đã cài | `all` | = 1 | `dpkg -s nftables \| grep -c 'Status: install ok installed'` |
| Không có gcc | `all` | = 1 | `command -v gcc >/dev/null 2>&1; echo $?` |
| Không có make | `all` | = 1 | `command -v make >/dev/null 2>&1; echo $?` |
| Không có telnet | `all` | = 1 | `command -v telnet >/dev/null 2>&1; echo $?` |

**Hoàn tác:** Cài lại gói bằng apt nếu cần; bước idempotent.

### P3.2 Truy cập (SSH, người dùng, sudo)

**Vì sao:** Chỉ SSH cert, không mật khẩu, không root, nhóm ops, sudo có ghi log. Lab vẫn vào bằng incus exec nên không tự khoá.

**Phụ thuộc:** P3.1  ·  **Thời gian dự kiến:** 2 phút

```
$ labs/bin/lab provision --tags access
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Cấm đăng nhập root | `all` | = no | `sshd -T \| awk '/^permitrootlogin/{print $2}'` |
| Cấm mật khẩu | `all` | = no | `sshd -T \| awk '/^passwordauthentication/{print $2}'` |
| Chỉ nhóm ops được SSH | `all` | = ops | `sshd -T \| awk '/^allowgroups/{print $2}'` |
| Tối đa 3 lần thử | `all` | = 3 | `sshd -T \| awk '/^maxauthtries/{print $2}'` |
| Hết hạn đăng nhập 30 giây | `all` | = 30 | `sshd -T \| awk '/^logingracetime/{print $2}'` |
| Ngắt phiên rảnh 300 giây | `all` | = 300 | `sshd -T \| awk '/^clientaliveinterval/{print $2}'` |
| Không X11 | `all` | = no | `sshd -T \| awk '/^x11forwarding/{print $2}'` |
| Cấm xác thực theo máy (hostbased) | `all` | = no | `sshd -T \| awk '/^hostbasedauthentication/{print $2}'` |
| Bỏ qua rhosts | `all` | = yes | `sshd -T \| awk '/^ignorerhosts/{print $2}'` |
| Cấm biến môi trường do người dùng đặt | `all` | = no | `sshd -T \| awk '/^permituserenvironment/{print $2}'` |
| Cấm tunnel | `all` | = false | `sshd -T \| awk '/^permittunnel/{print $2}'` |
| Không GSSAPI | `all` | = no | `sshd -T \| awk '/^gssapiauthentication/{print $2}'` |
| Giới hạn kết nối chưa xác thực | `all` | = 10:30:60 | `sshd -T \| awk '/^maxstartups/{print $2}'` |
| Chỉ mã hoá hiện đại (không có cbc) | `all` | = 0 | `sshd -T \| awk '/^ciphers/{print $2}' \| grep -c cbc` |
| Không chuyển tiếp TCP | `all-except:bastion` | = no | `sshd -T \| awk '/^allowtcpforwarding/{print $2}'` |
| Bastion cho chuyển tiếp TCP (ProxyJump) | `bastion` | = yes | `sshd -T \| awk '/^allowtcpforwarding/{print $2}'` |
| Có CA SSH tin cậy | `all` | = 1 | `sshd -T \| grep -c '^trustedusercakeys'` |
| Mật khẩu root bị khoá | `all` | = L | `passwd -S root \| awk '{print $2}'` |
| sudoers của ops quyền 440 | `all` | = 440 | `stat -c %a /etc/sudoers.d/90-waap-ops` |
| sudo ghi log | `all` | = 1 | `grep -c 'logfile="/var/log/sudo.log"' /etc/sudoers.d/90-waap-ops` |

**Hoàn tác:** Xoá /etc/ssh/sshd_config.d/10-waap-hardening.conf rồi systemctl restart ssh (qua incus exec).

### P3.3 Hệ tệp và module kernel

**Vì sao:** /tmp và /dev/shm không thực thi được; chặn module không dùng (USB, FireWire, hệ tệp lạ, giao thức hiếm).

**Phụ thuộc:** P3.2  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab provision --tags filesystem
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| /tmp noexec,nosuid,nodev | `all` | chứa noexec, nosuid, nodev | `findmnt -no OPTIONS /tmp` |
| /tmp là tmpfs | `all` | = tmpfs | `findmnt -no FSTYPE /tmp` |
| /dev/shm noexec,nosuid,nodev | `all` | chứa noexec, nosuid, nodev | `findmnt -no OPTIONS /dev/shm` |
| Chặn usb-storage | `all` | chứa /bin/false | `modprobe -n -v usb-storage` |
| Chặn firewire-core | `all` | chứa /bin/false | `modprobe -n -v firewire-core` |
| Chặn cramfs | `all` | chứa /bin/false | `modprobe -n -v cramfs` |
| Chặn sctp | `all` | chứa /bin/false | `modprobe -n -v sctp` |

**Hoàn tác:** Sửa /etc/fstab và /etc/modprobe.d/90-waap-blacklist.conf.

### P3.4 Dịch vụ hệ thống

**Vì sao:** Tắt dịch vụ không dùng; bật AppArmor, auditd.

**Phụ thuộc:** P3.3  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab provision --tags services
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| AppArmor chạy | `all` | = active | `systemctl is-active apparmor` |
| avahi không chạy | `all` | khớp /^(inactive\|unknown\|failed)/ | `systemctl is-active avahi-daemon; true` |
| cups không chạy | `all` | khớp /^(inactive\|unknown\|failed)/ | `systemctl is-active cups; true` |
| rpcbind không chạy | `all` | khớp /^(inactive\|unknown\|failed)/ | `systemctl is-active rpcbind; true` |
| Không có profile AppArmor ở chế độ complain | `all` | = 0 | `aa-status --complaining` |

**Hoàn tác:** Bật lại bằng systemctl unmask.

### P3.5 Sysctl chuẩn (mạng và kernel)

**Vì sao:** Chống giả mạo IP, tắt redirect, ASLR, che con trỏ kernel; tắt IPv6 ở lab. Chuyển tiếp IP chỉ bật ở fw-1 và node K8s.

**Phụ thuộc:** P3.4  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab provision --tags kernel
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| rp_filter | `all` | = 1 | `sysctl -n net.ipv4.conf.all.rp_filter` |
| Không nhận source route | `all` | = 0 | `sysctl -n net.ipv4.conf.all.accept_source_route` |
| Không nhận redirect | `all` | = 0 | `sysctl -n net.ipv4.conf.all.accept_redirects` |
| Không gửi redirect | `all` | = 0 | `sysctl -n net.ipv4.conf.all.send_redirects` |
| Ghi log gói lạ | `all` | = 1 | `sysctl -n net.ipv4.conf.all.log_martians` |
| SYN cookies | `all` | = 1 | `sysctl -n net.ipv4.tcp_syncookies` |
| Tắt IPv6 | `all` | = 1 | `sysctl -n net.ipv6.conf.all.disable_ipv6` |
| ASLR đầy đủ | `all` | = 2 | `sysctl -n kernel.randomize_va_space` |
| Che con trỏ kernel | `all` | = 2 | `sysctl -n kernel.kptr_restrict` |
| Không nhận redirect an toàn (default) | `all` | = 0 | `sysctl -n net.ipv4.conf.default.secure_redirects` |
| Ghi log gói lạ (default) | `all` | = 1 | `sysctl -n net.ipv4.conf.default.log_martians` |
| Chống TIME-WAIT assassination (RFC 1337) | `all` | = 1 | `sysctl -n net.ipv4.tcp_rfc1337` |
| Tắt SysRq | `all` | = 0 | `sysctl -n kernel.sysrq` |
| perf_event chỉ root | `all` | = 3 | `sysctl -n kernel.perf_event_paranoid` |
| Cấm nạp kernel mới bằng kexec | `all` | = 1 | `sysctl -n kernel.kexec_load_disabled` |
| Hạn chế dmesg | `all` | = 1 | `sysctl -n kernel.dmesg_restrict` |
| ptrace chỉ cha con | `all` | = 1 | `sysctl -n kernel.yama.ptrace_scope` |
| bpf không đặc quyền tắt (Ubuntu mặc định 2 | `all` | = 2 | `sysctl -n kernel.unprivileged_bpf_disabled` |
| Không core dump suid | `all` | = 0 | `sysctl -n fs.suid_dumpable` |
| Không định tuyến IP | `all-except:firewall,k8s-cp,k8s-app` | = 0 | `sysctl -n net.ipv4.ip_forward` |
| fw-1 định tuyến IP | `fw-1` | = 1 | `sysctl -n net.ipv4.ip_forward` |
| Node K8s định tuyến IP | `role:k8s-cp` | = 1 | `sysctl -n net.ipv4.ip_forward` |
| Node K8s định tuyến IP | `role:k8s-app` | = 1 | `sysctl -n net.ipv4.ip_forward` |

**Hoàn tác:** Xoá /etc/sysctl.d/99-waap-hardening.conf rồi sysctl --system.

### P3.6 auditd

**Vì sao:** Ghi nhận thay đổi danh tính, sudo, SSH, giờ hệ thống, module kernel, mount, xoá tệp.

**Phụ thuộc:** P3.5  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab provision --tags audit
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| auditd chạy | `all` | = active | `systemctl is-active auditd` |
| Audit bật | `all` | = 1 | `auditctl -s \| awk '/^enabled/{print $2}'` |
| Số luật đã nạp | `all` | ≥ 20 | `auditctl -l \| wc -l` |
| Có luật identity (passwd | `all` | ≥ 4 | `auditctl -l \| grep -c 'k identity'` |
| Có luật sudoers | `all` | ≥ 2 | `auditctl -l \| grep -c 'k sudoers'` |
| Có luật sshd | `all` | ≥ 3 | `auditctl -l \| grep -c 'k sshd'` |
| Có luật đổi giờ | `all` | ≥ 1 | `auditctl -l \| grep -c 'k time-change'` |
| Có luật system-locale (hostname | `all` | ≥ 3 | `auditctl -l \| grep -c 'k system-locale'` |
| Có luật PAM | `all` | ≥ 3 | `auditctl -l \| grep -c 'k pam'` |
| Có luật phiên đăng nhập (wtmp | `all` | ≥ 3 | `auditctl -l \| grep -c 'k session'` |
| Có luật chính sách AppArmor | `all` | ≥ 2 | `auditctl -l \| grep -c 'k mac-policy'` |
| Có luật giả danh người dùng (execve euid khác uid) | `all` | ≥ 1 | `auditctl -l \| grep -c 'k user_emulation'` |
| Log audit tối đa 50 MB mỗi tệp | `all` | = 50 | `grep -E '^max_log_file ' /etc/audit/auditd.conf \| awk '{print $3}'` |
| Giữ 10 tệp log | `all` | = 10 | `grep -E '^num_logs ' /etc/audit/auditd.conf \| awk '{print $3}'` |
| Xoay vòng log | `all` | = rotate | `grep -E '^max_log_file_action ' /etc/audit/auditd.conf \| awk '{print $3}'` |
| Đầy đĩa thì tạm dừng (không làm sập máy) | `all` | = suspend | `grep -E '^disk_full_action ' /etc/audit/auditd.conf \| awk '{print $3}'` |
| Có luật nạp module kernel | `all` | ≥ 1 | `auditctl -l \| grep -c 'k modules'` |

**Hoàn tác:** Xoá /etc/audit/rules.d/90-waap.rules rồi augenrules --load.

### P3.7 AIDE (toàn vẹn tệp hệ thống)

**Vì sao:** Cơ sở dữ liệu băm ban đầu; thay đổi ngoài Ansible bị báo.

**Phụ thuộc:** P3.6  ·  **Thời gian dự kiến:** 15 phút (aideinit quét cả hệ tệp)

```
$ labs/bin/lab provision --tags aide
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Cơ sở dữ liệu AIDE có nội dung | `all-except:firewall` | lệnh thành công (rc=0) | `test -s /var/lib/aide/aide.db` |
| Kiểm AIDE hằng ngày bật | `all-except:firewall` | = enabled | `systemctl is-enabled dailyaidecheck.timer` |
| Ngay sau khởi tạo AIDE không báo thay đổi (rc 0) | `all-except:firewall` | lệnh thành công (rc=0) | `aide --check >/dev/null 2>&1` |

**Hoàn tác:** rm /var/lib/aide/aide.db rồi chạy lại bước.

### P3.8 Đồng bộ giờ và vá tự động

**Vì sao:** Giờ chuẩn cho cert 24 giờ và log; vá bảo mật tự động, khởi động lại 03:30.

**Phụ thuộc:** P3.7  ·  **Thời gian dự kiến:** 2 phút

```
$ labs/bin/lab provision --tags updates
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| chrony chạy | `all` | = active | `systemctl is-active chrony` |
| Có ít nhất một nguồn giờ | `all` | ≥ 1 | `chronyc sources \| grep -c '^\^'` |
| Dùng pool 4 nguồn (chrony khuyến nghị 3 nguồn trở lên; bootstrap) | `all` | = 1 | `grep -c '^pool ntp.ubuntu.com iburst maxsources 4' /etc/chrony/chrony.conf` |
| Không phục vụ giờ cho ai (cổng 0) | `all` | = 1 | `grep -c '^port 0' /etc/chrony/chrony.conf` |
| Vá tự động bật | `all` | = 1 | `grep -c 'Unattended-Upgrade "1"' /etc/apt/apt.conf.d/20auto-upgrades` |
| Chỉ vá bảo mật | `all` | ≥ 1 | `grep -c 'security' /etc/apt/apt.conf.d/52unattended-upgrades-waap` |
| Giờ khởi động lại | `all` | = 03:30 | `grep -o '03:30' /etc/apt/apt.conf.d/52unattended-upgrades-waap` |
| Không khởi động lại khi có người đăng nhập | `all` | = 1 | `grep -c 'Automatic-Reboot-WithUsers "false"' /etc/apt/apt.conf.d/52unattended-upgrades-waap` |

**Hoàn tác:** Xoá các tệp apt.conf.d/20auto-upgrades và 52unattended-upgrades-waap.

## P4. Firewall máy chủ

### P4.1 Xem trước luật firewall máy chủ (không áp dụng)

**Vì sao:** Luật input default-deny của từng máy sinh từ ma trận. Đọc trước để chắc không tự chặn đường quản trị.

**Phụ thuộc:** P3.8  ·  **Thời gian dự kiến:** 2 phút

```
$ labs/bin/lab provision --tags hostfw --check --diff
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Mọi luật máy chủ qua nft -c | `host` | lệnh thành công (rc=0) | `for f in labs/infra/generated/compact/hosts/*.nft; do nft -c -f $f \|\| exit 1; done` |
| Có luật SSH từ bastion (N25) ở mọi máy | `host` | = 1 | `grep -L 'N25' labs/infra/generated/compact/hosts/*.nft \| wc -l` |
| Đọc diff: không máy nào mất đường SSH từ bastion; luật chỉ mở cổng có dòng trong ma trận | `host` | THỦ CÔNG (đọc đầu ra của lệnh run) | |

**Hoàn tác:** Bước chỉ xem, không đổi gì.

### P4.2 Áp firewall máy chủ

**Vì sao:** Input default-deny trên từng VM, cộng egress khoá cho vai trò nhạy cảm (kho dữ liệu, OpenBao, WAF) khi seal.

**Phụ thuộc:** P4.1  ·  **Thời gian dự kiến:** 2 phút

```
$ labs/bin/lab provision --tags hostfw
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| nftables chạy | `all-except:firewall` | = active | `systemctl is-active nftables` |
| Input policy drop | `all-except:firewall` | = 1 | `nft list chain inet host input \| grep -c 'policy drop'` |
| Luật nạp từ tệp sinh tự động | `all-except:firewall` | = 1 | `grep -c 'Sinh tự động' /etc/nftables.conf` |
| Node K8s chỉ mở cổng RKE2 cần thiết (6443 | `role:k8s-cp` | = 1 | `nft list chain inet host input \| grep -c 'L02a'` |
| Worker không có luật mở 6443 và 9345 cho ai (chỉ server mới có) | `role:k8s-app` | = 0 | `nft list chain inet host input \| grep -c 'L02a'` |
| Kubelet 10250 và Canal 9099 chỉ trong vùng Z3 | `role:k8s-cp` | ≥ 1 | `nft list chain inet host input \| grep 'L02b' \| grep -c '10.10.30.'` |
| VXLAN udp 8472 chỉ trong vùng Z3 | `role:k8s-cp` | ≥ 1 | `nft list chain inet host input \| grep 'L02c' \| grep -c '10.10.30.'` |
| Chấp nhận lưu lượng từ giao diện mạng pod (cni0 | `role:k8s-app` | = 1 | `nft list chain inet host input \| grep -c 'pod network'` |
| Chấp nhận lưu lượng từ giao diện mạng pod (cni0 | `role:k8s-cp` | = 1 | `nft list chain inet host input \| grep -c 'pod network'` |
| Bastion vẫn SSH được data-1 (N25) | `bastion` | lệnh thành công (rc=0) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.40.11/22'` |

**Hoàn tác:** incus exec <vm> -- nft flush ruleset (tạm), rồi sửa flows.yaml và chạy lại.

### P4.3 Thăm dò mạng sinh từ ma trận

**Vì sao:** Luồng cho phép phải đến được, luồng cấm phải hết thời gian. Kiểm hai chiều bằng chạy thật (T01 đến T07).

**Phụ thuộc:** P4.2  ·  **Thời gian dự kiến:** 3 phút

```
$ labs/bin/lab nettest
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Mọi thăm dò đúng kỳ vọng | `host` | lệnh thành công (rc=0) | `labs/bin/lab nettest` |
| Edge không tới được data-1 5432 (X01) | `edge-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.40.11/5432'` |
| Edge không tới được bao-1 8200 (X01) | `edge-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.50.11/8200'` |
| Internet giả không tới được data-1 (X02) | `loadgen` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.40.11/5432'` |
| OpenBao không gọi ra được vùng khác (X05) | `bao-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.40.11/5432'` |

**Hoàn tác:** Không đổi gì.

## P5. Tối ưu hệ điều hành theo dịch vụ

### P5.1 Tuning cho data-1 (PostgreSQL, RabbitMQ, Redis trên cùng máy)

**Vì sao:** Ba dịch vụ dùng chung kernel nên giá trị được gộp; xung đột ghi ở cuối mục này. Hugepage cấp sẵn cho shared_buffers.

**Phụ thuộc:** P4.3  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab provision --tags tuning --limit data-1
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| sysctl net.core.somaxconn | `data-1` | = 1024 | `sysctl -n net.core.somaxconn \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.tcp_keepalive_intvl | `data-1` | = 10 | `sysctl -n net.ipv4.tcp_keepalive_intvl \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.tcp_keepalive_probes | `data-1` | = 3 | `sysctl -n net.ipv4.tcp_keepalive_probes \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.tcp_keepalive_time | `data-1` | = 60 | `sysctl -n net.ipv4.tcp_keepalive_time \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.dirty_background_bytes | `data-1` | = 67108864 | `sysctl -n vm.dirty_background_bytes \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.dirty_bytes | `data-1` | = 536870912 | `sysctl -n vm.dirty_bytes \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.nr_hugepages | `data-1` | = 300 | `sysctl -n vm.nr_hugepages \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.overcommit_memory | `data-1` | = 1 | `sysctl -n vm.overcommit_memory \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.swappiness | `data-1` | = 1 | `sysctl -n vm.swappiness \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| transparent hugepage enabled | `data-1` | = [never] | `grep -o '\[[a-z]*\]' /sys/kernel/mm/transparent_hugepage/enabled` |
| transparent hugepage defrag | `data-1` | = [never] | `grep -o '\[[a-z]*\]' /sys/kernel/mm/transparent_hugepage/defrag` |
| hugepage thực sự cấp được | `data-1` | = 300 | `awk '/HugePages_Total/{print $2}' /proc/meminfo` |

**Vì sao các giá trị này:**
- `data-1` (role_rabbitmq): Keepalive 60 giây để phát hiện kết nối chết sớm; nhiều kết nối nên cần nhiều file descriptor.
- `data-1` (role_valkey_central): Valkey (và Redis) cần overcommit=1 để fork bgsave không thất bại; somaxconn >= tcp-backlog; THP làm trễ đuôi.
- `data-1` (role_postgres): swappiness thấp để không đẩy shared_buffers ra swap; dirty_* giới hạn để checkpoint không dồn cục; hugepages = shared_buffers 512MB / 2MB = 256, dư khoảng 15%.

**Hoàn tác:** Xoá /etc/sysctl.d/99-zz-waap-tuning.conf và waap-thp-off.service rồi sysctl --system.

### P5.2 Tuning cho ch-1 (ClickHouse)

**Vì sao:** max_map_count cho mmap, THP tắt, swappiness thấp.

**Phụ thuộc:** P5.1  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab provision --tags tuning --limit ch-1
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| sysctl vm.max_map_count | `ch-1` | = 262144 | `sysctl -n vm.max_map_count \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.swappiness | `ch-1` | = 1 | `sysctl -n vm.swappiness \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| transparent hugepage enabled | `ch-1` | = [never] | `grep -o '\[[a-z]*\]' /sys/kernel/mm/transparent_hugepage/enabled` |
| transparent hugepage defrag | `ch-1` | = [never] | `grep -o '\[[a-z]*\]' /sys/kernel/mm/transparent_hugepage/defrag` |

**Vì sao các giá trị này:**
- `ch-1` (role_clickhouse): ClickHouse mmap nhiều: max_map_count 262144; THP tắt; nhiều file mở cho các part.

**Hoàn tác:** Như P5.1.

### P5.3 Tuning cho edge-1 (mạng, hàng đợi, cổng nguồn)

**Vì sao:** Hàng đợi accept và SYN đủ lớn cho tải đột biến; dải cổng nguồn rộng để proxy tới origin.

**Phụ thuộc:** P5.2  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab provision --tags tuning --limit edge-1
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| sysctl fs.file-max | `edge-1` | = 2097152 | `sysctl -n fs.file-max \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.core.default_qdisc | `edge-1` | = fq | `sysctl -n net.core.default_qdisc \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.core.netdev_max_backlog | `edge-1` | = 16384 | `sysctl -n net.core.netdev_max_backlog \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.core.rmem_max | `edge-1` | = 16777216 | `sysctl -n net.core.rmem_max \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.core.somaxconn | `edge-1` | = 1024 | `sysctl -n net.core.somaxconn \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.core.wmem_max | `edge-1` | = 16777216 | `sysctl -n net.core.wmem_max \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.ip_local_port_range | `edge-1` | = 10240 65535 | `sysctl -n net.ipv4.ip_local_port_range \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.tcp_congestion_control | `edge-1` | = bbr | `sysctl -n net.ipv4.tcp_congestion_control \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.tcp_fin_timeout | `edge-1` | = 15 | `sysctl -n net.ipv4.tcp_fin_timeout \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.tcp_max_syn_backlog | `edge-1` | = 16384 | `sysctl -n net.ipv4.tcp_max_syn_backlog \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.tcp_rmem | `edge-1` | = 4096 87380 16777216 | `sysctl -n net.ipv4.tcp_rmem \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.tcp_slow_start_after_idle | `edge-1` | = 0 | `sysctl -n net.ipv4.tcp_slow_start_after_idle \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.tcp_tw_reuse | `edge-1` | = 1 | `sysctl -n net.ipv4.tcp_tw_reuse \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.tcp_wmem | `edge-1` | = 4096 65536 16777216 | `sysctl -n net.ipv4.tcp_wmem \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.overcommit_memory | `edge-1` | = 1 | `sysctl -n vm.overcommit_memory \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.swappiness | `edge-1` | = 1 | `sysctl -n vm.swappiness \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| transparent hugepage enabled | `edge-1` | = [never] | `grep -o '\[[a-z]*\]' /sys/kernel/mm/transparent_hugepage/enabled` |
| transparent hugepage defrag | `edge-1` | = [never] | `grep -o '\[[a-z]*\]' /sys/kernel/mm/transparent_hugepage/defrag` |
| module kernel tcp_bbr đã nạp | `edge-1` | = 1 | `lsmod \| awk '$1=="tcp_bbr"' \| wc -l` |

**Vì sao các giá trị này:**
- `edge-1` (role_edge_l7): Hàng đợi accept và SYN đủ lớn cho tải đột biến; dải cổng nguồn rộng để proxy tới origin; không chậm lại sau khi nhàn rỗi.
- `edge-1` (role_edge_waf): WAF tách CPU và RAM qua slice systemd (profile_edge_waf), không đổi sysctl riêng.
- `edge-1` (role_valkey_edge): Valkey (và Redis) cần overcommit=1 để fork bgsave không thất bại; somaxconn >= tcp-backlog; THP làm trễ đuôi.

**Xung đột giá trị khi gộp vai trò (vai trò sau thắng, xem `tuning_precedence`):**
- `edge-1`: net.core.somaxconn (đã chọn 65535 → 1024 theo vai trò role_valkey_edge)

**Hoàn tác:** Như P5.1.

### P5.4 Tuning cho bao-1 (OpenBao)

**Vì sao:** Không core dump vì bộ nhớ chứa khoá.

**Phụ thuộc:** P5.3  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab provision --tags tuning --limit bao-1
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| sysctl kernel.core_pattern | `bao-1` | = \|/bin/false | `sysctl -n kernel.core_pattern \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.yama.ptrace_scope | `bao-1` | = 2 | `sysctl -n kernel.yama.ptrace_scope \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |

**Vì sao các giá trị này:**
- `bao-1` (role_openbao): Không core dump: bộ nhớ chứa khoá. Không swap (profile_secret).

**Hoàn tác:** Như P5.1.

### P5.5 Tuning cho node K8s (kubelet protect-kernel-defaults)

**Vì sao:** kubelet từ chối chạy nếu sysctl lệch giá trị nó mong đợi; inotify và conntrack đủ cho nhiều pod.

**Phụ thuộc:** P5.4  ·  **Thời gian dự kiến:** 2 phút

```
$ labs/bin/lab provision --tags tuning --limit k8s-cp-1,k8s-wk-1,k8s-wk-2
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| sysctl fs.inotify.max_user_instances | `k8s-cp-1` | = 8192 | `sysctl -n fs.inotify.max_user_instances \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl fs.inotify.max_user_watches | `k8s-cp-1` | = 524288 | `sysctl -n fs.inotify.max_user_watches \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.keys.root_maxbytes | `k8s-cp-1` | = 25000000 | `sysctl -n kernel.keys.root_maxbytes \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.keys.root_maxkeys | `k8s-cp-1` | = 1000000 | `sysctl -n kernel.keys.root_maxkeys \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.panic | `k8s-cp-1` | = 10 | `sysctl -n kernel.panic \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.panic_on_oops | `k8s-cp-1` | = 1 | `sysctl -n kernel.panic_on_oops \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.bridge.bridge-nf-call-ip6tables | `k8s-cp-1` | = 1 | `sysctl -n net.bridge.bridge-nf-call-ip6tables \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.bridge.bridge-nf-call-iptables | `k8s-cp-1` | = 1 | `sysctl -n net.bridge.bridge-nf-call-iptables \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.ip_forward | `k8s-cp-1` | = 1 | `sysctl -n net.ipv4.ip_forward \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.netfilter.nf_conntrack_max | `k8s-cp-1` | = 262144 | `sysctl -n net.netfilter.nf_conntrack_max \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.overcommit_memory | `k8s-cp-1` | = 1 | `sysctl -n vm.overcommit_memory \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.panic_on_oom | `k8s-cp-1` | = 0 | `sysctl -n vm.panic_on_oom \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| module kernel overlay đã nạp | `k8s-cp-1` | = 1 | `lsmod \| awk '$1=="overlay"' \| wc -l` |
| module kernel br_netfilter đã nạp | `k8s-cp-1` | = 1 | `lsmod \| awk '$1=="br_netfilter"' \| wc -l` |
| module kernel nf_conntrack đã nạp | `k8s-cp-1` | = 1 | `lsmod \| awk '$1=="nf_conntrack"' \| wc -l` |
| sysctl fs.inotify.max_user_instances | `k8s-wk-1` | = 8192 | `sysctl -n fs.inotify.max_user_instances \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl fs.inotify.max_user_watches | `k8s-wk-1` | = 524288 | `sysctl -n fs.inotify.max_user_watches \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.keys.root_maxbytes | `k8s-wk-1` | = 25000000 | `sysctl -n kernel.keys.root_maxbytes \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.keys.root_maxkeys | `k8s-wk-1` | = 1000000 | `sysctl -n kernel.keys.root_maxkeys \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.panic | `k8s-wk-1` | = 10 | `sysctl -n kernel.panic \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.panic_on_oops | `k8s-wk-1` | = 1 | `sysctl -n kernel.panic_on_oops \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.bridge.bridge-nf-call-ip6tables | `k8s-wk-1` | = 1 | `sysctl -n net.bridge.bridge-nf-call-ip6tables \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.bridge.bridge-nf-call-iptables | `k8s-wk-1` | = 1 | `sysctl -n net.bridge.bridge-nf-call-iptables \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.ip_forward | `k8s-wk-1` | = 1 | `sysctl -n net.ipv4.ip_forward \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.netfilter.nf_conntrack_max | `k8s-wk-1` | = 262144 | `sysctl -n net.netfilter.nf_conntrack_max \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.overcommit_memory | `k8s-wk-1` | = 1 | `sysctl -n vm.overcommit_memory \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.panic_on_oom | `k8s-wk-1` | = 0 | `sysctl -n vm.panic_on_oom \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| module kernel overlay đã nạp | `k8s-wk-1` | = 1 | `lsmod \| awk '$1=="overlay"' \| wc -l` |
| module kernel br_netfilter đã nạp | `k8s-wk-1` | = 1 | `lsmod \| awk '$1=="br_netfilter"' \| wc -l` |
| module kernel nf_conntrack đã nạp | `k8s-wk-1` | = 1 | `lsmod \| awk '$1=="nf_conntrack"' \| wc -l` |
| sysctl fs.inotify.max_user_instances | `k8s-wk-2` | = 8192 | `sysctl -n fs.inotify.max_user_instances \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl fs.inotify.max_user_watches | `k8s-wk-2` | = 524288 | `sysctl -n fs.inotify.max_user_watches \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.keys.root_maxbytes | `k8s-wk-2` | = 25000000 | `sysctl -n kernel.keys.root_maxbytes \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.keys.root_maxkeys | `k8s-wk-2` | = 1000000 | `sysctl -n kernel.keys.root_maxkeys \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.panic | `k8s-wk-2` | = 10 | `sysctl -n kernel.panic \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl kernel.panic_on_oops | `k8s-wk-2` | = 1 | `sysctl -n kernel.panic_on_oops \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.bridge.bridge-nf-call-ip6tables | `k8s-wk-2` | = 1 | `sysctl -n net.bridge.bridge-nf-call-ip6tables \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.bridge.bridge-nf-call-iptables | `k8s-wk-2` | = 1 | `sysctl -n net.bridge.bridge-nf-call-iptables \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.ipv4.ip_forward | `k8s-wk-2` | = 1 | `sysctl -n net.ipv4.ip_forward \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl net.netfilter.nf_conntrack_max | `k8s-wk-2` | = 262144 | `sysctl -n net.netfilter.nf_conntrack_max \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.overcommit_memory | `k8s-wk-2` | = 1 | `sysctl -n vm.overcommit_memory \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| sysctl vm.panic_on_oom | `k8s-wk-2` | = 0 | `sysctl -n vm.panic_on_oom \| tr -s '[:space:]' ' ' \| sed 's/ $//'` |
| module kernel overlay đã nạp | `k8s-wk-2` | = 1 | `lsmod \| awk '$1=="overlay"' \| wc -l` |
| module kernel br_netfilter đã nạp | `k8s-wk-2` | = 1 | `lsmod \| awk '$1=="br_netfilter"' \| wc -l` |
| module kernel nf_conntrack đã nạp | `k8s-wk-2` | = 1 | `lsmod \| awk '$1=="nf_conntrack"' \| wc -l` |

**Vì sao các giá trị này:**
- `k8s-cp-1` (role_k8s_cp): kubelet protect-kernel-defaults đòi đúng các giá trị này; inotify đủ cho nhiều pod; conntrack đủ cho kube-proxy.
- `k8s-wk-1` (role_k8s_app): kubelet protect-kernel-defaults đòi đúng các giá trị này; inotify đủ cho nhiều pod; conntrack đủ cho kube-proxy.
- `k8s-wk-2` (role_k8s_app): kubelet protect-kernel-defaults đòi đúng các giá trị này; inotify đủ cho nhiều pod; conntrack đủ cho kube-proxy.

**Hoàn tác:** Như P5.1.

### P5.6 Pin CPU và giới hạn RAM cho WAF trên edge-1

**Vì sao:** WAF không được tranh CPU với L7. edge-1 có 2 vCPU nên L7 ở nhân 0, WAF bị khoá ở nhân 1 bằng slice systemd.

**Phụ thuộc:** P5.5  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab provision --tags profile-edge --limit edge-1
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| WAF chỉ chạy ở nhân 1 | `edge-1` | = 1 | `systemctl show waap-waf.slice -p AllowedCPUs --value` |
| WAF tối đa 100% một nhân | `edge-1` | = 1s | `systemctl show waap-waf.slice -p CPUQuotaPerSecUSec --value` |
| WAF tối đa 1 GiB RAM (byte) | `edge-1` | = 1073741824 | `systemctl show waap-waf.slice -p MemoryMax --value` |
| L7 tối đa 1 GiB RAM (byte) | `edge-1` | = 1073741824 | `systemctl show waap-edge -p MemoryMax --value` |
| L7 chỉ có quyền gắn cổng thấp | `edge-1` | = cap_net_bind_service | `systemctl show waap-edge -p CapabilityBoundingSet --value` |
| WAF không có capability nào | `edge-1` | =  | `systemctl show waap-waf -p CapabilityBoundingSet --value` |
| L7 chạy bằng user riêng không đăng nhập | `edge-1` | = /usr/sbin/nologin | `getent passwd waap-edge \| cut -d: -f7` |
| Pin vCPU của VM vào nhân máy chủ (chỉ máy Linux riêng, đường B) | `edge-1` | THỦ CÔNG (incus config get edge-1 limits.cpu; đặt `pin` trong profile rồi tofu apply; ở đường A (Hyper-V) bỏ qua) | |

**Hoàn tác:** Xoá waap-waf.slice và drop-in rồi daemon-reload.

## P6. Đĩa

### P6.1 Gắn đĩa phụ (dữ liệu, WAL, audit, cache)

**Vì sao:** ADR-0048: đĩa riêng cho dữ liệu, WAL, audit, cache; noatime, nodev, nosuid, noexec cho dữ liệu.

**Phụ thuộc:** P5.6  ·  **Thời gian dự kiến:** 3 phút

```
$ labs/bin/lab provision --tags disks
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| đĩa data gắn ở /var/lib/waap/data, hệ tệp xfs | `data-1` | = xfs | `findmnt -no FSTYPE /var/lib/waap/data` |
| /var/lib/waap/data tuỳ chọn mount | `data-1` | chứa noatime, nodev, nosuid, noexec | `findmnt -no OPTIONS /var/lib/waap/data` |
| /var/lib/waap/data dung lượng >= 48 GB (xfs tốn một phần) | `data-1` | ≥ 48 | `df -BG --output=size /var/lib/waap/data \| tail -1 \| tr -dc 0-9` |
| đĩa wal gắn ở /var/lib/waap/wal, hệ tệp xfs | `data-1` | = xfs | `findmnt -no FSTYPE /var/lib/waap/wal` |
| /var/lib/waap/wal tuỳ chọn mount | `data-1` | chứa noatime, nodev, nosuid, noexec | `findmnt -no OPTIONS /var/lib/waap/wal` |
| /var/lib/waap/wal dung lượng >= 8 GB (xfs tốn một phần) | `data-1` | ≥ 8 | `df -BG --output=size /var/lib/waap/wal \| tail -1 \| tr -dc 0-9` |
| đĩa data gắn ở /var/lib/waap/data, hệ tệp xfs | `ch-1` | = xfs | `findmnt -no FSTYPE /var/lib/waap/data` |
| /var/lib/waap/data tuỳ chọn mount | `ch-1` | chứa noatime, nodev, nosuid, noexec | `findmnt -no OPTIONS /var/lib/waap/data` |
| /var/lib/waap/data dung lượng >= 48 GB (xfs tốn một phần) | `ch-1` | ≥ 48 | `df -BG --output=size /var/lib/waap/data \| tail -1 \| tr -dc 0-9` |
| đĩa data gắn ở /var/lib/waap/data, hệ tệp xfs | `kafka-1` | = xfs | `findmnt -no FSTYPE /var/lib/waap/data` |
| /var/lib/waap/data tuỳ chọn mount | `kafka-1` | chứa noatime, nodev, nosuid, noexec | `findmnt -no OPTIONS /var/lib/waap/data` |
| /var/lib/waap/data dung lượng >= 28 GB (xfs tốn một phần) | `kafka-1` | ≥ 28 | `df -BG --output=size /var/lib/waap/data \| tail -1 \| tr -dc 0-9` |
| đĩa audit gắn ở /var/log/openbao-audit, hệ tệp xfs | `bao-1` | = xfs | `findmnt -no FSTYPE /var/log/openbao-audit` |
| /var/log/openbao-audit tuỳ chọn mount | `bao-1` | chứa noatime, nodev, nosuid, noexec | `findmnt -no OPTIONS /var/log/openbao-audit` |
| /var/log/openbao-audit dung lượng >= 0 GB (xfs tốn một phần) | `bao-1` | ≥ 0 | `df -BG --output=size /var/log/openbao-audit \| tail -1 \| tr -dc 0-9` |
| đĩa cache gắn ở /var/cache/waap-edge, hệ tệp xfs | `edge-1` | = xfs | `findmnt -no FSTYPE /var/cache/waap-edge` |
| /var/cache/waap-edge tuỳ chọn mount | `edge-1` | chứa noatime, nodev, nosuid | `findmnt -no OPTIONS /var/cache/waap-edge` |
| /var/cache/waap-edge dung lượng >= 8 GB (xfs tốn một phần) | `edge-1` | ≥ 8 | `df -BG --output=size /var/cache/waap-edge \| tail -1 \| tr -dc 0-9` |

**Hoàn tác:** umount, bỏ dòng trong /etc/fstab. Không mkfs lại khi đã có dữ liệu.

## P7. Hồ sơ hệ điều hành theo vai trò

### P7.1 Hồ sơ data (tài khoản backup, thư mục gốc dữ liệu)

**Vì sao:** Tài khoản backup riêng không đăng nhập được; thư mục gốc chỉ root và nhóm backup.

**Phụ thuộc:** P6.1  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab provision --tags profile-data
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Tài khoản backup không đăng nhập | `role:postgres` | = /usr/sbin/nologin | `getent passwd waap-backup \| cut -d: -f7` |
| Thư mục gốc dữ liệu quyền 750 | `role:postgres` | = 750 | `stat -c %a /var/lib/waap` |
| Thư mục gốc dữ liệu quyền 750 | `role:clickhouse` | = 750 | `stat -c %a /var/lib/waap` |

**Hoàn tác:** Xoá user waap-backup.

### P7.2 Hồ sơ secret (OpenBao: không swap, không core dump, khoá bộ nhớ)

**Vì sao:** Bộ nhớ chứa khoá không được ra đĩa. CAP_IPC_LOCK và LimitMEMLOCK cho mlock.

**Phụ thuộc:** P7.1  ·  **Thời gian dự kiến:** 1 phút

```
$ labs/bin/lab provision --tags profile-secret
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Không có swap | `bao-1` | = 0 | `swapon --show \| wc -l` |
| Không có dòng swap hoạt động trong fstab | `bao-1` | = 0 | `grep -vc '^#' /etc/fstab \| head -1; grep -c '^[^#].*swap' /etc/fstab` |
| Core dump systemd tắt | `bao-1` | = 1 | `grep -c 'Storage=none' /etc/systemd/coredump.conf.d/10-disable.conf` |
| User openbao không đăng nhập | `bao-1` | = /usr/sbin/nologin | `getent passwd openbao \| cut -d: -f7` |
| Thư mục audit riêng quyền 750 | `bao-1` | = 750 | `stat -c %a /var/log/openbao-audit` |
| Thư mục audit thuộc openbao | `bao-1` | = openbao | `stat -c %U /var/log/openbao-audit` |
| Cho phép mlock | `bao-1` | = infinity | `systemctl show openbao -p LimitMEMLOCK --value` |
| Chỉ có CAP_IPC_LOCK | `bao-1` | = cap_ipc_lock | `systemctl show openbao -p CapabilityBoundingSet --value` |

**Hoàn tác:** Xoá drop-in openbao.service.d.

### P7.3 Hồ sơ bastion (WireGuard, ghi phiên, không giữ khoá riêng)

**Vì sao:** Cửa quản trị duy nhất; ghi phím gõ vào auditd; MFA TOTP mặc định tắt để không tự khoá.

**Phụ thuộc:** P7.2  ·  **Thời gian dự kiến:** 2 phút

```
$ labs/bin/lab provision --tags profile-bastion
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Khoá WireGuard sinh tại chỗ | `bastion` | = 600 | `stat -c %a /etc/wireguard/private.key` |
| Ghi phiên bằng pam_tty_audit | `bastion` | = 1 | `grep -c pam_tty_audit /etc/pam.d/sshd` |
| Không có khoá riêng của ai trong /home | `bastion` | = 0 | `find /home -name 'id_*' -o -name '*.pem' \| wc -l` |
| MFA TOTP (mặc định tắt trong lab; bật khi mọi người đã có TOTP) | `bastion` | THỦ CÔNG (đặt bastion_mfa=true trong group_vars/role_bastion.yml khi sẵn sàng) | |

**Hoàn tác:** Gỡ dòng pam_tty_audit khỏi /etc/pam.d/sshd.

## P8. Platform và database

### P8.1 RKE2 server (control-plane)

**Vì sao:** Platform K8s (RKE2, không dùng k3s). Cần ghim phiên bản và sha256 của tarball (bộ cài từ chối tải nhị phân chưa kiểm). Profile CIS bắt buộc có user etcd và sysctl đúng giá trị; mã hoá secret; tắt ingress mặc định.

**Phụ thuộc:** P7.3  ·  **Thời gian dự kiến:** 8 phút (kéo ảnh container lần đầu)

```
$ labs/bin/lab provision --tags k8s --limit k8s-cp-1
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| rke2-server chạy | `k8s-cp-1` | = active | `systemctl is-active rke2-server` |
| Node Ready | `k8s-cp-1` | = 1 | `env KUBECONFIG=/etc/rancher/rke2/rke2.yaml /var/lib/rancher/rke2/bin/kubectl get nodes --no-headers \| grep -c ' Ready'` |
| Profile CIS bật | `k8s-cp-1` | = 1 | `grep -c '^profile: cis' /etc/rancher/rke2/config.yaml` |
| User etcd không đăng nhập | `k8s-cp-1` | = /sbin/nologin | `getent passwd etcd \| cut -d: -f7` |
| Dữ liệu etcd thuộc user etcd | `k8s-cp-1` | = etcd | `stat -c %U /var/lib/rancher/rke2/server/db/etcd` |
| Cổng supervisor 9345 nghe | `k8s-cp-1` | ≥ 1 | `ss -ltn \| grep -c ':9345'` |
| Cổng API 6443 nghe | `k8s-cp-1` | ≥ 1 | `ss -ltn \| grep -c ':6443'` |
| Cổng chỉ đọc kubelet 10255 đóng | `k8s-cp-1` | = 0 | `ss -ltn \| grep -c ':10255'` |
| Mã hoá secret bật | `k8s-cp-1` | ≥ 1 | `/usr/local/bin/rke2 secrets-encrypt status \| grep -c 'Enabled'` |
| CNI canal đang chạy | `k8s-cp-1` | ≥ 1 | `env KUBECONFIG=/etc/rancher/rke2/rke2.yaml /var/lib/rancher/rke2/bin/kubectl -n kube-system get pods --no-headers \| grep -c canal` |
| Không có ingress controller mặc định (ingress do ingress-1 đảm nhiệm) | `k8s-cp-1` | = 0 | `env KUBECONFIG=/etc/rancher/rke2/rke2.yaml /var/lib/rancher/rke2/bin/kubectl -n kube-system get pods --no-headers \| grep -ciE 'ingress-nginx\|traefik'` |
| API server dùng cấu hình Pod Security (admission) | `k8s-cp-1` | ≥ 1 | `ps -eo args \| grep -c '[a]dmission-control-config-file'` |
| API server có audit log | `k8s-cp-1` | ≥ 1 | `ps -eo args \| grep -c '[a]udit-log-path'` |
| Có audit policy mặc định do profile CIS tạo | `k8s-cp-1` | lệnh thành công (rc=0) | `ls /etc/rancher/rke2/audit-policy.yaml` |
| Ngắt kết nối stream rảnh 5 phút | `k8s-cp-1` | = 1 | `grep -c 'streaming-connection-idle-timeout=5m' /etc/rancher/rke2/config.yaml` |
| Token cụm quyền 600 | `k8s-cp-1` | = 600 | `stat -c %a /etc/rancher/rke2/token` |
| Cấu hình RKE2 quyền 600 | `k8s-cp-1` | = 600 | `stat -c %a /etc/rancher/rke2/config.yaml` |
| kubeconfig quyền 600 | `k8s-cp-1` | = 600 | `stat -c %a /etc/rancher/rke2/rke2.yaml` |
| Không swap | `k8s-cp-1` | = 0 | `swapon --show \| wc -l` |

**Hoàn tác:** incus exec k8s-cp-1 -- /usr/local/bin/rke2-uninstall.sh (có trong tarball) hoặc dựng lại VM.

### P8.2 RKE2 agent (hai worker)

**Vì sao:** Hai worker gia nhập cụm qua cổng supervisor 9345.

**Phụ thuộc:** P8.1  ·  **Thời gian dự kiến:** 6 phút

```
$ labs/bin/lab provision --tags k8s --limit k8s-wk-1,k8s-wk-2
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Ba node Ready | `k8s-cp-1` | = 3 | `env KUBECONFIG=/etc/rancher/rke2/rke2.yaml /var/lib/rancher/rke2/bin/kubectl get nodes --no-headers \| grep -c ' Ready'` |
| Agent chạy | `k8s-wk-1` | = active | `systemctl is-active rke2-agent` |
| Agent chạy | `k8s-wk-2` | = active | `systemctl is-active rke2-agent` |
| Worker vào được supervisor 9345 của server | `k8s-wk-1` | lệnh thành công (rc=0) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.30.11/9345'` |
| Worker vào được API 6443 | `k8s-wk-1` | lệnh thành công (rc=0) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.30.11/6443'` |
| Worker KHÔNG vào được etcd 2379 của server | `k8s-wk-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.30.11/2379'` |
| Worker KHÔNG vào được etcd peer 2380 | `k8s-wk-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.30.11/2380'` |
| Worker KHÔNG SSH được vào server | `k8s-wk-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.30.11/22'` |
| Worker KHÔNG SSH được vào worker khác | `k8s-wk-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.30.22/22'` |
| Server vào được kubelet 10250 của worker | `k8s-wk-1` | lệnh thành công (rc=0) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.30.22/10250'` |
| Pod hệ thống đều Running (CNI và đường pod tới API không bị firewall máy chủ chặn) | `k8s-cp-1` | ≥ 5 | `env KUBECONFIG=/etc/rancher/rke2/rke2.yaml /var/lib/rancher/rke2/bin/kubectl -n kube-system get pods --no-headers \| grep -c ' Running'` |
| Worker kubelet không mở cổng chỉ đọc | `k8s-wk-1` | = 0 | `ss -ltn \| grep -c ':10255'` |

**Hoàn tác:** incus exec <worker> -- /usr/local/bin/rke2-uninstall.sh hoặc dựng lại VM.

### P8.3 PostgreSQL (repo PGDG chính thức)

**Vì sao:** Cài từ repo PGDG với khoá ký ghim fingerprint, không dùng gói Ubuntu; gói bị hold nên chỉ nâng khi đổi biến. Đĩa dữ liệu và WAL riêng, checksum, scram-sha-256, TLS, chỉ nhận từ vùng ứng dụng (Z3).

**Phụ thuộc:** P8.2  ·  **Thời gian dự kiến:** 3 phút

```
$ labs/bin/lab services --tags postgres
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Gói đến từ repo PGDG | `data-1` | ≥ 1 | `apt-cache policy postgresql-17 \| grep -c 'apt.postgresql.org'` |
| Khoá ký repo PGDG đúng fingerprint đã ghim | `data-1` | ≥ 1 | `gpg --batch --show-keys --with-colons /usr/share/keyrings/pgdg.gpg \| awk -F: '/^fpr:/{print $10}' \| grep -c B97B0AFCAA1A47F044F244A07FCC7D46ACCC4CF8` |
| Gói PostgreSQL bị hold (không tự nâng) | `data-1` | ≥ 1 | `apt-mark showhold \| grep -c 'postgresql-17'` |
| Ghi lại version đã cài (đọc bằng mắt) | `data-1` | THỦ CÔNG (sudo -u postgres psql -tAc 'select version()'; ghi version vào nhật ký) | |
| Sẵn sàng nhận kết nối | `data-1` | lệnh thành công (rc=0) | `pg_isready -q` |
| TLS bật | `data-1` | = on | `sudo -u postgres psql -tAc 'show ssl'` |
| Mã hoá mật khẩu | `data-1` | = scram-sha-256 | `sudo -u postgres psql -tAc 'show password_encryption'` |
| Thư mục dữ liệu trên đĩa riêng | `data-1` | = /var/lib/waap/data/pg | `sudo -u postgres psql -tAc 'show data_directory'` |
| WAL trên đĩa riêng | `data-1` | = /var/lib/waap/wal/pg | `readlink -f /var/lib/waap/data/pg/pg_wal` |
| Checksum dữ liệu bật | `data-1` | = on | `sudo -u postgres psql -tAc 'show data_checksums'` |
| Chỉ nghe IP lab và localhost | `data-1` | = 127.0.0.1,10.10.40.11 | `sudo -u postgres psql -tAc 'show listen_addresses'` |
| Số kết nối tối đa | `data-1` | = 200 | `sudo -u postgres psql -tAc 'show max_connections'` |
| shared_buffers | `data-1` | = 512MB | `sudo -u postgres psql -tAc 'show shared_buffers'` |
| TLS tối thiểu 1.2 | `data-1` | = TLSv1.2 | `sudo -u postgres psql -tAc 'show ssl_min_protocol_version'` |
| effective_cache_size | `data-1` | = 2GB | `sudo -u postgres psql -tAc 'show effective_cache_size'` |
| work_mem | `data-1` | = 4MB | `sudo -u postgres psql -tAc 'show work_mem'` |
| maintenance_work_mem | `data-1` | = 128MB | `sudo -u postgres psql -tAc 'show maintenance_work_mem'` |
| random_page_cost cho SSD | `data-1` | = 1.1 | `sudo -u postgres psql -tAc 'show random_page_cost'` |
| effective_io_concurrency cho SSD | `data-1` | = 200 | `sudo -u postgres psql -tAc 'show effective_io_concurrency'` |
| Nén WAL | `data-1` | = on | `sudo -u postgres psql -tAc 'show wal_compression'` |
| max_wal_size | `data-1` | = 2GB | `sudo -u postgres psql -tAc 'show max_wal_size'` |
| checkpoint_completion_target | `data-1` | = 0.9 | `sudo -u postgres psql -tAc 'show checkpoint_completion_target'` |
| Ghi log checkpoint | `data-1` | = on | `sudo -u postgres psql -tAc 'show log_checkpoints'` |
| Ghi log chờ khoá | `data-1` | = on | `sudo -u postgres psql -tAc 'show log_lock_waits'` |
| Đo thời gian I/O | `data-1` | = on | `sudo -u postgres psql -tAc 'show track_io_timing'` |
| Ngắt giao dịch treo 1 phút | `data-1` | = 1min | `sudo -u postgres psql -tAc 'show idle_in_transaction_session_timeout'` |
| Keepalive TCP 60 giây | `data-1` | = 60 | `sudo -u postgres psql -tAc 'show tcp_keepalives_idle'` |
| Dùng hugepage nếu có (try) | `data-1` | = try | `sudo -u postgres psql -tAc 'show huge_pages'` |
| pg_hba có đúng một dòng hostssl | `data-1` | = 1 | `grep -c '^hostssl' /etc/postgresql/17/main/pg_hba.conf` |
| pg_hba không có trust | `data-1` | = 0 | `grep -c trust /etc/postgresql/17/main/pg_hba.conf` |
| Ghi log kết nối | `data-1` | = on | `sudo -u postgres psql -tAc 'show log_connections'` |
| postgresql@17-main: LimitNOFILE | `data-1` | = 65536 | `systemctl show postgresql@17-main -p LimitNOFILE --value` |
| rabbitmq-server: LimitNOFILE | `data-1` | = 65536 | `systemctl show rabbitmq-server -p LimitNOFILE --value` |
| valkey-server: LimitNOFILE | `data-1` | = 65536 | `systemctl show valkey-server -p LimitNOFILE --value` |
| Vùng ứng dụng tới được 5432 (N14) | `k8s-wk-1` | lệnh thành công (rc=0) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.40.11/5432'` |
| Edge không tới được 5432 (X01) | `edge-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.40.11/5432'` |

**Hoàn tác:** apt-mark unhold rồi apt purge postgresql-{pg_version}; xoá /var/lib/waap/data/pg và /var/lib/waap/wal/pg.

### P8.4 RabbitMQ (repo Team RabbitMQ)

**Vì sao:** Erlang và RabbitMQ từ repo của Team RabbitMQ (khoá ký ghim fingerprint), không dùng gói Ubuntu; gói bị hold vì Erlang phải tương thích RabbitMQ. Không tài khoản guest; chỉ nghe IP lab.

**Phụ thuộc:** P8.3  ·  **Thời gian dự kiến:** 2 phút

```
$ labs/bin/lab services --tags rabbitmq
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| rabbitmq-server đến từ repo Team RabbitMQ | `data-1` | ≥ 1 | `apt-cache policy rabbitmq-server \| grep -cE 'deb[12].rabbitmq.com'` |
| Erlang đến từ repo Team RabbitMQ (không phải bản của Ubuntu) | `data-1` | ≥ 1 | `apt-cache policy erlang-base \| grep -cE 'deb[12].rabbitmq.com'` |
| rabbitmq-server bị hold | `data-1` | = 1 | `apt-mark showhold \| grep -c '^rabbitmq-server$'` |
| Erlang bị hold (không nâng riêng) | `data-1` | = 1 | `apt-mark showhold \| grep -c '^erlang-base$'` |
| Khoá ký repo đúng fingerprint đã ghim | `data-1` | ≥ 1 | `gpg --batch --show-keys --with-colons /usr/share/keyrings/rabbitmq-server.gpg \| awk -F: '/^fpr:/{print $10}' \| grep -c 0A9AF2115F4687BD29803A206B73A36E6026DFCA` |
| RabbitMQ chạy | `data-1` | = active | `systemctl is-active rabbitmq-server` |
| Không còn user guest | `data-1` | = 0 | `rabbitmqctl list_users --silent \| grep -c '^guest'` |
| Chỉ nghe IP lab ở 5672 | `data-1` | = 1 | `ss -ltn \| grep -c '10.10.40.11:5672'` |
| Không nghe 0.0.0.0 | `data-1` | = 0 | `ss -ltn \| grep -c '0.0.0.0:5672'` |
| User ứng dụng không là administrator | `data-1` | = 0 | `rabbitmqctl list_users --silent \| grep '^metap' \| grep -c administrator` |
| epmd chỉ nghe loopback | `data-1` | = 0 | `ss -ltn \| grep ':4369' \| grep -vc '127.0.0.1'` |
| Cổng phân tán Erlang chỉ nghe loopback | `data-1` | = 0 | `ss -ltn \| grep ':25672' \| grep -vc '127.0.0.1'` |
| Không có loopback_users=none (không dùng cho production) | `data-1` | = 0 | `grep -c 'loopback_users' /etc/rabbitmq/rabbitmq.conf` |
| Ngưỡng bộ nhớ tường minh 0.4 | `data-1` | = 1 | `grep -c '^vm_memory_high_watermark.relative = 0.4' /etc/rabbitmq/rabbitmq.conf` |
| Ngưỡng đĩa trống >= ngưỡng bộ nhớ (2 GB) | `data-1` | = 1 | `grep -c '^disk_free_limit.absolute = 2GB' /etc/rabbitmq/rabbitmq.conf` |
| postgresql@17-main: LimitNOFILE | `data-1` | = 65536 | `systemctl show postgresql@17-main -p LimitNOFILE --value` |
| rabbitmq-server: LimitNOFILE | `data-1` | = 65536 | `systemctl show rabbitmq-server -p LimitNOFILE --value` |
| valkey-server: LimitNOFILE | `data-1` | = 65536 | `systemctl show valkey-server -p LimitNOFILE --value` |
| Cổng 5672 thường chưa mở cho vùng ứng dụng (N15 chỉ mở 5671 TLS; TLS làm ở P2/P5) | `k8s-wk-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.40.11/5672'` |

**Hoàn tác:** apt-mark unhold rồi apt purge rabbitmq-server.

### P8.5 Valkey (valkey-central trên data-1, valkey-edge trên edge-1)

**Vì sao:** Valkey thay Redis (Redis 7.4 trở lên đổi giấy phép, ADR-0024 loại). Binary chính thức ghim sha256 vào /opt/valkey/<version>, nâng bằng đổi symlink current. ACL, user default tắt, không loại bỏ khoá (cấu hình, không phải cache), lệnh nguy hiểm chặn bằng ACL.

**Phụ thuộc:** P8.4  ·  **Thời gian dự kiến:** 2 phút

```
$ labs/bin/lab services --tags valkey
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Binary nằm trong /opt/valkey/<version> qua symlink current | `data-1` | = 1 | `readlink -f /opt/valkey/current \| grep -c '^/opt/valkey/'` |
| Version đang chạy khớp thư mục (đọc bằng mắt) | `data-1` | THỦ CÔNG (valkey-server --version) | |
| Valkey chạy | `data-1` | = active | `systemctl is-active valkey-server` |
| Valkey chạy | `edge-1` | = active | `systemctl is-active valkey-server` |
| Không xác thực thì bị từ chối | `data-1` | chứa NOAUTH | `valkey-cli -h 10.10.40.11 ping` |
| User control ghi được | `data-1` | = OK | `valkey-cli -h 10.10.40.11 --no-auth-warning --user control --pass {secret:redis_control_password} set waf:probe 1` |
| User edge không ghi được | `data-1` | chứa NOPERM | `valkey-cli -h 10.10.40.11 --no-auth-warning --user edge --pass {secret:redis_edge_password} set waf:probe 2` |
| User edge đọc được | `data-1` | = 1 | `valkey-cli -h 10.10.40.11 --no-auth-warning --user edge --pass {secret:redis_edge_password} get waf:probe` |
| FLUSHALL bị chặn bằng ACL (-@dangerous) | `data-1` | chứa NOPERM | `valkey-cli -h 10.10.40.11 --no-auth-warning --user control --pass {secret:redis_control_password} flushall` |
| Không loại bỏ khoá | `data-1` | = 1 | `valkey-cli -h 10.10.40.11 --no-auth-warning --user control --pass {secret:redis_control_password} info memory \| grep -c 'maxmemory_policy:noeviction'` |
| AOF bật | `data-1` | = 1 | `valkey-cli -h 10.10.40.11 --no-auth-warning --user control --pass {secret:redis_control_password} info persistence \| grep -c 'aof_enabled:1'` |
| valkey-server: LimitNOFILE | `edge-1` | = 65536 | `systemctl show valkey-server -p LimitNOFILE --value` |
| Thư mục dữ liệu Valkey ghi được dưới sandbox của unit | `data-1` | chứa /var/lib/waap/data/valkey | `systemctl show valkey-server -p ReadWritePaths --value` |

**Hoàn tác:** rm /etc/systemd/system/valkey-server.service* và /opt/valkey; xoá user valkey.

### P8.6 ClickHouse

**Vì sao:** Cài từ repo apt chính thức của ClickHouse (khoá ký ghim fingerprint), ba gói cùng một version và bị hold; chỉ nhận từ vùng ứng dụng; dữ liệu trên đĩa riêng.

**Phụ thuộc:** P8.5  ·  **Thời gian dự kiến:** 4 phút

```
$ labs/bin/lab services --tags clickhouse
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Gói đến từ repo packages.clickhouse.com | `ch-1` | ≥ 1 | `apt-cache policy clickhouse-server \| grep -c 'packages.clickhouse.com'` |
| Ba gói ClickHouse cùng một version | `ch-1` | = 1 | `dpkg-query -W -f='${Version}\n' clickhouse-server clickhouse-client clickhouse-common-static \| sort -u \| wc -l` |
| Gói ClickHouse bị hold | `ch-1` | ≥ 3 | `apt-mark showhold \| grep -c 'clickhouse'` |
| Khoá ký repo đúng fingerprint đã ghim | `ch-1` | ≥ 1 | `gpg --batch --show-keys --with-colons /usr/share/keyrings/clickhouse.gpg \| awk -F: '/^fpr:/{print $10}' \| grep -c 3A9EA1193A97B548BE1457D48919F6BD2B48D754` |
| ClickHouse chạy | `ch-1` | = active | `systemctl is-active clickhouse-server` |
| Truy vấn được | `ch-1` | = 1 | `clickhouse-client -q 'select 1'` |
| Thư mục dữ liệu trên đĩa riêng | `ch-1` | = /var/lib/waap/data/clickhouse/ | `clickhouse-client -q "select value from system.server_settings where name='path'"` |
| Chỉ nghe IP lab | `ch-1` | = 0 | `ss -ltn \| grep -c '0.0.0.0:9000'` |
| clickhouse-server: LimitNOFILE | `ch-1` | = 262144 | `systemctl show clickhouse-server -p LimitNOFILE --value` |
| Có tài khoản analytics (có mật khẩu) | `ch-1` | = 1 | `clickhouse-client -q "select count() from system.users where name='analytics'"` |
| 70% RAM cho server | `ch-1` | = 0.7 | `clickhouse-client -q "select value from system.server_settings where name='max_server_memory_usage_to_ram_ratio'"` |
| Cổng native 9000 chưa mở cho vùng ứng dụng (N17 chỉ mở 8443 và 9440 | `k8s-wk-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.40.12/9000'` |

**Hoàn tác:** apt-mark unhold rồi apt purge clickhouse-server clickhouse-client clickhouse-common-static.

### P8.7 egress-1 (squid, đường ra duy nhất có allowlist)

**Vì sao:** Chỉ Z3 và ops ra Internet qua proxy; chặn dải riêng và metadata cloud; chỉ domain trong allowlist.

**Phụ thuộc:** P8.6  ·  **Thời gian dự kiến:** 2 phút

```
$ labs/bin/lab services --tags egress
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| squid chạy | `perimeter-1` | = active | `systemctl is-active squid` |
| Nghe 3128 ở IP lab | `perimeter-1` | = 1 | `ss -ltn \| grep -c '10.10.20.11:3128'` |
| Tới domain trong allowlist (github.com) | `ops-1` | khớp /^(200\|301)$/ | `curl -s -o /dev/null -w '%{http_code}' --max-time 15 -x http://10.10.20.11:3128 https://github.com` |
| Domain ngoài allowlist bị chặn (CONNECT 403) | `ops-1` | = 403 | `curl -s -o /dev/null -w '%{http_connect}' --max-time 15 -x http://10.10.20.11:3128 https://example.com` |
| Dải riêng bị chặn qua proxy | `ops-1` | = 403 | `curl -s -o /dev/null -w '%{http_code}' --max-time 10 -x http://10.10.20.11:3128 http://10.10.40.11/` |
| Metadata cloud bị chặn qua proxy | `ops-1` | = 403 | `curl -s -o /dev/null -w '%{http_code}' --max-time 10 -x http://10.10.20.11:3128 http://169.254.169.254/` |

**Hoàn tác:** apt purge squid.

### P8.8 Mirror nội bộ ở registry (aptly, kho artifact, apt-cacher-ng)

**Vì sao:** Sau seal các vùng dữ liệu không có đường ra, nên mirror là nơi duy nhất lấy bản database mới. Mirror lấy từ vendor qua egress-1, chụp snapshot, ký lại bằng khoá riêng; client ghim khoá đó. Bản mới chỉ tới client khi chạy promote.

**Phụ thuộc:** P8.7  ·  **Thời gian dự kiến:** 10 phút

```
$ labs/bin/lab services --tags registry
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| nginx chạy | `ops-1` | = active | `systemctl is-active nginx` |
| apt-cacher-ng chạy | `ops-1` | = active | `systemctl is-active apt-cacher-ng` |
| Mirror đã tạo đủ 4 repo vendor | `ops-1` | = 4 | `sudo -u aptly aptly mirror list -raw \| wc -l` |
| Cả 4 repo đã được publish | `ops-1` | = 4 | `sudo -u aptly aptly publish list -raw \| wc -l` |
| Mirror phục vụ khoá công khai | `ops-1` | = 200 | `curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1/apt/mirror-key.asc` |
| Có timer đồng bộ hằng tuần | `ops-1` | = enabled | `systemctl is-enabled waap-mirror-sync.timer` |
| Khoá mirror đã về máy điều khiển (client ghim) | `host` | lệnh thành công (rc=0) | `test -s labs/infra/generated/mirror-key.fpr` |

**Hoàn tác:** systemctl disable --now nginx apt-cacher-ng; apt purge aptly. Dữ liệu ở /var/lib/aptly.

## P9. Khoá (seal)

### P9.1 Seal (gỡ bootstrap)

**Vì sao:** Gỡ đường ra Internet tạm của mọi vùng; nguồn vá chuyển sang egress-1; SSH chỉ cert; egress khoá cho vai trò nhạy cảm.

**Phụ thuộc:** P8.8  ·  **Thời gian dự kiến:** 8 phút

```
$ labs/bin/lab seal
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Không còn luật BOOTSTRAP | `fw-1` | = 0 | `nft list chain inet lab forward \| grep -c BOOTSTRAP` |
| Khoá SSH tĩnh bị tắt | `all` | = none | `sshd -T \| awk '/^authorizedkeysfile/{print $2}'` |
| apt đi qua apt-cacher-ng của registry | `role:k8s-app` | = 1 | `grep -c 'Acquire::http::Proxy' /etc/apt/apt.conf.d/90waap-proxy` |
| apt đi qua egress-1 | `ops-1` | = 1 | `grep -c 'Acquire::http::Proxy' /etc/apt/apt.conf.d/90waap-proxy` |
| OpenBao không có đường ra (egress khoá) | `bao-1` | = 1 | `nft list chain inet host output \| grep -c 'egress-locked'` |
| Kho dữ liệu không có đường ra | `data-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/1.1.1.1/443'` |

**Hoàn tác:** labs/bin/lab fw bootstrap rồi lab provision -e lab_bootstrap=true (chỉ để gỡ lỗi).

### P9.2 Kiểm mạng và hạ tầng sau khi seal

**Vì sao:** Chạy lại thăm dò mạng và kiểm sống; lần cuối trước khi giao cho các bước triển khai dịch vụ.

**Phụ thuộc:** P9.1  ·  **Thời gian dự kiến:** 5 phút

```
$ labs/bin/lab nettest
$ labs/bin/lab smoke
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Thăm dò mạng đúng kỳ vọng | `host` | lệnh thành công (rc=0) | `labs/bin/lab nettest` |
| Kiểm sống hạ tầng | `host` | lệnh thành công (rc=0) | `labs/bin/lab smoke` |

**Hoàn tác:** Không đổi gì.

### P9.3 Cài lại database từ mirror sau khi seal

**Vì sao:** Chứng minh vùng dữ liệu (không có đường ra) vẫn cài và nâng được phần mềm, chỉ qua mirror nội bộ.

**Phụ thuộc:** P9.2  ·  **Thời gian dự kiến:** 5 phút

```
$ labs/bin/lab services --tags postgres
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Nguồn apt PostgreSQL trỏ vào registry | `data-1` | = 1 | `grep -c '10.10.60.12/apt/pgdg' /etc/apt/sources.list.d/pgdg.sources` |
| Khoá Signed-By là khoá của mirror | `data-1` | = 41 | `gpg --show-keys --with-colons /usr/share/keyrings/pgdg.gpg \| awk -F: '/^fpr/{print $10}' \| head -1 \| wc -c` |
| apt update chỉ qua mirror | `data-1` | khớp /^[1-9]/ | `apt-get update -o Dir::Etc::sourcelist=/etc/apt/sources.list.d/pgdg.sources -o Dir::Etc::sourceparts=- 2>&1 \| grep -c '10.10.60.12'` |
| PostgreSQL còn chạy sau khi chạy lại | `data-1` | = active | `systemctl is-active postgresql@17-main` |

**Hoàn tác:** Không đổi gì nếu phiên bản đã ghim không đổi.

### P9.4 Vá và cài gói cho Z1 qua egress-1 (M03)

**Vì sao:** Z1 không tới được registry (X01) nhưng được tới egress-1 ở Z2, nên vá Ubuntu và tải gói vendor đi qua proxy có allowlist.

**Phụ thuộc:** P9.2  ·  **Thời gian dự kiến:** 3 phút

```
$ labs/bin/lab nettest
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| apt đi qua egress-1 | `edge-1` | = 1 | `grep -c '10.10.20.11:3128' /etc/apt/apt.conf.d/90waap-proxy` |
| apt update chạy được qua proxy | `edge-1` | lệnh thành công (rc=0) | `apt-get update -qq` |
| Không tới được registry (X01) | `edge-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.60.12/80'` |
| Không có đường ra trực tiếp | `edge-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/1.1.1.1/443'` |

**Hoàn tác:** Xoá luồng M03 trong flows.yaml rồi lab fw sealed; Z1 quay về không có đường vá.

### P9.5 Đẩy bản vá vào Z5 từ registry

**Vì sao:** Z5 (secret) không gọi ra bất kỳ vùng nào (X05). Bản vá do registry chuẩn bị, đi qua máy điều khiển và bastion (N25) vào Z5; máy đích kiểm sha256 rồi cài từ file cục bộ.

**Phụ thuộc:** P9.2  ·  **Thời gian dự kiến:** 3 phút

```
$ labs/bin/lab push
```

| Kiểm | Ở đâu | Phải là | Lệnh |
|---|---|---|---|
| Không tới được registry | `bao-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/10.10.60.12/80'` |
| Không có đường ra | `bao-1` | lệnh thất bại (bị chặn) | `timeout 4 bash -c 'exec 3<>/dev/tcp/1.1.1.1/443'` |
| Thư mục nhận đã dọn sau khi cài | `bao-1` | = 0 | `ls /var/cache/waap-push/debs 2>/dev/null \| wc -l` |
| Registry có script chuẩn bị gói | `ops-1` | lệnh thành công (rc=0) | `test -x /usr/local/sbin/waap-mirror-push-prepare` |
| Chạy lại không lỗi (không có gì mới thì thoát sớm) | `host` | lệnh thành công (rc=0) | `labs/bin/lab push` |

**Hoàn tác:** Cài lại bản cũ bằng dpkg -i từ /var/cache/apt/archives nếu cần; snapshot VM trước khi đẩy.

