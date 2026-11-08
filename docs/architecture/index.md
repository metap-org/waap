# Kiến trúc WAAP mục tiêu

Tài liệu kiến trúc **đích** cho sản phẩm WAAP (Web Application & API Protection) enterprise, bán
dạng SaaS trước, giữ đường cho on-prem. Tài liệu dựng từ yêu cầu sản phẩm, **không** mô tả code
hiện có trong repo này: phần code ở `management-plane/`, `control-plane/`, `edge-plane/` là bản demo,
chênh lệch so với kiến trúc đích được ghi ở [11. Risks and Technical Debt](11-risks/00-index.md).

Trạng thái: **bản nháp có quyết định** (2026-11-06). Mục 1–4 và level 1–2 của mục 5 mô tả *vai trò*,
không gắn sản phẩm. Lựa chọn công nghệ nằm ở [05 · bản đồ công nghệ](05-building-blocks/07-technology-map.md)
và ADR-0014 trở đi (ADR-0030 cho ngôn ngữ từng plane); detail design mức C3 ở [5.8](05-building-blocks/08-components/00-index.md).

## Cách tổ chức

Cùng cách `metap-docs/docs/architectures` đang làm, ba framework bổ trợ nhau:

- **[arc42](https://arc42.org)** là khung tài liệu: 12 thư mục đánh số, mỗi thư mục có
  `00-index.md`.
- **[C4](https://c4model.com)** là ký hiệu diagram, chỉ dùng ở chỗ nó hợp: System Context ở
  [03](03-context/00-index.md), Container và Component ở [05](05-building-blocks/00-index.md).
- **4+1 View Model** là danh sách kiểm: Logical/Development → [05](05-building-blocks/00-index.md),
  Process/Scenarios → [06](06-runtime/00-index.md), Physical → [07](07-deployment/00-index.md).

Diagram viết bằng Mermaid để GitHub render trực tiếp và diff được trong PR.

**Quy ước đánh số:** số đầu tên thư mục/file trùng số mục arc42.

- Thư mục `NN-…` là mục `N`; `00-index.md` là trang của chính mục đó.
- File `MM-…` trong thư mục `NN` là mục `N.M`; ví dụ `06-runtime/10-analytics-flows.md` là mục 6.10,
  vì 6.1–6.9 nằm ngay trong `06-runtime/00-index.md`.
- Thư mục con lặp lại quy tắc một cấp nữa: `05-building-blocks/08-components/05-config-and-compiler.md`
  là mục 5.8.5.
- ADR đánh số riêng, liên tục `0001`, `0002`, … và không bao giờ đánh lại.

## Các mục

1. [Introduction and Goals](01-introduction/00-index.md): sản phẩm làm gì, nghiệp vụ, mục tiêu chất lượng, các bên liên quan
2. [Architecture Constraints](02-constraints/00-index.md): ràng buộc kỹ thuật, tổ chức, pháp lý
3. [System Scope and Context](03-context/00-index.md): bối cảnh nghiệp vụ và kỹ thuật, C4 System Context
4. [Solution Strategy](04-strategy/00-index.md): các quyết định nền tảng và lý do
5. [Building Block View](05-building-blocks/00-index.md): 4 plane, thành phần, hợp đồng giữa các plane, mô hình dữ liệu, công nghệ, detail design C3
6. [Runtime View](06-runtime/00-index.md): các kịch bản chính dạng sequence
7. [Deployment View](07-deployment/00-index.md): mô hình triển khai SaaS / hybrid / on-prem, vùng chịu lỗi
8. [Cross-cutting Concepts](08-cross-cutting/00-index.md): multi-tenancy, bảo mật nền tảng, version, quyền riêng tư, entitlement
9. [Architecture Decisions](09-adr/00-index.md): ADR
10. [Quality Requirements](10-quality/00-index.md): cây chất lượng, kịch bản đo được
11. [Risks and Technical Debt](11-risks/00-index.md): rủi ro, nợ kỹ thuật, chênh lệch với demo
12. [Glossary](12-glossary/00-index.md)

## Quan hệ với tài liệu khác trong repo

- `docs/requirements/` ([index](../requirements/index.md)): bản đồ phân rã chức năng, requirements, phân tích
  nghiệp vụ. Tách khỏi kiến trúc: requirements nói hệ thống phải làm gì, thư mục này nói làm bằng gì. Kiến
  trúc trỏ sang requirements khi cần lý do, không chép lại.
- `docs/overview/`: bản HTML để đọc và trình bày (xem `README.md` trong đó), prefix `v1-`; bản hiện hành sinh từ thư mục này.
- `management-plane/docs/`: spec nghiệp vụ và màn hình của portal demo. Phần nghiệp vụ (persona, domain,
  gói cước) vẫn dùng được làm đầu vào; phần ranh giới kiến trúc ở đó được thay bằng tài liệu này.
- Tên gọi: thư mục `management-plane/` của repo là **Management plane** ở đây (trước 2026-11-08 thư mục
  này tên là `data-plane/`, đổi cho khớp [ADR-0001](09-adr/0001-four-planes.md)). Theo nghĩa chuẩn ngành,
  *data plane* là đường đi của traffic, tức **Edge plane**.
