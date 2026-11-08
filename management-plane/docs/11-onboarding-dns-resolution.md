# 11 — Onboarding: phân giải domain/IP

Bổ sung vào luồng onboard (`06-onboarding-rules-lists.md` mục 2-3) — thêm bước phân giải DNS,
tách bạch rõ 2 loại "verify" khác nhau đang dễ bị nhầm là 1.

## 2 loại kiểm tra khác nhau, đừng gộp làm 1

1. **Xác minh quyền sở hữu domain** (đã có ở `06` mục 2 — TXT record/HTTP file) — chứng minh
   khách **kiểm soát được** domain, KHÔNG cần traffic thật đã trỏ về hệ thống. Bắt buộc để
   `activate`.
2. **Trạng thái DNS routing** (mới, bổ sung ở đây) — domain đã thực sự **trỏ traffic** về
   edge-plane hay chưa (CNAME/A record đã cutover chưa). Chỉ mang tính thông tin, **không** chặn
   `activate` — vì trước khi cutover DNS thật, "activate" chỉ là bật sẵn sàng, chưa có traffic
   nào chảy qua để mà bảo vệ cả.

## Bước phân giải khi nhập hostname (trước khi submit form Add Zone)

Ngay khi khách gõ `hostname`, hệ thống tra DNS hiện tại (A/AAAA/CNAME) và hiện luôn:
> "Hiện tại `shop.example.com` đang trỏ về: `203.0.113.10`"

Dùng kết quả này để:
- **Gợi ý sẵn `originAddress`** (khách chỉ cần xác nhận thay vì tự gõ tay IP gốc — giảm sai sót
  gõ nhầm).
- Nếu tra không ra gì (domain chưa từng trỏ đi đâu, hoặc gõ sai) → cảnh báo ngay tại form, đỡ để
  khách submit xong mới biết.

## Panel "Trạng thái DNS routing" (trên Zone Overview, song song panel ownership verification)

- Hiện kết quả tra DNS gần nhất: đã trỏ đúng về edge-plane hay còn trỏ thẳng origin.
- Nút "Kiểm tra lại" (tra DNS on-demand) + auto re-check định kỳ giống panel ownership.
- Badge trạng thái: "Chưa trỏ (traffic vẫn đi thẳng origin)" / "Đã trỏ về hệ thống bảo vệ".
- **Không có nút chặn hành động nào phụ thuộc panel này** — chỉ để khách tự biết mình đã cutover
  xong chưa, tránh thắc mắc "sao tôi activate rồi mà vẫn không thấy traffic/event nào" (câu trả
  lời: vì DNS chưa trỏ qua, không phải hệ thống lỗi).

## Test Origin Connection

Nút riêng (cạnh field `originAddress`): thử kết nối tới `originAddress` (HTTP HEAD hoặc TCP
connect) — báo "Kết nối được" hoặc "Không phản hồi, kiểm tra lại địa chỉ/firewall origin" trước
khi khách activate. Cảnh báo mềm (không chặn cứng) — origin có thể chỉ mở port cho riêng IP của
edge-plane (chưa whitelist được lúc đang test), nên "không kết nối được" chỉ là gợi ý kiểm tra
lại, không phải lỗi bắt buộc sửa.

## Field kỹ thuật cần thêm (khi build tới `waf.zones`)

| Field | Kiểu | Ghi chú |
|---|---|---|
| `dnsRoutingStatus` | Enum(`notRouted`, `routed`, `unknown`) | cache kết quả check gần nhất, tránh tra DNS mỗi lần render trang |
| `lastDnsCheckAt` | Datetime | |

Không phải guard, không ảnh hưởng workflow `status` — thuần hiển thị.
