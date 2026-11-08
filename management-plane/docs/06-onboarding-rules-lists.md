# 06 — Onboarding website, cấu hình rule, whitelist/blacklist, domain/subdomain

Đào sâu 4 mảnh nghiệp vụ mà `01-04` mới nói lướt qua (workflow #1 ở `03-personas-workflows.md`
chỉ có 5 dòng). Tài liệu **phân tích**, chưa code — có 1 gap bảo mật thật cần chốt trước khi làm
vỏ tiếp (mục 2), và 1 quyết định kiến trúc ảnh hưởng `Zone` (mục 1). Review xong, phần "Thay đổi
cần áp dụng" ở cuối là checklist để fold vào `02`/`05`/code khi bắt đầu build.

## 1. Domain vs Subdomain — giữ nguyên model hiện tại, không làm Cloudflare-style

**Đảo ngược 2026-09-13**: quyết định "không tạo entity `Domain`" bên dưới đã bị **chủ dự án chủ
động đảo ngược** — `waf.domains` giờ là entity thật, `Zone.domainId` (Reference, required) trỏ vào
đó, tự động gán lúc tạo Zone (`zones-service/src/routes.rs::zone_domain_guard`, suy ra apex domain
từ `hostname` qua `zones-service/src/apex_domain.rs`, tìm-hoặc-tạo Domain cùng apex cho tenant đó).
Lý do đảo ngược: xác thực quyền sở hữu domain (mục 2 bên dưới) nên làm **1 lần cho cả apex domain**
thay vì lặp lại cho từng subdomain — `verificationToken`/`verificationMethod`/`verificationStatus`
chuyển hẳn từ `Zone` sang `Domain`; `Zone.verificationStatus` vẫn còn nhưng chỉ là field kỹ thuật
"mirror", được `Domain`'s verify-dns cascade cập nhật (cùng pattern `hasConfig` đã dùng), guard
`activate` ở mục 2 không đổi gì. Wildcard hostname (`*.example.com`, đề xuất bên dưới) **không**
được làm cùng đợt này — vẫn là việc chưa làm, để dành riêng nếu cần sau. Phần phân tích gốc bên
dưới giữ nguyên làm lịch sử/bối cảnh dẫn tới quyết định ban đầu, không xoá.

Cloudflare thật: **Zone = domain gốc** (`example.com`), khách delegate nameserver cho Cloudflare,
Cloudflare quản lý toàn bộ DNS record của domain đó, mỗi subdomain là 1 DNS record có thể
"proxied" (qua edge, được bảo vệ) hoặc "DNS only" (đi thẳng, không bảo vệ).

**Sản phẩm này không làm vậy được** — `01-product-vision.md` đã chốt SSL/TLS + quản lý DNS
"ngoài phạm vi, giả định khách tự quản DNS". Không hosting DNS nghĩa là không thể có khái niệm
"1 domain, N subdomain là DNS record con của nó" theo đúng nghĩa Cloudflare — đây là mô hình
**reverse-proxy theo từng hostname cụ thể**: khách tự tạo CNAME (hoặc A record) trỏ **từng
hostname muốn bảo vệ** về edge-plane, DNS provider vẫn của khách.

→ **Giữ nguyên**: `Zone.hostname` = 1 hostname cụ thể (`shop.example.com`), mỗi hostname là 1
`Zone` độc lập — đúng cái đã build. Không tạo entity `Domain` cha chứa nhiều `Zone` con.

**2 điểm nên thêm** (không đổi model, chỉ UX + validate):
- **Wildcard hostname**: cho phép `hostname` nhận dạng `*.example.com` (1 Zone bảo vệ mọi
  subdomain hiện có/tương lai của domain đó, cùng 1 `originAddress`). Trade-off: tiện (không phải
  tạo lại Zone mỗi khi thêm subdomain mới) nhưng mất khả năng cấu hình policy khác nhau theo từng
  subdomain (`api.example.com` cần rule khác `shop.example.com` thì bắt buộc phải tách Zone
  riêng). Để khách tự chọn: wildcard 1 Zone hoặc N Zone cụ thể — cả 2 đều hợp lệ, chỉ khác giá trị
  `hostname` + validate format (FQDN thường hoặc `*.`+FQDN).
- **Group theo apex domain (UX-only, không phải entity mới)**: portal hiển thị danh sách Zone gộp
  nhóm theo domain gốc (`example.com (3 zones): shop., api., www.`) bằng cách tự suy ra từ
  `hostname` lúc render (2 label cuối cùng phân tách bởi dấu chấm, chưa tính TLD phức hợp như
  `.co.uk` — chấp nhận được ở v1, không cần thư viện Public Suffix List đầy đủ). Không thêm field/
  entity DB nào cho việc này.

## 2. Domain ownership verification — **gap bảo mật thật, chưa có trong spec hiện tại**

Luồng onboard hiện tại (`03-personas-workflows.md` workflow #1) **không có bước xác minh khách
thực sự sở hữu hostname** trước khi tạo `Zone`. Đây không phải thiếu sót nhỏ:

**Kịch bản tấn công cụ thể**: edge-plane dùng chung 1 địa chỉ IP/CNAME target cho mọi khách hàng
(kiến trúc multi-tenant reverse-proxy chuẩn). Kẻ tấn công tạo `Zone` với `hostname:
"victim-shop.com"` (không sở hữu domain này), `originAddress` trỏ về server của attacker. Nếu
nạn nhân thật sự *cũng* từng/sẽ trỏ DNS `victim-shop.com` về cùng edge IP đó (trùng hợp, hoặc do
nhầm hướng dẫn), traffic có thể bị route theo config `Zone` của attacker thay vì của nạn nhân —
lộ traffic, hoặc ít nhất fake được analytics/incident record dưới tên domain người khác trong hệ
thống. Cloudflare, Sucuri, mọi WAAP reverse-proxy thật đều bắt buộc verify ownership trước khi
zone có hiệu lực — sản phẩm này đang thiếu đúng bước đó.

**Đề xuất**: thêm bước verify kiểu ACME (`Let's Encrypt` DNS-01/HTTP-01 challenge), không cần
tự làm CA, chỉ cần chứng minh quyền kiểm soát DNS/webserver:

| Field mới trên `Zone` | Kiểu | Ghi chú |
|---|---|---|
| `verificationToken` | String | random token, sinh tự động lúc tạo Zone |
| `verificationMethod` | Enum(`dnsTxt`, `httpFile`) | khách chọn cách nào tiện hơn |
| `verificationStatus` | Enum(`unverified`, `verified`) | |

- `dnsTxt`: khách thêm TXT record `_waf-verify.<hostname>` = `verificationToken`.
- `httpFile`: khách đặt file `http://<hostname>/.well-known/waf-verify/<verificationToken>`
  (chỉ dùng được nếu origin đã public trước khi bật protection — chấp nhận được, đúng use-case
  "khách đã có site chạy, giờ mới thêm WAF").
- Check thật (DNS lookup / HTTP GET) chạy ở tầng app, **không phải primitive của `metap`** — cần
  1 DNS resolver library (`hickory-resolver` hoặc gọi `reqwest` cho HTTP check) + 1 job định kỳ
  hoặc nút "Verify now" (đồng bộ) gọi check rồi cập nhật `verificationStatus` qua `CrudService`.
  Job định kỳ hợp với `metap-cron` `trigger_type: Schedule` (tự poll các Zone đang `unverified`
  mỗi N phút, tự transition khi thấy verified) — không cần tự viết scheduler.

**Guard `activate` đổi**: hiện tại chỉ check `hasConfig == true`. Thêm điều kiện verify, dùng
`PolicyCondition::All`:
```
activate: pending → active, guard: All([
  Attribute{hasConfig, Eq, true},
  Attribute{verificationStatus, Eq, "verified"},
])
```
Không cần state riêng cho "chưa verify" — verify và cấu hình policy là 2 việc độc lập, khách có
thể làm song song (thêm DdosPolicy/FirewallRule trong lúc chờ DNS propagate), `activate` chỉ chặn
khi thiếu 1 trong 2.

## 3. Luồng onboard — bản đầy đủ (thay thế workflow #1 rút gọn ở `03`)

1. Tenant Admin tạo `Zone`: nhập `hostname` (validate FQDN hoặc wildcard) + `originAddress`.
   `verificationToken` sinh tự động, `verificationStatus = unverified`, `status = pending`.
2. Portal hiện hướng dẫn verify (chọn `dnsTxt` hoặc `httpFile`) + hướng dẫn tạo CNAME/A record
   trỏ hostname về edge-plane (địa chỉ cụ thể lấy từ config hệ thống, không phải field của Zone).
3. Song song: cấu hình ít nhất 1 `DdosPolicy` hoặc `FirewallRule` (`hasConfig` tự bật).
4. Đặt `protectionMode = monitor` — chạy thử.
5. Hệ thống (job định kỳ hoặc khách bấm "Verify now") xác nhận DNS/HTTP challenge →
   `verificationStatus = verified`.
6. Khi cả 2 điều kiện đủ, khách chuyển `protectionMode = enforce` → transition
   `status: pending → active`.
7. Edge-plane nhận config, bắt đầu bảo vệ thật (đúng bước cuối workflow #1 cũ).

## 4. Cấu hình rule — model hiện tại đã đủ tốt, không cần entity mới

Xem lại `FirewallRule` (đã build): 2 nhu cầu người dùng hay hỏi khi "cấu hình rule" đã có sẵn
đường giải quyết, không phải gap:

- **Test 1 rule mới mà không ảnh hưởng rule khác đang chạy thật**: dùng `action = log` cho riêng
  rule đó (không cần đưa cả `Zone` về `protectionMode = monitor`, cái đó ảnh hưởng toàn bộ zone).
  Đây là điểm mạnh sẵn có của model — action theo từng rule, độc lập với `protectionMode` toàn
  zone.
- **Sắp xếp lại thứ tự evaluate**: đổi `priority`. Cần 1 portal feature "kéo-thả reorder" — về
  data chỉ là update field `priority` hàng loạt (bulk update qua nhiều lần `PATCH`, hoặc 1
  `BulkQueryAction` nếu `metap-cron`'s target type đó dùng được cho use-case ngoài cron — cần
  kiểm tra khi build, không chắc `BulkQueryAction` nghĩa là gì chính xác từ tên).

**Rule template/managed ruleset** (kiểu OWASP CRS, bấm 1 nút bật cả bộ rule có sẵn) — giữ nguyên
quyết định "v2+" của `01-product-vision.md`, không kéo vào v1.

## 5. Whitelist / Blacklist — tái dùng `FirewallRule`, KHÔNG tạo entity mới

**Đảo ngược 2026-09-13**: chủ dự án chủ động chọn tạo entity `waf.ip_access_lists` riêng (IP/CIDR
only — geo/country vẫn ở lại `FirewallRule` như cũ, chỉ đổi nhãn `ruleType: geoFirewall` → `waf`),
thay vì tái dùng `FirewallRule` như đề xuất gốc bên dưới — API/UI rõ ràng hơn cho whitelist/
blacklist, không cần khách hiểu `matchCondition` JSON. `compile.rs::compile_ip_access_list` vẫn
dịch xuống đúng `CompiledRule`/`Predicate` shape đã có (không có wire type mới). Thứ tự evaluate ở
§5a bên dưới **đã code thật** (`edge-plane/waf-edge/src/evaluate.rs`): DDoS chuyển thành bước cuối
cùng, chỉ check khi không entry/rule nào match — áp dụng cho `IpAccessList` lẫn `FirewallRule`
thường, không riêng gì `ruleType ∈ {ipFirewall, geoFirewall}` như văn bản gốc hình dung (đơn giản
hoá hơn, vì `IpAccessList` giờ là nguồn riêng biệt, không còn là 1 `ruleType` của `FirewallRule`
nữa). Scope tenant-wide ở §5b **đã code thật** ở cả `FirewallRule` lẫn `IpAccessList`. Phần phân
tích gốc bên dưới giữ nguyên làm bối cảnh, không xoá.

`01-product-vision.md` đã chốt lý do gộp WAF/rate-limit/IP-geo firewall vào 1 entity để tránh
"3 UI khác nhau trùng lặp logic" — whitelist/blacklist chính là `ruleType: ipFirewall` hoặc
`geoFirewall` với `action: allow` hoặc `block`. Không cần entity `IpAccessList` riêng. Việc thật
sự còn thiếu là **2 điều chưa được quyết, không phải thiếu entity**:

### 5a. Precedence với `DdosPolicy` — gap chưa document

`FirewallRule` và `DdosPolicy` hiện là 2 cơ chế evaluate song song, **chưa có thứ tự ưu tiên nào
được ghi lại**. Whitelist chỉ có ý nghĩa thật ("IP này luôn được vào, kể cả khi đang bị DDoS
mitigation chặn") nếu nó bypass được cả `DdosPolicy`, không chỉ các `FirewallRule` khác — đúng
hành vi Cloudflare IP Access Rules thật (allow là bypass toàn bộ, kể cả rate limiting).

**Đề xuất thứ tự evaluate ở edge-plane** (ghi vào `04-architecture-boundary.md` khi build
edge-plane, chưa code được nhưng cần chốt hành vi ngay vì ảnh hưởng cách management-plane/control-plane
mô tả rule cho edge hiểu):
1. `FirewallRule` có `ruleType ∈ {ipFirewall, geoFirewall}` — **evaluate rule `action = allow`
   (whitelist) trước, rồi mới tới `action = block`/`challenge` (blacklist)**, bất kể số
   `priority` — `priority` chỉ tie-break thứ tự *trong cùng 1 nhóm action* (nhiều rule allow với
   nhau, hoặc nhiều rule block với nhau), không dùng để so allow với block. Match `allow` →
   bypass toàn bộ (mọi `FirewallRule` khác + `DdosPolicy`) cho request này, dừng evaluate. Match
   `block`/`challenge` → thực hiện ngay, dừng evaluate. Match `log` → ghi nhận, **không dừng**,
   evaluate tiếp.
2. `FirewallRule` còn lại (`waf`/`rateLimit`), theo `priority`, first-match-wins (đã có ở `02`).
3. `DdosPolicy` (rate-based) — chỉ evaluate nếu chưa bị match ở bước 1/2.

### 5b. Scope: zone-only hay tenant-wide — quyết định v1

Hiện `FirewallRule.zoneId` bắt buộc — whitelist chỉ áp dụng được cho 1 zone, khách có N zone phải
tạo lại rule y hệt N lần (vd whitelist IP văn phòng cho mọi site). Đây là friction thật nhưng
**đề xuất chấp nhận ở v1** (đúng tinh thần tối giản đã theo xuyên suốt `01-product-vision.md`) —
tenant-wide rule (`zoneId` optional, `null` = áp dụng mọi zone) để v2+: cần đổi
`metap-metadata` (nullable Reference với semantics đặc biệt) + edge-plane phải merge rule
tenant-wide với rule riêng zone, phức tạp hơn đáng kể so với lợi ích ở quy mô demo.

### 5c. Portal UX (không đổi data model)

Whitelist/blacklist nên có 1 **view riêng trong portal** (list-view filter sẵn
`ruleType in [ipFirewall, geoFirewall]`, ẩn bớt field không liên quan như `rateLimitThreshold`),
không phải 1 form CRUD chung chung cho `FirewallRule` — để khách không phải hiểu khái niệm
`matchCondition` JSON khi chỉ muốn "chặn IP này". Cần thêm:
- **Bulk add**: dán nhiều IP/CIDR 1 lúc, portal tự tạo nhiều `FirewallRule` (mỗi dòng
  `matchCondition: {"attribute":"sourceIp","op":"In","value":[...]}` gộp chung 1 rule thay vì N
  rule riêng — gọn hơn, evaluate nhanh hơn ở edge).
- Form riêng cho `ipFirewall` chỉ hỏi IP/CIDR + action; form riêng cho `geoFirewall` chỉ hỏi
  country code + action — `matchCondition` build tự động ở tầng app/FE, khách không tự viết JSON.

## Thay đổi cần áp dụng khi build (nếu duyệt phần trên)

- `zone_entity.rs` / `02-domain-model.md`: thêm `verificationToken`, `verificationMethod`,
  `verificationStatus`; sửa guard `activate` thành `All([hasConfig, verificationStatus])`.
- `05-metap-technical-mapping.md`: cập nhật bảng field `waf.zones` + đoạn Workflow tương ứng.
- `04-architecture-boundary.md`: thêm mục thứ tự evaluate edge-plane (5a) — ghi nhận, chưa cần
  code vì edge-plane chưa bắt đầu, nhưng chốt behavior contract sớm để control-plane compile rule
  đúng thứ tự sau này.
- Portal (chưa tới lượt — chưa có FE): ghi nhận 2 view cần có sau khi có FE — whitelist/blacklist
  view riêng (5c), rule reorder UI (mục 4).
- Cần 1 job xác minh domain (`metap-cron` `Schedule`) — chưa code, ghi nhận vào thứ tự build ở
  `05` khi tới bước `waf.zones`.
