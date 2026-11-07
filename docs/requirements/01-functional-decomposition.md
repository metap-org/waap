# 1. Bản đồ phân rã chức năng

> **Trạng thái: bản nháp chờ review (2026-11-06).** Đây là đầu vào cho [§2 Requirements](02-requirements.md)
> và cho thiết kế dữ liệu ([5.6](../architecture/05-building-blocks/06-data-model.md), [5.8.14](../architecture/05-building-blocks/08-components/14-management-database.md)).
> Thiết kế database đã viết trước bản đồ này, nên phải được đối chiếu lại sau khi bản đồ và requirements chốt.

Bản đồ trả lời câu hỏi **hệ thống phải làm những việc gì**. Có hai cách nhìn trên cùng một tập chức năng lá:

- **Theo nghiệp vụ** (mục F1–F12): chức năng gom theo mảng nghiệp vụ người dùng nhận ra.
- **Theo plane** ([Phân rã theo plane](#phân-rã-theo-plane)): cùng các lá đó chia theo plane chủ và thành phần,
  kèm [liên kết giữa các plane](#liên-kết-giữa-các-plane) cho biết chức năng nào của plane này cần chức năng nào của plane khác.

Cách nhìn theo nghiệp vụ phân rã sản phẩm WAAP theo ba cấp:

| Cấp | Mã | Ý nghĩa | Ví dụ |
|---|---|---|---|
| 1 | `F1`…`F12` | Nhóm chức năng, ứng với một mảng nghiệp vụ | F5 Vòng đời policy |
| 2 | `F5.3` | Chức năng: một việc người dùng hoặc hệ thống nhận ra được | F5.3 Duyệt và phát hành |
| 3 | `F5.3.2` | Chức năng lá: đủ nhỏ để viết requirement và kiểm thử | F5.3.2 Duyệt 4-eyes |

Quy tắc:

- **Mỗi chức năng lá có đúng một plane chủ** (M = Management, C = Control, E = Edge, A = Analytics). Plane
  chủ là nơi giữ quyết định và dữ liệu gốc của chức năng; plane khác chỉ gọi tới hoặc nhận kết quả.
- **Mức** theo [1.2](../architecture/01-introduction/00-index.md#12-tổng-quan-yêu-cầu): **MVP**, **MR** (Mở rộng), **NC** (Nâng cao).
- Cột **Dữ liệu** ghi đối tượng dữ liệu chính mà chức năng tạo ra hoặc sửa (ghi đậm) hay chỉ đọc. Đây là
  cầu nối sang thiết kế database: một đối tượng không có chức năng nào tạo ra là thừa, một chức năng ghi
  dữ liệu mà không có đối tượng tương ứng là thiếu.
- Persona dùng tên ở [1.4](../architecture/01-introduction/00-index.md#14-các-bên-liên-quan).

## Tổng quan cấp 1 và cấp 2

```mermaid
flowchart LR
    root(("WAAP"))
    g1["<b>F1 Tenant và danh tính</b><br/>F1.1 Tổ chức và account<br/>F1.2 Thành viên và phân quyền<br/>F1.3 Đăng nhập và SSO<br/>F1.4 API key<br/>F1.5 Audit log"]
    g2["<b>F2 Zone và onboarding</b><br/>F2.1 Thêm và xác minh domain<br/>F2.2 Hostname và origin<br/>F2.3 Chứng chỉ TLS<br/>F2.4 Vòng đời zone<br/>F2.5 Nhóm zone<br/>F2.6 Cấu hình ban đầu và kiểm thử<br/>F2.7 Chuyển traffic và hoàn tất"]
    g3["<b>F3 Bảo vệ runtime</b><br/>F3.1 Kết thúc TLS và chuẩn hoá<br/>F3.2 Lọc sớm<br/>F3.3 DDoS L7<br/>F3.4 Rate limit<br/>F3.5 Managed ruleset và custom rule<br/>F3.6 Bot management<br/>F3.7 API security<br/>F3.8 Thực thi hành động<br/>F3.9 Bảo vệ origin và fail mode"]
    g4["<b>F4 Hiệu năng và phân phối</b><br/>F4.1 Cache<br/>F4.2 Purge<br/>F4.3 DNS điều hướng<br/>F4.4 Load balancing và failover"]
    g5["<b>F5 Vòng đời policy</b><br/>F5.1 Soạn policy<br/>F5.2 Kiểm tra và mô phỏng<br/>F5.3 Duyệt và phát hành<br/>F5.4 Version và rollback<br/>F5.5 Kênh khẩn<br/>F5.6 Policy as code"]
    g6["<b>F6 Phân phối cấu hình và fleet</b><br/>F6.1 Biên dịch<br/>F6.2 Phân phối<br/>F6.3 Rollout an toàn<br/>F6.4 Fleet và danh tính node<br/>F6.5 Đặt zone vào cell"]
    g7["<b>F7 Quan sát và điều tra</b><br/>F7.1 Thu nhận telemetry<br/>F7.2 Xem realtime<br/>F7.3 Tìm kiếm và tra cứu<br/>F7.4 Dashboard<br/>F7.5 Chặn nhầm và hiệu quả rule<br/>F7.6 Export SIEM"]
    g8["<b>F8 Phát hiện, cảnh báo, sự cố</b><br/>F8.1 Phát hiện<br/>F8.2 Alert policy<br/>F8.3 Gửi thông báo<br/>F8.4 Case và sự cố"]
    g9["<b>F9 Báo cáo và tuân thủ</b><br/>F9.1 Báo cáo định kỳ<br/>F9.2 Báo cáo tuân thủ<br/>F9.3 Retention và archive<br/>F9.4 Data residency"]
    g10["<b>F10 Gói và billing</b><br/>F10.1 Danh mục gói<br/>F10.2 Subscription<br/>F10.3 Entitlements<br/>F10.4 Đo usage<br/>F10.5 Hạn mức và trần cứng<br/>F10.6 Hoá đơn và thanh toán"]
    g11["<b>F11 Threat intelligence</b><br/>F11.1 Phát hành ruleset<br/>F11.2 Rule khẩn toàn fleet<br/>F11.3 IP reputation<br/>F11.4 Virtual patching"]
    g12["<b>F12 Vận hành NCC</b><br/>F12.1 Giám sát nền tảng<br/>F12.2 Xử lý sự cố nền tảng<br/>F12.3 Support<br/>F12.4 Quản trị nền tảng"]
    g1 --- root
    g2 --- root
    g3 --- root
    g4 --- root
    g5 --- root
    g6 --- root
    root --- g7
    root --- g8
    root --- g9
    root --- g10
    root --- g11
    root --- g12
    style g1 text-align:left
    style g2 text-align:left
    style g3 text-align:left
    style g4 text-align:left
    style g5 text-align:left
    style g6 text-align:left
    style g7 text-align:left
    style g8 text-align:left
    style g9 text-align:left
    style g10 text-align:left
    style g11 text-align:left
    style g12 text-align:left
```

Phân bố theo plane chủ:

```mermaid
flowchart LR
    subgraph M["Management"]
        F1["F1 Tenant"]
        F2["F2 Zone"]
        F5["F5 Policy"]
        F9["F9 Báo cáo"]
        F10["F10 Billing"]
    end
    subgraph C["Control"]
        F6["F6 Phân phối"]
        F11["F11 Threat intel"]
    end
    subgraph E["Edge"]
        F3["F3 Bảo vệ runtime"]
        F4["F4 Hiệu năng"]
    end
    subgraph A["Analytics"]
        F7["F7 Quan sát"]
        F8["F8 Cảnh báo, sự cố"]
    end
    F12["F12 Vận hành NCC<br/>(mọi plane)"]
    M -->|cấu hình có version| C -->|snapshot ký| E -->|telemetry| A
    A -->|cảnh báo, usage, đề xuất| M
```

Một nhóm cấp 1 có thể có lá ở plane khác plane chủ của nhóm. Ví dụ F8.4 Case thuộc Management dù phát
hiện ở Analytics, F10.4 Đo usage đếm ở Edge và gộp ở Analytics. Bảng dưới ghi plane cho từng lá.

## F1 Tenant và danh tính

Ai là khách, ai được làm gì. Persona chính: chủ tài khoản, quản trị bảo mật, DevOps.

| Mã | Chức năng lá | Plane | Mức | Dữ liệu |
|---|---|---|---|---|
| **F1.1** | **Tổ chức và account** | | | |
| F1.1.1 | Đăng ký org mới (self-service) hoặc NCC tạo org (Enterprise) | M | MVP | **Org**, **Account** |
| F1.1.2 | Tạo, đổi tên, đóng account trong org | M | MVP | **Account** |
| F1.1.3 | Chọn region dữ liệu cho account | M | NC | **Account**.region |
| F1.1.4 | Xoá org theo yêu cầu (sau thời hạn giữ dữ liệu) | M | MVP | **Org**, toàn bộ dữ liệu tenant |
| **F1.2** | **Thành viên và phân quyền** | | | |
| F1.2.1 | Mời thành viên qua email, chấp nhận lời mời | M | MVP | **Invitation**, **Member** |
| F1.2.2 | Gán vai trò có sẵn ở cấp org, account, nhóm zone hoặc zone | M | MVP | **RoleBinding** |
| F1.2.3 | Tạo vai trò tuỳ chỉnh từ tập quyền | M | MR | **Role** |
| F1.2.4 | Gỡ thành viên, thu hồi mọi phiên và key của họ | M | MVP | **Member**, Session, **ApiKey** |
| F1.2.5 | Đồng bộ user và nhóm từ IdP qua SCIM | M | MR | **Member**, **Group** |
| F1.2.6 | Một người thuộc nhiều org, chuyển org trong portal ([3.6](03-business-analysis/06-members-access.md#3-đã-quyết)) | M | MVP | User, **Member** |
| **F1.3** | **Đăng nhập và SSO** | | | |
| F1.3.1 | Đăng nhập email + mật khẩu, MFA (TOTP, WebAuthn) | M | MVP | User, **MfaFactor** |
| F1.3.2 | Cấu hình SSO SAML/OIDC theo domain email | M | MVP | **SsoConnection**, **SsoDomain** |
| F1.3.3 | Bắt buộc SSO, bắt buộc MFA theo org | M | MVP | **Org**.securitySettings |
| F1.3.4 | Quản lý phiên: xem, đăng xuất từ xa, hết hạn | M | MVP | Session |
| **F1.4** | **API key** | | | |
| F1.4.1 | Tạo API key có phạm vi (quyền, zone) và ngày hết hạn | M | MVP | **ApiKey** |
| F1.4.2 | Xoay, thu hồi API key; xem lần dùng cuối | M | MVP | **ApiKey**.lastUsedAt |
| F1.4.3 | Service account cho CI/CD, Terraform | M | MVP | **ServiceAccount** |
| **F1.5** | **Audit log** | | | |
| F1.5.1 | Ghi mọi thay đổi: ai, cái gì, trước/sau, lúc nào, từ IP nào, qua UI hay API | M | MVP | **AuditEntry** |
| F1.5.2 | Xem, lọc audit log theo người, đối tượng, thời gian | M | MVP | AuditEntry |
| F1.5.3 | Xuất audit log (CSV, JSON) và đẩy liên tục sang SIEM của khách qua cùng exporter với log; gồm thao tác của nhân viên NCC trên zone khách | M | MVP | AuditEntry |
| F1.5.4 | Chống sửa audit log (chuỗi hash), kiểm được tính toàn vẹn khi xuất | M | MVP | AuditEntry |

## F2 Zone và onboarding

> **Chờ onboarding.** F2.1.5–F2.1.8, F2.2.6, F2.2.7, F2.3.6, F2.6, F2.7 và plane C của F2.1.2, F2.1.4, F2.2.3 đến
> từ bản nháp onboarding chưa duyệt; onboarding đang được phân tích lại theo nhiều kịch bản
> ([§3.1](03-business-analysis/01-onboarding.md)). Nhóm F2 sẽ được viết lại sau khi các câu hỏi ở đó có trả lời.

Đưa một site vào bảo vệ. Persona chính: DevOps, quản trị bảo mật.

| Mã | Chức năng lá | Plane | Mức | Dữ liệu |
|---|---|---|---|---|
| **F2.1** | **Thêm và xác minh domain** | | | |
| F2.1.1 | Thêm zone theo domain gốc; chặn domain đã thuộc tenant khác | M | MVP | **Zone**, **DomainClaim** (toàn cục) |
| F2.1.2 | Xác minh quyền sở hữu: CNAME uỷ quyền `_acme-challenge` (mặc định) hoặc TXT, kiểm từ nhiều resolver, kiểm lại định kỳ | C | MVP | **DomainVerification** |
| F2.1.3 | Hướng dẫn trỏ DNS theo phương thức kết nối (CNAME, ALIAS, A tĩnh cho apex, nameserver, sau CDN của khách) | M | MVP | **Zone**.connectionMethod |
| F2.1.4 | Quét bản ghi DNS hiện có khi chuyển nameserver | C | MR | **DnsRecord** |
| F2.1.5 | Tra DNS hiện tại khi thêm domain: NS, A/AAAA/CNAME, CAA, TTL, CDN hoặc WAF đang đứng trước, gợi ý origin | C | MVP | **DnsSnapshot** |
| F2.1.6 | Gợi ý hostname từ DNS và Certificate Transparency | C | MR | DnsSnapshot |
| F2.1.7 | Kiểm trạng thái trỏ DNS về edge từ nhiều resolver, theo hostname | C | MVP | **Hostname**.dnsRoutingStatus |
| F2.1.8 | Huỷ claim chưa xác minh sau 14 ngày, giải phóng domain | M | MVP | **DomainClaim** |
| **F2.2** | **Hostname và origin** | | | |
| F2.2.1 | Thêm, xoá hostname trong zone; wildcard | M | MVP | **Hostname**, **HostnameClaim** (toàn cục) |
| F2.2.2 | Khai báo origin (IP, hostname, cổng, giao thức, SNI, header host) | M | MVP | **Origin** |
| F2.2.3 | Kiểm origin từ edge ở ≥ 2 PoP trước khi kích hoạt (kết nối, TLS, mã HTTP, thời gian) | C | MVP | **OriginCheck** |
| F2.2.4 | Cấu hình timeout, retry, keepalive tới origin | M | MVP | **Origin** |
| F2.2.5 | Chế độ đứng sau CDN của khách (tin IP và header client của CDN đó) | M | MVP | **ZoneSettings** |
| F2.2.6 | Kiểm origin còn lộ: thử kết nối từ điểm không thuộc edge | C | MR | **OriginCheck** |
| F2.2.7 | Nhập hàng loạt hostname, origin (CSV, API) | M | MR | **BulkOperation** |
| **F2.3** | **Chứng chỉ TLS** | | | |
| F2.3.1 | Cấp cert tự động (ACME) cho hostname đã xác minh | C | MVP | **CertRequest**, Cert (khoá ở secret store) |
| F2.3.2 | Gia hạn tự động, cảnh báo khi gia hạn lỗi | C | MVP | Cert |
| F2.3.3 | Khách tải cert riêng lên | M | MR | **CustomCert** (khoá ở secret store) |
| F2.3.4 | Cấu hình TLS: phiên bản tối thiểu, cipher, HSTS, chế độ TLS tới origin | M | MVP | **ZoneSettings** |
| F2.3.5 | Keyless (khoá ở hạ tầng khách) | E | NC | — |
| F2.3.6 | Kiểm CAA trước khi xin cert, báo bản ghi cần thêm | C | MVP | DnsSnapshot |
| **F2.4** | **Vòng đời zone** | | | |
| F2.4.1 | Kích hoạt zone khi qua cổng OG1 (xác minh, cert, origin, preset), phát cấu hình xuống edge | M | MVP | **Zone**.status |
| F2.4.2 | Tạm dừng zone (chuyển tiếp không bảo vệ), mở lại | M | MVP | **Zone**.status |
| F2.4.3 | Gỡ zone, giải phóng domain và hostname | M | MVP | **Zone**, DomainClaim, HostnameClaim |
| F2.4.4 | Chuyển zone giữa các account cùng org | M | MR | **Zone**.accountId |
| **F2.5** | **Nhóm zone** | | | |
| F2.5.1 | Gắn tag, tạo nhóm zone để phân quyền và áp policy | M | MVP | **ZoneGroup**, **ZoneTag** |
| F2.5.2 | Thao tác hàng loạt trên nhóm zone (bật tính năng, đổi mặc định) | M | MR | **BulkOperation** |
| **F2.6** | **Cấu hình ban đầu và kiểm thử** ([requirements §3.1](03-business-analysis/01-onboarding.md) OB6, OB7) | | | |
| F2.6.1 | Chọn preset theo loại ứng dụng: managed ruleset log-only, DDoS L7 và lọc sớm bật chặn, exception sẵn có | M | MVP | **ZonePreset** (NCC), **PolicyDraft** |
| F2.6.2 | Phục vụ request kiểm thử qua edge trước khi đổi DNS (`curl --resolve`, `hosts`) | E | MVP | — |
| F2.6.3 | Checklist kiểm thử (trang, đăng nhập, upload, API, WebSocket) và kết quả | M | MR | **OnboardingProgress** |
| **F2.7** | **Chuyển traffic và hoàn tất** ([requirements §3.1](03-business-analysis/01-onboarding.md) OB8–OB12) | | | |
| F2.7.1 | Hướng dẫn hạ TTL, đổi DNS, kế hoạch quay lui (lưu bản ghi cũ) | M | MVP | DnsSnapshot |
| F2.7.2 | Ghi nhận traffic thật đầu tiên qua edge theo hostname | M | MVP | **OnboardingProgress** |
| F2.7.3 | Báo cáo hằng ngày trong log-only: request sẽ bị chặn, rule nghi chặn nhầm, mức sẵn sàng | A | MVP | Bảng tổng hợp |
| F2.7.4 | Bật chặn theo bậc nhóm rule qua change request | M | MVP | **ChangeRequest** |
| F2.7.5 | Hướng dẫn khoá origin, ghi nhận đã khoá hoặc bỏ qua | M | MVP | **OnboardingProgress** |
| F2.7.6 | Tiến độ onboarding theo bước, nhắc khi bỏ dở ở ngày 1, 3, 7 | M | MVP | **OnboardingProgress** |

## F3 Bảo vệ runtime

Chạy trên từng request ở edge. Không có UI riêng: cấu hình qua F5, kết quả xem qua F7.

| Mã | Chức năng lá | Plane | Mức | Dữ liệu |
|---|---|---|---|---|
| **F3.1** | **Kết thúc TLS và chuẩn hoá** | | | |
| F3.1.1 | Kết thúc TLS theo SNI, HTTP/1.1, HTTP/2, HTTP/3 | E | MVP | Cert (đọc) |
| F3.1.2 | Xác định zone và hostname của request | E | MVP | Routing index (đọc) |
| F3.1.3 | Gán request ID, chuẩn hoá URI, header, giới hạn kích thước | E | MVP | — |
| F3.1.4 | Nhận diện IP client thật khi đứng sau CDN của khách: khách khai header chứa IP thật và dải IP của CDN; chỉ tin header khi kết nối đến từ dải đó; preset cho CDN phổ biến | E | MVP | ZoneSettings, TrustedProxyRange (đọc) |
| F3.1.5 | SSL passthrough theo SNI cho hostname đã chọn (Enterprise): không giải mã, chỉ định tuyến và chống DDoS L3/L4 | E | MR | ZoneSettings (đọc) |
| **F3.2** | **Lọc sớm** | | | |
| F3.2.1 | Access list IP, CIDR, quốc gia, ASN (allow, block, challenge) | E | MVP | AccessList (đọc) |
| F3.2.2 | Chặn theo IP reputation | E | MR | Reputation feed (đọc) |
| F3.2.3 | Chặn request sai giao thức rõ ràng | E | MVP | — |
| F3.2.4 | Hành động Trust (bỏ qua bot và challenge; WAF vẫn kiểm; vẫn chịu DDoS L7 và rate limit) và Skip (bỏ qua một chặng chỉ định); cả hai vẫn ghi log | E | MVP | AccessList, CustomRule (đọc) |
| **F3.3** | **DDoS L7** | | | |
| F3.3.1 | Đo traffic theo zone và nguồn, phát hiện đột biến theo baseline | E | MVP | Bộ đếm (bộ nhớ) |
| F3.3.2 | Giảm thiểu tự động theo độ nhạy và hành động khách chọn | E | MVP | DdosSettings (đọc) |
| F3.3.3 | Under Attack mode: challenge mọi request chưa có clearance | E | MVP | ZoneSettings (đọc) |
| F3.3.4 | Chuyển zone bị flood lớn sang cell cách ly | C | MR | **ZonePlacement** |
| F3.3.5 | DDoS L3/L4 qua hạ tầng hoặc đối tác | E | MR | — |
| **F3.4** | **Rate limit** | | | |
| F3.4.1 | Rate limit theo IP, đường dẫn, phương thức, toàn fleet | E | MVP | RateLimitRule (đọc) |
| F3.4.2 | Rate limit theo header, cookie, API key, JWT claim (consumer API) | E | MVP | RateLimitRule (đọc) |
| F3.4.3 | Rate limit theo method gRPC | E | MR | RateLimitRule (đọc) |
| **F3.5** | **Managed ruleset và custom rule** | | | |
| F3.5.1 | Kiểm theo CRS và system ruleset, paranoia level, ngưỡng anomaly | E | MVP | RulesetRelease, ManagedRulesetConfig (đọc) |
| F3.5.2 | Kiểm body JSON, XML, multipart, form trong giới hạn kích thước của gói | E | MVP | Entitlements (đọc) |
| F3.5.3 | Đánh giá custom rule theo thứ tự ưu tiên | E | MVP | CustomRule (đọc) |
| F3.5.4 | Áp exception có phạm vi (rule, trường, đường dẫn) | E | MVP | Exception (đọc) |
| F3.5.5 | Kiểm WebSocket (bắt tay; frame ở mức NC), gRPC theo method | E | MR | — |
| **F3.6** | **Bot management** | | | |
| F3.6.1 | Chặn bot xấu rõ ràng, nhận diện bot đã xác minh | E | MVP | Verified bot list (đọc) |
| F3.6.2 | Tính điểm bot và áp policy theo đường dẫn | E | MR | BotPolicy (đọc) |
| F3.6.3 | Chống chiếm tài khoản (credential stuffing) | E | NC | — |
| **F3.7** | **API security** | | | |
| F3.7.1 | Kiểm request theo schema OpenAPI của endpoint | E | MR | ApiSchema (đọc) |
| F3.7.2 | Xác thực JWT (JWKS của khách), mTLS client | E | MR | ApiAuthConfig (đọc) |
| F3.7.3 | Ghi nhận endpoint để discovery | E | MR | Telemetry |
| F3.7.4 | Phát hiện BOLA, dữ liệu nhạy cảm trong response | E | NC | — |
| **F3.8** | **Thực thi hành động** | | | |
| F3.8.1 | Block, challenge (JS, tương tác, managed: edge tự chọn theo điểm), log, allow, trust, skip | E | MVP | — |
| F3.8.2 | Trang lỗi tuỳ biến theo tenant hoặc zone, map với từng loại lỗi (403 WAF, 429, 502, 504, 503 bảo trì, trang challenge), JSON cho path API | E | MR | **CustomErrorPage** (đọc) |
| F3.8.3 | Phát security event cho mọi quyết định không phải allow | E | MVP | **SecurityEvent** |
| F3.8.4 | Thực thi trần cứng của gói (chuyển tiếp tối giản, hạ cấp) | E | MVP | Entitlements (đọc) |
| F3.8.5 | Tarpit: giữ kết nối vài giây trước khi trả 403; có trần số kết nối tarpit trên mỗi node, vượt trần thì block | E | MR | — |
| **F3.9** | **Bảo vệ origin và fail mode** | | | |
| F3.9.1 | Fail-open hoặc fail-closed khi tầng WAF không trả lời kịp | E | MVP | ZoneSettings (đọc) |
| F3.9.2 | Xác thực edge với origin (mTLS tới origin, header bí mật) | E | MVP | ZoneSettings (đọc) |
| F3.9.3 | Phục vụ bản cache cũ khi origin lỗi | E | MVP | Cache |

## F4 Hiệu năng và phân phối

| Mã | Chức năng lá | Plane | Mức | Dữ liệu |
|---|---|---|---|---|
| **F4.1** | **Cache** | | | |
| F4.1.1 | Cache theo Cache-Control, stale-if-error | E | MVP | Cache |
| F4.1.2 | Cache rule của khách: cache key, TTL, bỏ qua cache | M | MVP | **CacheRule** |
| F4.1.3 | Tiered cache, origin shield | E | MR | — |
| **F4.2** | **Purge** | | | |
| F4.2.1 | Purge toàn zone, theo URL, theo prefix | M | MVP | **PurgeRequest** |
| F4.2.2 | Purge theo tag | M | MR | **PurgeRequest** |
| **F4.3** | **DNS điều hướng** | | | |
| F4.3.1 | Trả IP PoP gần và khoẻ cho hostname của khách | E | MVP | PoP health (đọc) |
| F4.3.2 | DNS authoritative cho khách: bản ghi proxied, DNSSEC | M | MR | **DnsRecord** |
| **F4.4** | **Load balancing và failover** | | | |
| F4.4.1 | Nhóm origin active-passive, round-robin, theo trọng số; health check chủ động và thụ động; chuyển sang origin dự phòng khi lỗi | M | MR | **OriginPool**, **HealthCheck** |

## F5 Vòng đời policy

Mọi thay đổi ảnh hưởng tới traffic đi qua nhóm này. Persona chính: quản trị bảo mật, SOC analyst, DevOps.

| Mã | Chức năng lá | Plane | Mức | Dữ liệu |
|---|---|---|---|---|
| **F5.1** | **Soạn policy** | | | |
| F5.1.1 | Soạn custom rule bằng builder hoặc biểu thức text | M | MVP | **CustomRule** (trong **PolicyDraft**) |
| F5.1.2 | Cấu hình managed ruleset: profile, paranoia, ngưỡng, rule tắt | M | MVP | **ManagedRulesetConfig** |
| F5.1.3 | Soạn rate limit rule, access list | M | MVP | **RateLimitRule**, **AccessList** |
| F5.1.4 | Tạo exception có phạm vi, có hạn | M | MVP | **Exception** |
| F5.1.5 | Cấu hình DDoS L7, Under Attack mode, bot, API security của zone | M | MVP | **DdosSettings**, **BotPolicy**, **ApiSchema** |
| F5.1.6 | Bật log-only theo rule hoặc theo zone | M | MVP | thuộc tính của rule, **ZoneSettings** |
| F5.1.7 | Kế thừa policy org → nhóm zone → zone | M | MR | **PolicyTemplate**, **PolicyBinding** |
| F5.1.8 | Duyệt và xác nhận endpoint API từ discovery, tải OpenAPI | M | MR | **ApiEndpoint**, **ApiSchema** |
| **F5.2** | **Kiểm tra và mô phỏng** | | | |
| F5.2.1 | Kiểm cú pháp, kiểu, giới hạn gói ngay khi soạn | M | MVP | — |
| F5.2.2 | Biên dịch thử trước khi gửi duyệt (cùng checker với Control) | C | MVP | **CompileResult** |
| F5.2.3 | Mô phỏng trên traffic lịch sử, báo số request bị ảnh hưởng | A | MR | **ReplayJob**, **ReplayResult** |
| F5.2.4 | Kiểm tra không lưu (validate, dry-run) qua public API cho Terraform plan và CI; không tạo PolicyVersion | C | MVP | — |
| **F5.3** | **Duyệt và phát hành** | | | |
| F5.3.1 | Gửi change request kèm diff và kết quả kiểm tra | M | MVP | **ChangeRequest** |
| F5.3.2 | Duyệt 4-eyes (người duyệt khác người tạo), bật theo account | M | MR | **Approval** |
| F5.3.3 | Khung giờ được phép thay đổi | M | MR | **ChangeWindow** |
| F5.3.4 | Phát hành: tạo PolicyVersion bất biến, gửi sang Control | M | MVP | **PolicyVersion**, Outbox |
| F5.3.5 | Theo dõi trạng thái phát hành tới edge realtime trên portal (đã biên dịch, % node đã áp), lý do khi dừng hoặc rollback | C | MVP | **RolloutStatus** |
| **F5.4** | **Version và rollback** | | | |
| F5.4.1 | Xem lịch sử version của zone, ai duyệt, khi nào | M | MVP | PolicyVersion |
| F5.4.2 | So sánh hai version | M | MVP | PolicyVersion |
| F5.4.3 | Rollback một bấm về version bất kỳ còn giữ | M | MVP | **PolicyVersion** (version mới từ nội dung cũ) |
| **F5.5** | **Kênh khẩn** | | | |
| F5.5.1 | Bật Under Attack mode không cần duyệt | M | MVP | **ZoneSettings**, AuditEntry |
| F5.5.2 | Rule tạm có hạn từ case hoặc từ SOC, tự hết hạn | M | MVP | **TemporaryMitigation** |
| F5.5.3 | Emergency override ở account cần duyệt: áp ngay thay đổi siết chặt, có hạn, duyệt bù, cảnh báo critical tới Owner và Admin | M | MR | **ChangeRequest**.emergency |
| **F5.6** | **Policy as code** | | | |
| F5.6.1 | Toàn bộ F5.1–F5.4 qua public API | M | MVP | — |
| F5.6.2 | Terraform provider | M | MR | — |
| F5.6.3 | Đồng bộ từ Git (GitOps) | M | MR | **GitSource** |
| **F5.7** | **Cấu hình nhiều tầng** ([3.14](03-business-analysis/14-config-tiers-attack-history-thresholds.md)) | | | |
| F5.7.1 | Tầng system: bộ rule managed mặc định cho mọi website; cập nhật phát toàn fleet theo canary | C | MVP | **RulesetRelease** |
| F5.7.2 | Tầng global: cấu hình chung và tập rule custom của NCC, gán cho domain, cluster hoặc node | M | MR | **RuleSet**, **RuleSetAssignment** |
| F5.7.3 | Tầng website: rule của riêng website, số lượng theo gói | M | MVP | CustomRule, Entitlements |
| F5.7.4 | Ưu tiên giữa tầng: tầng trên ghi đè tầng dưới; whitelist ưu tiên theo OQ-163 | C | MVP | — |
| F5.7.5 | Hiện cho khách rule nào áp lên website của họ và đến từ tầng nào | M | MVP | — |

## F6 Phân phối cấu hình và fleet

Không có persona khách; NOC/SRE xem và can thiệp.

| Mã | Chức năng lá | Plane | Mức | Dữ liệu |
|---|---|---|---|---|
| **F6.1** | **Biên dịch** | | | |
| F6.1.1 | Biên dịch PolicyVersion và cấu hình zone thành bundle của zone | C | MVP | **ZoneBundle** |
| F6.1.2 | Từ chối bundle vượt ngân sách (kích thước, độ phức tạp) | C | MVP | **CompileResult** |
| F6.1.3 | Ký bundle, dựng manifest | C | MVP | **Manifest** |
| **F6.2** | **Phân phối** | | | |
| F6.2.1 | Đẩy bundle tới PoP store, node nạp khi cần | C | MVP | PoP store |
| F6.2.2 | Đường nhanh cho lớp cấu hình khẩn (Under Attack, rule khẩn, purge) | C | MVP | — |
| F6.2.3 | Node mới hoặc nối lại tự đồng bộ tới manifest mới nhất | E | MVP | — |
| **F6.3** | **Rollout an toàn** | | | |
| F6.3.1 | Canary theo cell, gate theo chỉ số, tự rollback | C | MVP | **Rollout** |
| F6.3.2 | Rollout bản edge và ruleset theo wave | C | MVP | **Rollout** |
| **F6.4** | **Fleet và danh tính node** | | | |
| F6.4.1 | Đăng ký node, cấp cert node 24 giờ, gia hạn tự động | C | MVP | **Node**, **NodeCert** |
| F6.4.2 | Thu hồi node | C | MVP | **NodeDenyList** |
| F6.4.3 | Biết node nào chạy bản edge nào, manifest nào | C | MVP | **Node**.state |
| **F6.5** | **Đặt zone vào cell** | | | |
| F6.5.1 | Chọn pool cell theo gói, chuyển khi đổi gói | C | MVP | **ZonePlacement** |
| F6.5.2 | Cân bằng lại cell, cách ly zone ồn | C | MR | **ZonePlacement** |

## F7 Quan sát và điều tra

Persona chính: SOC analyst, quản trị bảo mật; support của NCC với quyền giới hạn.

| Mã | Chức năng lá | Plane | Mức | Dữ liệu |
|---|---|---|---|---|
| **F7.1** | **Thu nhận telemetry** | | | |
| F7.1.1 | Nhận security event đầy đủ, không mất | A | MVP | **SecurityEvent** |
| F7.1.2 | Nhận access log lấy mẫu theo gói | A | MVP | **AccessLog** |
| F7.1.3 | Nhận mẫu request cho replay (gói cao) | A | MR | **RequestSample** |
| F7.1.4 | Đối soát số bản ghi gửi và nhận | A | MVP | Bộ đếm đối soát |
| **F7.2** | **Xem realtime** | | | |
| F7.2.1 | Luồng security event trực tiếp theo zone | A | MVP | — (live tap) |
| F7.2.2 | Thông báo trong portal (cảnh báo, kết quả phát hành) | A | MVP | — |
| F7.2.3 | Live tail có bộ lọc (rule ID, IP, path, hành động), gồm cả hit của rule log-only | A | MVP | — (live tap) |
| **F7.3** | **Tìm kiếm và tra cứu** | | | |
| F7.3.1 | Tra cứu theo request ID: rule nào, bước nào, version nào | A | MVP | SecurityEvent, AccessLog |
| F7.3.2 | Lọc, nhóm event theo trường, khoảng thời gian | A | MVP | SecurityEvent |
| F7.3.3 | Lưu truy vấn, chia sẻ trong account | M | MVP | **SavedSearch** |
| **F7.4** | **Dashboard** | | | |
| F7.4.1 | Dashboard traffic, bảo mật, cache, origin | A | MVP | Bảng tổng hợp |
| F7.4.2 | Hiệu quả rule: số khớp, hành động, theo thời gian | A | MVP | Bảng tổng hợp |
| F7.4.3 | Phân bố bot, endpoint API | A | MR | Bảng tổng hợp |
| **F7.5** | **Chặn nhầm và hiệu quả rule** | | | |
| F7.5.1 | Đánh dấu một event là chặn nhầm | M | MVP | **FalsePositiveLabel** |
| F7.5.2 | Tỉ lệ chặn nhầm theo rule | A | MVP | Bảng tổng hợp |
| F7.5.3 | Từ event đánh dấu, tạo nháp exception | M | MVP | **Exception** (nháp) |
| **F7.6** | **Export SIEM** | | | |
| F7.6.1 | Cấu hình đích export (Splunk, S3, HTTP, Kafka, syslog) | M | MVP | **ExportDestination** |
| F7.6.2 | Đẩy event liên tục, thử lại, báo đích lỗi | A | MVP | Trạng thái export |
| F7.6.3 | Bộ lọc export theo hành động, loại event, zone, trường; lấy mẫu log allow | A | MVP | **ExportDestination** |
| **F7.7** | **Lịch sử tấn công** | | | |
| F7.7.1 | Gom event thành một bản ghi cho mỗi đợt tấn công, hai loại: DDoS L7 và vuln-scan | A | MVP | **AttackEpisode** |
| F7.7.2 | Xem danh sách, chi tiết đợt tấn công (bắt đầu, đỉnh, nguồn, biện pháp đã áp) theo quyền và gói | A | MVP | AttackEpisode |
| F7.7.3 | Giữ lịch sử theo gói; nhân viên NCC có quyền xem, có audit | A | MVP | AttackEpisode |
| **F7.8** | **Event WAF** | | | |
| F7.8.1 | Màn riêng: danh sách event, chi tiết, rule vi phạm | A | MVP | SecurityEvent |
| F7.8.2 | Tuning chặn nhầm ngay từ event (nối F7.5) | M | MVP | **FalsePositiveLabel**, **Exception** |

## F8 Phát hiện, cảnh báo, sự cố

| Mã | Chức năng lá | Plane | Mức | Dữ liệu |
|---|---|---|---|---|
| **F8.1** | **Phát hiện** | | | |
| F8.1.1 | Đánh giá alert template (W, D, O, BOT, API) theo cửa sổ | A | MVP | **AlertState** |
| F8.1.2 | Phát hiện bất thường theo baseline | A | NC | Baseline |
| F8.1.3 | Đề xuất mở case và mitigation | A | MR | **CaseProposal** |
| **F8.2** | **Alert policy** | | | |
| F8.2.1 | Tạo alert policy: điều kiện, phạm vi zone, mức | M | MVP | **AlertPolicy** |
| F8.2.2 | Kênh thông báo: email, webhook, Slack, Teams, PagerDuty, SMS | M | MVP | **NotificationChannel** |
| F8.2.3 | Giờ im lặng, chu kỳ lặp, gộp cảnh báo | M | MVP | **AlertPolicy** |
| F8.2.4 | Gửi thử kênh | M | MVP | — |
| F8.2.5 | Ngưỡng cảnh báo theo từng loại tấn công, ba mức low, medium, high, có mặc định; đặt ở tầng zone | M | MVP | **AlertThreshold** |
| F8.2.6 | Template nội dung thông báo: mẫu mặc định, khách tuỳ chỉnh | M | MVP | **NotificationTemplate** |
| F8.2.7 | Định tuyến thông báo theo mức và vai trò; leo thang khi mức high không ack | M | MVP | **AlertPolicy** |
| **F8.3** | **Gửi thông báo** | | | |
| F8.3.1 | Gửi theo kênh, thử lại, ghi kết quả | A | MVP | **NotificationDelivery** |
| F8.3.2 | Chống bão: dedup theo alert, zone, nguồn; gom vào một thread hoặc digest; mức tăng hoặc nguồn mới thì gửi tin mới | A | MVP | **NotificationDelivery** |
| F8.3.3 | Thông báo kèm link xử lý một bấm: mở portal, đăng nhập, xác nhận mitigation đề xuất | A | MR | **CaseProposal** |
| **F8.4** | **Case và sự cố** | | | |
| F8.4.1 | Mở case tay hoặc từ đề xuất | M | MR | **Case** |
| F8.4.2 | Gắn event, truy vấn, mitigation vào case | M | MR | **CaseLink** |
| F8.4.3 | Workflow case: mở → tiếp nhận → giảm thiểu → đóng, người phụ trách, SLA | M | MR | **Case**.status |
| F8.4.4 | Đóng case kèm tóm tắt | M | MR | **Case** |

## F9 Báo cáo và tuân thủ

| Mã | Chức năng lá | Plane | Mức | Dữ liệu |
|---|---|---|---|---|
| **F9.1** | **Báo cáo định kỳ** | | | |
| F9.1.1 | Định nghĩa báo cáo, lịch gửi, người nhận | M | MR | **ReportDefinition** |
| F9.1.2 | Chạy báo cáo, xuất PDF/CSV | M | MR | **ReportRun** |
| **F9.2** | **Báo cáo tuân thủ** | | | |
| F9.2.1 | Báo cáo theo PCI DSS 6.4, SOC 2 (thay đổi, quyền, retention) | M | MR | ReportRun |
| F9.2.2 | Báo cáo SLA của NCC theo zone | A | MR | Bảng tổng hợp |
| **F9.3** | **Retention và archive** | | | |
| F9.3.1 | Giữ log theo thời hạn của gói, xoá khi hết hạn | A | MVP | SecurityEvent, AccessLog |
| F9.3.2 | Archive audit log, không xoá | M | MVP | AuditEntry archive |
| F9.3.3 | Xoá dữ liệu cá nhân theo yêu cầu | M | MVP | Member, User |
| **F9.4** | **Data residency** | | | |
| F9.4.1 | Log của account nằm trong region đã chọn | A | NC | — |
| F9.4.2 | Cấu hình của tenant Enterprise nằm trong region đã chọn | M | NC | Tenant placement |

## F10 Gói và billing

Persona chính: chủ tài khoản, bộ phận tài chính của khách; sales của NCC.

| Mã | Chức năng lá | Plane | Mức | Dữ liệu |
|---|---|---|---|---|
| **F10.1** | **Danh mục gói** | | | |
| F10.1.1 | NCC định nghĩa gói, giá, hạn mức, đơn giá vượt, có version | M | MVP | **Plan**, **PlanVersion**, **Price** |
| F10.1.2 | Gói riêng cho hợp đồng Enterprise | M | MVP | **Plan** (riêng tenant) |
| F10.1.3 | Add-on (zone phụ, tính năng lẻ) | M | MR | **AddOn** |
| **F10.2** | **Subscription** | | | |
| F10.2.1 | Dùng thử 14 ngày, giới hạn số lượt dùng thử mỗi khách | M | MVP | **Subscription**, **TrialClaim** (toàn cục) |
| F10.2.2 | Mua, nâng, hạ gói theo zone hoặc account | M | MVP | **Subscription** |
| F10.2.3 | Quá hạn thanh toán: chuyển tiếp tối giản 30 ngày rồi tạm dừng | M | MVP | **Subscription**.status |
| **F10.3** | **Entitlements** | | | |
| F10.3.1 | Tính entitlements của zone từ gói, add-on, hợp đồng | M | MVP | **Entitlements** |
| F10.3.2 | Phát entitlements mới khi đổi gói, có hiệu lực ≤ 5 phút | M | MVP | Outbox |
| F10.3.3 | Chặn tạo vượt giới hạn gói (số rule, số đích export, …) | M | MVP | Entitlements (đọc) |
| **F10.4** | **Đo usage** | | | |
| F10.4.1 | Đếm băng thông sạch, request theo zone tại edge | E | MVP | Bộ đếm |
| F10.4.2 | Gộp theo giờ, ngày, kỳ; đối soát với node | A | MVP | **UsageCounter** |
| F10.4.3 | Nhận usage vào Management để tính tiền | M | MVP | **UsageDaily** |
| **F10.5** | **Hạn mức và trần cứng** | | | |
| F10.5.1 | Cảnh báo 80% và 100% hạn mức | M | MVP | **QuotaNotice** |
| F10.5.2 | Gói có trần: phát lệnh áp trần khi vượt | M | MVP | **Entitlements**.capState |
| F10.5.3 | Khách đặt trần chi tiêu; xử lý phần vượt quota chờ phân tích ([3.5](03-business-analysis/05-plans-billing.md#3-đã-quyết)) | M | MVP | **SpendingCap** |
| **F10.6** | **Hoá đơn và thanh toán** | | | |
| F10.6.1 | Lập hoá đơn cuối kỳ: phí nền, phí vượt, add-on, thuế | M | MVP | **Invoice**, **InvoiceLine** |
| F10.6.2 | Thanh toán qua cổng, lưu phương thức (token của cổng) | M | MVP | **PaymentMethod**, **Payment** |
| F10.6.3 | Xuất hoá đơn điện tử theo quy định | M | MVP | **Invoice** |
| F10.6.4 | Điều chỉnh, hoàn tiền, ghi có | M | MR | **CreditNote** |

## F11 Threat intelligence

Persona chính: threat research của NCC.

| Mã | Chức năng lá | Plane | Mức | Dữ liệu |
|---|---|---|---|---|
| F11.1.1 | Phát hành system ruleset và bản CRS có version | C | MVP | **RulesetRelease** |
| F11.1.2 | Rollout ruleset theo wave, log-only trước khi chặn | C | MVP | **Rollout** |
| F11.2.1 | Rule khẩn toàn fleet cho CVE mới, ≤ 15 s tới 100% node | C | MVP | **EmergencyRule** |
| F11.3.1 | Nhập và cập nhật feed IP reputation | C | MR | **ReputationFeed** |
| F11.4.1 | Virtual patching theo CVE: gợi ý rule cho zone có công nghệ bị ảnh hưởng | C | MR | **VirtualPatch** |
| F11.4.2 | Tín hiệu chéo tenant ẩn danh | A | NC | — |

## F12 Vận hành NCC

Persona chính: NOC/SRE, support, product. Chi tiết vận hành ở [8.12](../architecture/08-cross-cutting/12-platform-monitoring.md),
[8.13](../architecture/08-cross-cutting/13-operational-behaviors.md).

| Mã | Chức năng lá | Plane | Mức | Dữ liệu |
|---|---|---|---|---|
| F12.1.1 | Giám sát SLI, probe tổng hợp, lan truyền cấu hình | mọi plane | MVP | Metric |
| F12.1.2 | Giám sát dung lượng PoP, cell, chống noisy neighbor | C | MR | Metric |
| F12.2.1 | Rút PoP khỏi DNS, chuyển traffic | C | MVP | **PopState** |
| F12.2.2 | Đóng băng phát hành toàn fleet | C | MVP | **FleetFreeze** |
| F12.3.1 | Support tra request ID của khách không cần đọc toàn bộ tenant | A | MVP | **SupportAccessGrant** |
| F12.3.2 | Support truy cập tenant khi khách cho phép, có hạn, có audit | M | MVP | **SupportAccessGrant** |
| F12.4.1 | Quản trị org: xem, tạm khoá, mở khoá tenant vi phạm | M | MVP | **Org**.status |
| F12.4.2 | Quản lý danh mục gói, đối tác hạ tầng, PoP | M | MVP | Plan, **Partner**, **Pop** |
| F12.4.3 | Bật mitigation trên zone của khách từ portal vận hành của NCC ([3.3](03-business-analysis/03-attack-response.md#3-đã-quyết)) | M | MVP | **ZoneSettings**, **TemporaryMitigation**, AuditEntry |
| F12.4.4 | Đặt log-only có hạn cho một rule khẩn của NCC trên một zone bị chặn nhầm, qua support, có audit | M | MVP | **EmergencyRuleOverride**, AuditEntry |

## Phân rã theo plane

Các bảng F1–F12 ở trên chia chức năng theo **nghiệp vụ**. Mục này lấy lại đúng các chức năng lá đó và chia
theo **plane chủ**, rồi theo **thành phần** trong plane ([5.8](../architecture/05-building-blocks/08-components/00-index.md)).
Không có chức năng mới: mỗi lá xuất hiện đúng một lần ở đây. Cột **Liên kết** trỏ tới bảng
[liên kết giữa các plane](#liên-kết-giữa-các-plane) bên dưới.

```mermaid
flowchart TB
    M["<b>Management</b> · nơi người dùng tạo ra cấu hình<br/>PM1 Định danh và quyền<br/>PM2 Zone và hiệu năng<br/>PM3 Policy và thay đổi<br/>PM4 Vận hành bảo mật của khách<br/>PM5 Thương mại<br/>PM6 Dữ liệu và tuân thủ<br/>PM7 Quản trị NCC"]
    C["<b>Control</b> · biến cấu hình thành snapshot an toàn<br/>PC1 Biên dịch<br/>PC2 Phân phối<br/>PC3 Rollout<br/>PC4 Fleet và danh tính node<br/>PC5 Đặt zone vào cell<br/>PC6 Xác minh domain và cert<br/>PC7 Threat intel và ruleset<br/>PC8 Điều khiển fleet"]
    E["<b>Edge</b> · xử lý từng request<br/>PE1 Tầng L7<br/>PE2 Tầng WAF inspection<br/>PE3 Edge agent<br/>PE4 DNS điều hướng"]
    A["<b>Analytics</b> · biến telemetry thành bằng chứng<br/>PA1 Thu nhận<br/>PA2 Lưu trữ và retention<br/>PA3 Query API<br/>PA4 Realtime gateway<br/>PA5 Detection<br/>PA6 Thông báo và export<br/>PA7 Replay<br/>PA8 Usage"]
    M ==>|"cấu hình có version"| C
    C ==>|"snapshot ký"| E
    E ==>|"telemetry"| A
    E -->|"trạng thái node"| C
    A -->|"cảnh báo, usage"| M
    A -->|"tín hiệu rollout, cách ly"| C
    M <-->|"truy vấn, realtime"| A
    style M text-align:left
    style C text-align:left
    style E text-align:left
    style A text-align:left
```

### Management

| Thành phần | Chức năng lá | Liên kết |
|---|---|---|
| **PM1 Định danh và quyền** | F1.1.1–F1.1.4, F1.2.1–F1.2.6, F1.3.1–F1.3.4, F1.4.1–F1.4.3, F1.5.1–F1.5.3 | LK-19 (token realtime, đổi quyền) |
| **PM2 Zone và hiệu năng** | F2.1.1, F2.1.3, F2.1.8, F2.2.1, F2.2.2, F2.2.4, F2.2.5, F2.2.7, F2.3.3, F2.3.4, F2.4.1–F2.4.4, F2.5.1, F2.5.2, F2.6.1, F2.6.3, F2.7.1, F2.7.2, F2.7.4–F2.7.6, F4.1.2, F4.2.1, F4.2.2, F4.3.2, F4.4.1 | LK-2, LK-3, LK-18, LK-24, LK-28 |
| **PM3 Policy và thay đổi** | F5.1.1–F5.1.8, F5.2.1, F5.3.1–F5.3.4, F5.4.1–F5.4.3, F5.5.1, F5.5.2, F5.6.1–F5.6.3 | LK-1, LK-6, LK-7, LK-22 |
| **PM4 Vận hành bảo mật của khách** | F7.3.3, F7.5.1, F7.5.3, F7.6.1, F8.2.1–F8.2.4, F8.4.1–F8.4.4, F9.1.1, F9.1.2, F9.2.1 | LK-18, LK-20, LK-21 |
| **PM5 Thương mại** | F10.1.1–F10.1.3, F10.2.1–F10.2.3, F10.3.1–F10.3.3, F10.4.3, F10.5.1–F10.5.3, F10.6.1–F10.6.4 | LK-4, LK-5, LK-17 |
| **PM6 Dữ liệu và tuân thủ** | F9.3.2, F9.3.3, F9.4.2 | LK-23 |
| **PM7 Quản trị NCC** | F12.3.2, F12.4.1–F12.4.3 | LK-5 (khoá org đổi entitlements) |

### Control

| Thành phần | Chức năng lá | Liên kết |
|---|---|---|
| **PC1 Biên dịch** | F5.2.2, F6.1.1, F6.1.2, F6.1.3 | LK-1, LK-2, LK-3, LK-5, LK-6 vào; LK-7, LK-8 ra |
| **PC2 Phân phối** | F6.2.1, F6.2.2 | LK-3 vào; LK-8, LK-9 ra |
| **PC3 Rollout** | F5.3.5, F6.3.1, F6.3.2, F11.1.2 | LK-7 ra; LK-12, LK-16 vào |
| **PC4 Fleet và danh tính node** | F6.4.1, F6.4.2, F6.4.3 | LK-10, LK-11, LK-12 |
| **PC5 Đặt zone vào cell** | F3.3.4, F6.5.1, F6.5.2, F12.1.2 | LK-5 vào; LK-25 vào |
| **PC6 Xác minh domain và cert** | F2.1.2, F2.1.4–F2.1.7, F2.2.3, F2.2.6, F2.3.1, F2.3.2, F2.3.6 | LK-2 vào; LK-8, LK-24, LK-28 ra; LK-27 |
| **PC7 Threat intel và ruleset** | F11.1.1, F11.2.1, F11.3.1, F11.4.1 | LK-8 ra; LK-26 vào |
| **PC8 Điều khiển fleet** | F12.2.1, F12.2.2 | LK-13 ra |

### Edge

| Thành phần | Chức năng lá | Liên kết |
|---|---|---|
| **PE1 Tầng L7** | F2.3.5, F2.6.2, F3.1.1–F3.1.4, F3.2.1–F3.2.3, F3.3.1–F3.3.3, F3.3.5, F3.4.1, F3.4.2, F3.6.1, F3.6.2, F3.7.1, F3.7.2, F3.8.1–F3.8.4, F3.9.1–F3.9.3, F4.1.1, F4.1.3 | LK-14 ra (tới PE2) |
| **PE2 Tầng WAF inspection** | F3.5.1–F3.5.5, F3.6.3, F3.7.3, F3.7.4 | LK-14 vào (từ PE1) |
| **PE3 Edge agent** | F6.2.3, F10.4.1; gửi telemetry do PE1, PE2 sinh ra (F3.8.3); chạy `OriginProbe` | LK-8, LK-9, LK-10, LK-27 vào; LK-12, LK-15 ra |
| **PE4 DNS điều hướng** | F4.3.1 | LK-13 vào |

Phân chia F3 giữa PE1 và PE2 theo [ADR-0033](../architecture/09-adr/0033-two-tier-edge-proxy-and-waf.md): việc rẻ, cần
trạng thái kết nối hoặc trạng thái toàn fleet (rate limit, DDoS, bot score, JWT) ở L7; việc tốn CPU trên
nội dung (managed ruleset, body, schema body) ở WAF. Chi tiết chia còn phụ thuộc kết quả spike ở
[ADR-0031](../architecture/09-adr/0031-edge-l7-and-waf-engine-options.md).

### Analytics

| Thành phần | Chức năng lá | Liên kết |
|---|---|---|
| **PA1 Thu nhận** | F7.1.1–F7.1.4 | LK-4, LK-15 vào |
| **PA2 Lưu trữ và retention** | F9.3.1, F9.4.1 | LK-4 vào; LK-23 vào |
| **PA3 Query API** | F2.7.3, F7.3.1, F7.3.2, F7.4.1–F7.4.3, F7.5.2, F9.2.2, F12.3.1 | LK-18 vào |
| **PA4 Realtime gateway** | F7.2.1, F7.2.2 | LK-19 vào |
| **PA5 Detection** | F8.1.1–F8.1.3, F11.4.2 | LK-20 vào; LK-21, LK-25, LK-26 ra |
| **PA6 Thông báo và export** | F7.6.2, F8.3.1 | LK-20 vào |
| **PA7 Replay** | F5.2.3 | LK-22 |
| **PA8 Usage** | F10.4.2 | LK-15 vào; LK-17 ra |

Chức năng F12.1.1 (giám sát SLI, probe) nằm ở **mọi plane** và gửi về hệ thống giám sát của NCC
([8.12](../architecture/08-cross-cutting/12-platform-monitoring.md)), không đi qua liên kết giữa các plane.

## Liên kết giữa các plane

Mỗi liên kết là một chỗ dữ liệu hoặc lệnh đi từ chức năng của plane này sang chức năng của plane khác.
Tên hợp đồng lấy từ [5.5](../architecture/05-building-blocks/05-contracts.md). Hợp đồng đánh dấu **(tạm)** nằm ở mục
[hợp đồng bổ sung](../architecture/05-building-blocks/05-contracts.md#hợp-đồng-bổ-sung-tạm-2026-11-06) của 5.5: bản đồ tìm ra
liên kết, hợp đồng đã được thêm tạm và có thể đổi khi phân tích nghiệp vụ ([requirements §3](03-business-analysis/00-index.md)).

Kiểu giao tiếp:

- **Outbox**: ghi cùng transaction với dữ liệu nghiệp vụ, qua RabbitMQ, at-least-once, bên nhận bỏ trùng.
- **Sync**: gọi đồng bộ, có timeout; bên gọi phải chịu được khi bên kia không trả lời.
- **Pull**: bên nhận tự kéo theo chu kỳ; mất thông báo đẩy cũng không sai.
- **Stream**: luồng liên tục có buffer.

```mermaid
flowchart LR
    M["<b>Management</b>"]
    C["<b>Control</b>"]
    E["<b>Edge</b><br/>PE1 L7 → PE2 WAF: LK-14"]
    A["<b>Analytics</b>"]
    M -->|"LK-1…LK-6<br/>PolicyVersionPublished, ZoneChanged,<br/>EntitlementsChanged, biên dịch thử, PurgeRequested*"| C
    C -->|"LK-7, LK-24, LK-28<br/>CompileResult*, RolloutProgress*,<br/>CertStatusChanged*, ZoneCheckResult*"| M
    C -->|"LK-8…LK-11, LK-13, LK-27<br/>Snapshot, Announcement, NodeProfile,<br/>deny list, PopState*, OriginProbe*"| E
    E -->|"LK-12<br/>NodeStatus"| C
    E -->|"LK-15<br/>TelemetryRecord, UsageCounter"| A
    A -->|"LK-16, LK-25, LK-26<br/>RolloutSignal, IsolationRequested*,<br/>ReputationBatch*"| C
    A -->|"LK-17, LK-21<br/>UsageReport, AlertRaised, CaseProposed"| M
    M -->|"LK-4, LK-18…LK-20, LK-22, LK-23<br/>EntitlementsChanged, Query, PermissionsChanged*,<br/>AlertPolicyChanged*, ExportDestinationChanged*,<br/>FalsePositiveLabeled*, Replay*, DataErasure*"| A
```

`*` là hợp đồng tạm ở 5.5. Không có cạnh Management ↔ Edge và Analytics → Edge, có chủ đích (giải thích dưới bảng).

| ID | Từ → tới | Chức năng phát → chức năng nhận | Hợp đồng | Kiểu | Khi bên nhận hỏng |
|---|---|---|---|---|---|
| **Management → Control** | | | | | |
| LK-1 | PM3 → PC1 | F5.3.4 phát hành → F6.1.1 biên dịch | `PolicyVersionPublished` | Outbox | Edge giữ snapshot cũ; portal hiện "chờ biên dịch"; outbox giữ tới khi Control nhận |
| LK-2 | PM2 → PC1, PC6 | F2.1–F2.4 đổi zone, hostname, origin, yêu cầu cert → F6.1.1, F2.3.1 | `ZoneChanged` | Outbox | Như LK-1; zone mới chưa nhận traffic, zone cũ không đổi |
| LK-3 | PM2 → PC1, PC2 | F4.1.2 cache rule → F6.1.1; F4.2.1, F4.2.2 purge → F6.2.2 đường nhanh | Cache rule nằm trong `ZoneChanged`; purge là `PurgeRequested` (tạm) | Outbox | Purge trễ; bản cũ vẫn được phục vụ tới khi purge tới (QS-15 vỡ) |
| LK-4 | PM5 → PA2, PA1 | F10.3.2 → F7.1.2 tỉ lệ lấy mẫu, F9.3.1 retention | `EntitlementsChanged` | Outbox | Analytics dùng entitlements cũ tới khi nhận |
| LK-5 | PM5, PM7 → PC5, PC1 | F10.3.2, F12.4.1 → F6.5.1 đổi pool cell; giới hạn gói vào bundle (F3.8.4) | `EntitlementsChanged` | Outbox | Zone ở pool cũ, áp giới hạn cũ; QS-14 vỡ |
| LK-6 | PM3 → PC1 | F5.2.1 → F5.2.2 biên dịch thử | Control internal API | Sync | Không gửi duyệt được; portal báo Control không sẵn sàng, bản nháp vẫn lưu |
| **Control → Management** | | | | | |
| LK-7 | PC1, PC3 → PM3 | F6.1.2 kết quả biên dịch, F6.3.1 trạng thái rollout → F5.3.5 | `CompileResult`, `RolloutProgress` (tạm); đọc lại qua Control internal API | Outbox + sync (đọc) | Portal không hiện tiến độ; phát hành vẫn chạy |
| LK-24 | PC6 → PM2 | F2.3.1, F2.3.2 trạng thái cert, gia hạn lỗi → FR-2-08 cảnh báo | `CertStatusChanged` (tạm) | Outbox | Khách không thấy cert sắp hết hạn; Control vẫn tự thử lại |
| LK-28 | PC6 → PM2 | F2.1.2, F2.1.5–F2.1.7, F2.2.3, F2.2.6, F2.3.6 kết quả kiểm → tiến độ onboarding, cổng OG1 (F2.4.1, F2.7.6) | `ZoneCheckResult` (tạm) | Outbox; kiểm theo yêu cầu thì sync | Onboarding đứng ở bước kiểm; khách bấm kiểm lại (sync) |
| **Control → Edge** | | | | | |
| LK-8 | PC1, PC2, PC6, PC7 → PE3 | F6.1.3, F6.2.1 → F6.2.3 nạp bundle, cert, ruleset | `Snapshot` | Pull | Edge chạy tiếp bằng snapshot đang có (Q1) |
| LK-9 | PC2 → PE3 | F6.2.1, F6.2.2 → F6.2.3 | `SnapshotAnnouncement` | Đẩy nhẹ + pull | Agent tự kiểm định kỳ; chỉ chậm hơn |
| LK-10 | PC4 → PE3 | F6.4.1 cấp, gia hạn cert node; cấu hình node | `NodeProfile`, enroll ([8.19](../architecture/08-cross-cutting/19-node-identity-and-mtls.md)) | Sync | Node giữ cert cũ tới hạn 24 giờ; QS-39 |
| LK-11 | PC4 → mọi bên nhận | F6.4.2 thu hồi → bên nhận từ chối kết nối của node | Deny list ([8.19](../architecture/08-cross-cutting/19-node-identity-and-mtls.md)) | Pull | Node bị thu hồi còn kết nối tới khi cert hết hạn; QS-40 |
| LK-27 | PC6 ↔ PE3 | F2.2.3 kiểm origin → edge ở ≥ 2 PoP thử kết nối, trả kết quả | `OriginProbe` (tạm) | Sync qua kênh mTLS của agent | Không kiểm được origin; khách xác nhận bỏ qua để qua OG1 |
| LK-13 | PC8 → PE4 | F12.2.1 trạng thái PoP → F4.3.1 | `PopState` (tạm) | Pull | DNS trả theo trạng thái cũ; QS-16 vỡ |
| **Edge → Control** | | | | | |
| LK-12 | PE3 → PC4, PC3 | Version đang chạy, lỗi nạp, sức khoẻ → F6.4.3, F5.3.5, F6.3.1 | `NodeStatus` | Stream (heartbeat) | Control không biết node đã áp; rollout dừng ở gate, không tiến tiếp |
| **Trong Edge** | | | | | |
| LK-14 | PE1 → PE2 | F3.5.x được gọi cho từng request | Inspection API (IA1–IA13 còn mở) | Sync, có deadline | Fail mode của zone (F3.9.1); QS-18 |
| **Edge → Analytics** | | | | | |
| LK-15 | PE3 → PA1, PA8 | F3.8.3 security event, access log, mẫu request; F10.4.1 bộ đếm usage | `TelemetryRecord`, `UsageCounter` | Stream, buffer đĩa | Buffer trên node; security event và usage không bỏ; access log bỏ khi đầy (QS-10) |
| **Analytics → Control** | | | | | |
| LK-16 | PA5 → PC3 | Chỉ số theo zone, version trong cửa sổ canary → F6.3.1 gate | `RolloutSignal` | Định kỳ | Rollout dừng ở gate, không tự tiến khi thiếu tín hiệu |
| LK-25 | PA5 → PC5 | Phát hiện flood lớn → F3.3.4 chuyển sang cell cách ly | `IsolationRequested` (tạm) | RabbitMQ | Không tự cách ly; NOC làm tay theo runbook (8.13) |
| LK-26 | PA5 → PC7 | Reputation, dữ liệu bot gộp từ telemetry → F11.3.1 và dữ liệu bot ([5.8.7](../architecture/05-building-blocks/08-components/07-cert-ruleset-release.md)) | `ReputationBatch` (tạm) | Theo lô | Dùng bản reputation cũ |
| **Analytics → Management** | | | | | |
| LK-17 | PA8 → PM5 | F10.4.2 → F10.4.3, F10.5.1, F10.5.2, F10.6.1 | `UsageReport` | Định kỳ, chạy lại được | Cảnh báo hạn mức, trần cứng, hoá đơn trễ; không mất vì chạy lại được |
| LK-21 | PA5 → PM4 | F8.1.1, F8.1.3 → F8.4.1 case, F7.2.2 thông báo portal | `AlertRaised`, `CaseProposed` | At-least-once | Thông báo vẫn gửi qua PA6; case không tự mở |
| **Management → Analytics** | | | | | |
| LK-18 | PM4, PM2 → PA3 | F7.3, F7.4, F9.1.2, F12.3.1, F2.7.2 (traffic đầu tiên), F2.7.3 → Query API | `Query` | Sync, có giới hạn chi phí | Portal không có dashboard, tìm kiếm; cấu hình vẫn sửa được |
| LK-19 | PM1 → PA4 | Cấp token realtime, `PermissionsChanged` → F7.2.1 | Token JWT ([5.8.13](../architecture/05-building-blocks/08-components/13-realtime-gateway.md)) + `PermissionsChanged` (tạm) | Sync (token) + outbox | Phiên đang mở giữ tới hết hạn token; không mở phiên mới |
| LK-20 | PM4 → PA5, PA6 | F8.2.1–F8.2.3 alert policy, F7.6.1 đích export, F7.5.1 nhãn chặn nhầm | `AlertPolicyChanged`, `ExportDestinationChanged`, `FalsePositiveLabeled` (tạm) | Outbox | Analytics dùng cấu hình cũ; nhãn chặn nhầm chờ trong outbox |
| LK-22 | PM3 ↔ PA7 | F5.3.1 yêu cầu mô phỏng → F5.2.3, trả kết quả | `ReplayRequested`, `ReplayCompleted` (tạm) | Outbox hai chiều, job ID | Change request không có kết quả mô phỏng; duyệt vẫn làm được nếu không bắt buộc |
| LK-23 | PM6 → PA2 | F9.3.3 xoá dữ liệu cá nhân, F1.1.4 xoá org → xoá trong log | `DataErasureRequested`, `DataErasureCompleted` (tạm) | Outbox | Yêu cầu xoá chờ; phải có hạn hoàn tất theo Nghị định 13 |

Hai liên kết **không** tồn tại, có chủ đích:

- **Edge → Management** và **Management → Edge**: không có. Edge chỉ đọc snapshot từ Control
  ([ADR-0001](../architecture/09-adr/0001-four-planes.md)); nhờ vậy Management sập không ảnh hưởng traffic.
- **Analytics → Edge**: không có. Mọi phản ứng dựa trên phân tích (cách ly, rule khẩn, reputation) đi qua
  Control để vẫn có ký, canary và rollback.

### Hợp đồng tạm

Bản đồ cho thấy bảng chính của 5.5 mới có đường cấu hình chính và telemetry. 15 hợp đồng cho các liên kết
còn lại đã được thêm vào 5.5 ở dạng **tạm**
([5.5, hợp đồng bổ sung](../architecture/05-building-blocks/05-contracts.md#hợp-đồng-bổ-sung-tạm-2026-11-06)); `ZoneCheckResult`
và `OriginProbe` đến từ phân tích onboarding ([requirements §3.1](03-business-analysis/01-onboarding.md)). Chúng được chốt
dần khi phân tích từng nghiệp vụ ([OQ-15](02-requirements.md#9-câu-hỏi-mở-cho-review)).

## Chức năng ngoài phạm vi

Không nằm trong bản đồ, nhắc lại để không bị thêm vào thiết kế dữ liệu: vulnerability scanning (DAST, sản
phẩm riêng nếu làm), edge compute, tối ưu ảnh, video, client-side protection. Xem [1.2](../architecture/01-introduction/00-index.md#12-tổng-quan-yêu-cầu).

## Đối chiếu với thiết kế hiện có

| Phát hiện khi phân rã | Ảnh hưởng |
|---|---|
| Có đối tượng dữ liệu chưa có trong [5.8.12](../architecture/05-building-blocks/08-components/12-management.md): DnsSnapshot, OnboardingProgress, ZonePreset (từ §3.1), Invitation, MfaFactor, SsoDomain, ServiceAccount, DomainVerification, OriginCheck, ChangeWindow, TemporaryMitigation, PolicyTemplate/PolicyBinding, GitSource, QuotaNotice, PaymentMethod, Payment, CreditNote, AddOn, TrialClaim, SupportAccessGrant, Partner, Pop | Bổ sung vào module sau khi requirements chốt |
| F1.1.4, F9.3.3 (xoá org, xoá dữ liệu cá nhân) chưa có đường đi trong thiết kế database: audit không xoá nhưng có dữ liệu cá nhân | Câu hỏi mở OQ-6 ở [requirements §2](02-requirements.md#9-câu-hỏi-mở-cho-review) |
| F2.3, F6.4, F11 có dữ liệu gốc ở Control, không ở Management | Database của Control cần thiết kế riêng, không gộp vào 5.8.14 |
| F12 cần dữ liệu của NCC (Partner, Pop, gói) trong cùng database với tenant | Dùng `PLATFORM_TENANT_ID`, đã ghi ở 5.8.14 §2 |
