# 09 — Access control: 2 portal, gán quyền theo (user, domain)

Phát sinh khi đào sâu module Team & Permissions (`08` mục 9) — hoá ra không phải chuyện riêng của
Developer, mà là nguyên tắc phân quyền chung cho toàn sản phẩm. Sửa lại 1 chỗ đã ghi sai ở
`03-personas-workflows.md` (Platform Admin **không** mặc định toàn quyền mọi tenant).

## Nguyên tắc

1. **2 portal riêng**: Admin Portal (đội vận hành SaaS) và Customer Portal (tổ chức khách hàng
   dùng dịch vụ WAF) — khác domain/URL, khác đối tượng đăng nhập.
2. **Không ai mặc định thấy hết** — kể cả tài khoản admin. Mọi user (cả 2 portal) chỉ xem được
   domain nào **được gán quyền cho họ**, gán theo cặp (user, domain) — không suy ra từ role không
   thôi (vd không phải "Platform Admin thì tự nhiên thấy hết mọi tenant").
3. **Customer Portal luôn bị khoá cứng trong 1 tổ chức** (ranh giới tenant, không đổi — user của
   tổ chức A không bao giờ thấy dữ liệu tổ chức B). Trong phạm vi tổ chức mình, xem được domain
   nào vẫn theo gán quyền như (2) — không phải cứ vào được tổ chức là thấy hết mọi Zone của tổ
   chức đó.
4. **Admin Portal không bị khoá theo 1 tổ chức** — 1 tài khoản admin có thể được gán quyền vào
   nhiều tổ chức/domain khác nhau (khác Customer Portal ở điểm này), nhưng vẫn phải được gán mới
   thấy, không mặc định.
5. **Tạo user phải gán quyền ngay** (role + danh sách domain được xem) — không gán thì mặc định
   read-only, không phải toàn quyền hay không thấy gì cả.

## Vì sao đây là 1 cơ chế, không phải 2 (admin riêng, customer riêng)

Cả 2 portal cùng cần đúng 1 câu hỏi: "user X có được xem domain Y không?" — khác nhau duy nhất ở
chỗ Customer Portal *luôn* filter thêm 1 lớp "domain đó phải thuộc tổ chức của user" (mục 3),
Admin Portal thì không filter theo tổ chức (mục 4). Vậy nên dùng **chung 1 bảng gán quyền** dạng
`(userId, zoneId)` — "user này được gán domain này" — thay vì làm 2 cơ chế riêng biệt cho 2
portal. Tenant Admin không cần gán từng zone (họ mặc định thấy hết zone trong tổ chức mình, đúng
vai trò "chủ tài khoản") — chỉ role thấp hơn (SOC/Developer/Viewer) và mọi tài khoản Admin Portal
mới cần gán rõ theo zone.

## Vướng kỹ thuật cần xác nhận khi build (không chặn việc code Zone/DdosPolicy/FirewallRule hiện
tại, chỉ cần biết trước khi làm module Team & Permissions)

`metap` tổ chức multi-tenant theo kiểu **mỗi tenant 1 schema/DB riêng** (`Router` chọn DB theo
`tenantId` trong request context — đã xác nhận lúc nghiên cứu `metap-permission`/`metap-control`
ở phần trước). Nghĩa là:

- Bảng gán quyền `(userId, zoneId)` cho **Customer Portal** đặt trong DB/schema riêng của từng
  tenant là tự nhiên, không có gì đặc biệt (giống mọi entity khác đã build).
- Nhưng bảng gán quyền cho **Admin Portal** (1 admin thấy zone của N tổ chức khác nhau) **không
  thể** nằm trong DB riêng của 1 tenant — phải nằm ở tầng platform dùng chung (giống
  `control.tenants` — bảng platform-level, không thuộc tenant nào), vì trước khi biết "admin X
  được gán những tenant/zone nào", hệ thống còn chưa biết phải mở kết nối tới DB tenant nào để
  tra.
- `PolicyCondition` (`metap-permission`) chỉ so sánh attribute của **1 record đã load sẵn** với
  literal/context — không tự query bảng khác được (xác nhận lúc nghiên cứu trước). Vậy việc "user
  X có được gán zone Y không" cần **resolve trước** khi tới bước evaluate permission (query bảng
  gán quyền, đưa kết quả vào context dạng danh sách `assignedZoneIds`, rồi `PolicyCondition` chỉ
  cần so `zoneId In context.assignedZoneIds`) — đúng kiểu giải pháp đã dùng cho guard `hasConfig`
  ở `Zone` (resolve trước, guard chỉ đọc field/context đã tính sẵn, không tự join).
- Danh sách `assignedZoneIds` đưa vào context lúc nào — mint JWT (đơn giản, nhưng đổi phân công
  phải cấp lại token) hay tra DB mỗi request (luôn đúng real-time, nhưng thêm 1 query mỗi
  request, cho cả 2 portal)? **Chưa chốt — cần quyết định lúc build module Team & Permissions**,
  không ảnh hưởng gì tới việc code Zone/DdosPolicy/FirewallRule đang làm.

## Việc cần làm khi tới module Team & Permissions (chưa làm ngay)

- Entity mới `waf.zone_access_grants` (hoặc tên khác) — `(userId, zoneId)`, có thể thêm
  `role`/`scope` nếu cần phân biệt "xem" vs "sửa" chi tiết hơn RBAC role chung.
- Với Admin Portal: entity gán quyền tương tự nhưng đặt ở tầng platform (`control` schema,
  giống `control.tenants`) — cần xác nhận `metap` có sẵn chỗ cho việc này hay phải tự thêm
  migration/table platform-level mới (khả năng phải nghiên cứu thêm 1 vòng như đã làm với
  `PolicyCondition`/`metap-cron` trước đây).
- Portal: form tạo user bắt buộc chọn role + (nếu không phải Tenant Admin) chọn domain được gán,
  không cho bỏ trống im lặng.
