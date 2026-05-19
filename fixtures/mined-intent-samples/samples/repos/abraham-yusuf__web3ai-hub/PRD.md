# PRD — Product Requirements Document
## Web3AI Hub: Platform Blog & Learning Web3 + AI

| | |
|---|---|
| **Versi** | 1.0.0 |
| **Status** | Draft — In Review |
| **Tanggal** | 2025 |
| **Author** | Platform Owner |
| **Reviewer** | — |

---

## 1. Executive Summary

Web3AI Hub adalah platform konten berbasis Next.js yang menyajikan empat pilar utama: blog edukasi, dokumentasi interaktif, direktori airdrop/bounty, dan katalog AI tools. Platform ini ditargetkan untuk pemula hingga intermediate yang ingin belajar Web3 dan AI, sekaligus dirancang untuk menghasilkan pendapatan pasif melalui Google AdSense dan affiliate marketing.

**Problem Statement:**
Konten Web3 dan AI tersebar di banyak tempat, tidak terstruktur, sering tidak terupdate, dan jarang tersedia dalam Bahasa Indonesia. Tidak ada platform tunggal yang menyatukan blog, tutorial, airdrop tracker, dan AI tools dalam satu ekosistem yang kohesif.

**Solution:**
Platform all-in-one yang dibangun untuk kreator konten solo maupun tim kecil, dengan AI sebagai force multiplier untuk produksi konten, dan monetisasi built-in sejak hari pertama.

---

## 2. Goals & Non-Goals

### Goals (v1.0)
- Membangun platform blog MDX yang dapat dipublikasikan dan dikelola sepenuhnya dari browser
- Mengintegrasikan AI writer dengan dukungan multi-provider yang dapat dikonfigurasi
- Menyediakan dokumentasi interaktif dengan AI Q&A sidebar
- Membangun Airdrop Hub dengan step tracker yang bisa disave per user
- Membuat AI Tools Directory dengan fitur compare dan rating
- Mengintegrasikan Google AdSense di semua halaman konten
- Mendeploy ke Vercel dengan performa Lighthouse ≥ 90 di semua kategori

### Non-Goals (v1.0)
- Forum / komunitas diskusi (direncanakan v2.0)
- Mobile app native (iOS/Android)
- Multi-bahasa selain Indonesia dan Inggris
- User-generated blog posts (hanya admin)
- Live price tracker crypto terintegrasi
- Wallet connect / Web3 authentication

---

## 3. User Personas

### Persona 1 — Pemula Web3 "Budi"
- **Usia:** 22 tahun, mahasiswa / fresh graduate
- **Goal:** Belajar cara dapat crypto gratis dari airdrop, memahami dasar Web3
- **Pain point:** Tutorial yang ada tidak jelas langkah-langkahnya, takut salah dan kehilangan gas fee
- **Behavior:** Lebih suka konten visual, sering pakai HP
- **Fitur utama:** Airdrop Hub, Learn Web3 track, Blog beginner

### Persona 2 — Developer AI "Sari"
- **Usia:** 28 tahun, software engineer
- **Goal:** Selalu update tools AI terbaru, belajar integrasi LLM ke aplikasi
- **Pain point:** Terlalu banyak noise di Twitter/X, sulit tahu tool mana yang worth it
- **Behavior:** Suka konten teknikal mendalam, pakai laptop, habiskan 30+ menit per session
- **Fitur utama:** AI Tools Directory, Learn AI track, Blog teknikal

### Persona 3 — Investor/Trader "Reza"
- **Usia:** 35 tahun, karyawan dengan income sampingan crypto
- **Goal:** Dapat token gratis dari airdrop sebelum listing, stay updated market
- **Pain point:** Banyak airdrop scam, step tutorial tidak lengkap
- **Behavior:** Aktif di beberapa platform sekaligus, value time sangat tinggi
- **Fitur utama:** Airdrop Hub (filter by reward size), Blog market analysis

### Persona 4 — Platform Owner "Admin"
- **Usia:** 25–35 tahun, content creator / developer
- **Goal:** Produksi konten konsisten tanpa burn out, monetisasi maksimal
- **Pain point:** Nulis artikel tiap hari melelahkan, setup teknikal rumit
- **Behavior:** Power user admin panel, ingin semua bisa dikontrol dari satu tempat
- **Fitur utama:** AI Writer, Admin Dashboard, Settings AI provider

---

## 4. Feature Requirements

### 4.1 Blog Module

#### FR-B01: Halaman Listing Blog
- **Priority:** P0 (Must Have)
- Menampilkan grid/list post dengan thumbnail, judul, excerpt, tanggal, kategori, reading time
- Pagination atau infinite scroll
- Filter berdasarkan kategori dan tag
- Sticky search bar
- AdSense slot di atas fold dan antara konten

#### FR-B02: Halaman Detail Post
- **Priority:** P0 (Must Have)
- Render MDX dengan syntax highlighting (Shiki/Prism)
- Table of contents (sticky sidebar di desktop)
- Progress bar scroll
- Related posts di bawah artikel
- Share buttons (Twitter/X, Telegram, copy link)
- Reading time estimate
- AdSense: in-article ad setelah paragraf ke-3, dan di akhir artikel

#### FR-B03: MDX Support
- **Priority:** P0 (Must Have)
- Frontmatter: title, date, author, category, tags, excerpt, coverImage, published, featured
- Custom MDX components: Callout, CodeBlock, YoutubeEmbed, ImageCaption, Comparison
- GitHub Flavored Markdown support
- Mermaid diagram support

#### FR-B04: AI Writer
- **Priority:** P0 (Must Have)
- Form input: topic, tone (formal/casual/technical), length (short/medium/long), language (ID/EN)
- Pilih AI provider dan model dari dropdown (sesuai yang dikonfigurasi di settings)
- Output: MDX lengkap siap publish (frontmatter + content)
- Streaming output (tampil karakter per karakter)
- Tombol: Copy, Insert ke Editor, Regenerate, Edit manual
- Template prompt untuk berbagai jenis konten: tutorial, opini, news, review

#### FR-B05: Admin Post Manager
- **Priority:** P0 (Must Have)
- CRUD operasi untuk semua post
- Draft / published / scheduled status
- Rich text preview MDX sebelum publish
- Upload cover image dengan crop
- Schedule publish dengan tanggal/waktu

### 4.2 Learn Module

#### FR-L01: Navigasi Dokumentasi
- **Priority:** P0 (Must Have)
- Sidebar dengan struktur bertingkat (sections > chapters > pages)
- Highlight halaman aktif
- Collapsible sections
- Breadcrumb navigation
- Previous / Next page navigation
- Pencarian dalam dokumentasi

#### FR-L02: Halaman Konten Docs
- **Priority:** P0 (Must Have)
- MDX render sama dengan blog
- Right sidebar: Table of Contents halaman saat ini
- AI Chat sidebar (collapsible, default collapsed di mobile)
- "On this page" mini TOC

#### FR-L03: AI Q&A Sidebar
- **Priority:** P1 (Should Have)**
- Sliding panel dari kanan
- Konteks: AI mendapat isi halaman saat ini sebagai konteks
- Chat history per session (localStorage)
- Suggested questions di bawah konten
- Powered by provider yang dikonfigurasi

#### FR-L04: Progress Tracker
- **Priority:** P1 (Should Have)
- Track progress per user (login required untuk save ke DB, guest pakai localStorage)
- Progress bar per track dan per section
- Tombol "Mark as Complete" di setiap halaman
- Dashboard progress di profil user

#### FR-L05: Track Kurikulum
- **Priority:** P0 (Must Have)
- Track Web3: Blockchain Basics → Ethereum → Smart Contract → DeFi → NFT → DAO
- Track AI: AI Fundamentals → Prompt Engineering → LLM APIs → AI Agents → Fine-tuning

### 4.3 Airdrop Hub Module

#### FR-A01: Listing Airdrop
- **Priority:** P0 (Must Have)
- Card tiap airdrop: nama, logo, network, status badge (Active/Upcoming/Ended), estimasi reward, difficulty, deadline
- Filter: status, network (ETH/SOL/BSC/ARB/OP/dll), difficulty (Easy/Medium/Hard), reward size
- Sort: reward DESC, newest, deadline terdekat
- Search by nama project

#### FR-A02: Detail Airdrop
- **Priority:** P0 (Must Have)
- Header: logo, nama, deskripsi singkat, network badge, status, estimasi reward USD
- Step-by-step tutorial dengan MDX
- **Step Tracker**: checkbox per langkah, progress tersimpan di localStorage
- Requirements: wallet yang dibutuhkan, minimum balance, tools yang perlu diinstall
- Social links: Twitter, Discord, Website
- "Report Scam" button

#### FR-A03: Admin Airdrop Manager
- **Priority:** P0 (Must Have)
- CRUD airdrop listings
- Field: nama, slug, deskripsi, logo, network, estimatedReward, difficulty, deadline, status, steps (MDX), requirements, links
- AI Generate Tutorial: input nama project, AI hasilkan step-by-step guide

#### FR-A04: Bounty Board
- **Priority:** P2 (Nice to Have)
- Listing bounty/task dengan reward crypto
- Filter by skill (development, design, content)

### 4.4 AI Tools Directory

#### FR-T01: Listing Tools
- **Priority:** P0 (Must Have)
- Grid card: logo, nama, tagline, kategori badge, pricing badge (Free/Freemium/Paid), rating
- Filter: kategori, pricing model
- Sort: rating, newest, featured
- Featured tools section di atas (dapat dikonfigurasi)

#### FR-T02: Halaman Detail Tool
- **Priority:** P0 (Must Have)
- Header: logo, nama, tagline, link website (affiliate), pricing, rating
- Deskripsi lengkap (MDX)
- Key features, pros & cons
- Alternatif tools (manual atau auto-suggest AI)
- Screenshots/demo video embed
- User reviews dan rating (P1)

#### FR-T03: Fitur Compare
- **Priority:** P1 (Should Have)
- Pilih 2–3 tool untuk dibandingkan
- Tabel perbandingan: fitur, harga, rating, kelebihan/kekurangan
- Share comparison link

#### FR-T04: Kategori Tools
- **Priority:** P0 (Must Have)
- Writing & Content, Coding & Development, Image Generation, Video Generation, Audio & Music, Research & Analysis, Web3 & Crypto, Productivity & Automation

#### FR-T05: Admin Tool Manager
- **Priority:** P0 (Must Have)
- CRUD tool listings
- Upload logo, screenshots
- Set affiliate link per tool
- Toggle featured status

### 4.5 Admin Dashboard

#### FR-AD01: Overview Dashboard
- **Priority:** P1 (Should Have)
- Metric cards: total posts, total views (7/30 hari), top posts, total airdrops aktif, total tools
- Quick actions: New Post, New Airdrop, New Tool

#### FR-AD02: AI Provider Settings
- **Priority:** P0 (Must Have)
- List provider dengan toggle enable/disable
- Input API key (disimpan terenkripsi di DB)
- Dropdown pilih model per provider
- Slider temperature (0.0–1.0)
- Textarea custom system prompt
- Default provider per fitur (blog writing, learn chat, airdrop tutorial)
- Test connection button

#### FR-AD03: AdSense Settings
- **Priority:** P0 (Must Have)
- Input Publisher ID (ca-pub-XXXX)
- Ad slot ID per posisi (blog in-article, sidebar, footer, learn page, tools page)
- Toggle enable/disable per section

---

## 5. Non-Functional Requirements

### Performance
- **LCP (Largest Contentful Paint):** ≤ 2.5 detik pada koneksi 4G
- **FID / INP:** ≤ 200ms
- **CLS:** ≤ 0.1
- **Lighthouse Score:** ≥ 90 untuk Performance, Accessibility, Best Practices, SEO
- Semua halaman menggunakan ISR (Incremental Static Regeneration)
- Image optimization dengan `next/image` (WebP/AVIF)
- Core Web Vitals dimonitor dengan Vercel Analytics

### SEO
- Dynamic OG image generation (menggunakan `@vercel/og`)
- JSON-LD structured data (Article, HowTo untuk airdrop, SoftwareApplication untuk tools)
- Sitemap.xml dinamis
- robots.txt terkonfigurasi
- Canonical URLs untuk semua halaman
- Meta tags lengkap (title, description, og:*, twitter:*)

### Security
- API keys AI provider disimpan terenkripsi (AES-256) di database
- Admin routes dilindungi NextAuth.js middleware
- Rate limiting pada semua API routes AI (10 req/menit per IP)
- Input sanitization untuk semua form
- CSRF protection
- Security headers via `next.config.js`

### Accessibility
- WCAG 2.1 Level AA compliance
- Keyboard navigable sepenuhnya
- Screen reader friendly (ARIA labels)
- Color contrast ratio ≥ 4.5:1
- Focus indicators yang visible

### Responsiveness
- Mobile-first design
- Breakpoints: 375px (mobile), 768px (tablet), 1024px (desktop), 1440px (wide)
- Touch-friendly tap targets (≥ 44×44px)
- Sidebar docs collapsible di mobile

---

## 6. Design System

### Brand Identity
- **Nama:** Web3AI Hub
- **Tagline:** "Belajar Web3 & AI, Satu Platform."
- **Tone:** Friendly, technical tapi approachable, bilingual (ID/EN)

### Color Palette
```
Primary:    #7C3AED  (Violet-600)  — brand utama
Secondary:  #06B6D4  (Cyan-500)   — aksen Web3
Accent:     #F59E0B  (Amber-500)  — highlight/CTA
Success:    #10B981  (Emerald-500)
Warning:    #F59E0B  (Amber-500)
Danger:     #EF4444  (Red-500)
Background: #0F0F0F  (dark) / #FAFAFA (light)
Surface:    #1A1A1A  (dark) / #FFFFFF (light)
```

### Typography
```
Font Display:  Inter (headings, UI)
Font Mono:     JetBrains Mono (code blocks)
Scale:         12 / 14 / 16 / 18 / 20 / 24 / 30 / 36 / 48px
```

### Dark Mode
- Default: Dark mode
- User dapat toggle light/dark
- Tersimpan di localStorage + `prefers-color-scheme` respecting

---

## 7. Analytics & Metrics

### Metric Sukses (3 bulan post-launch)
| Metric | Target |
|--------|--------|
| Monthly Active Users | ≥ 1.000 |
| Avg. Session Duration | ≥ 3 menit |
| Blog posts dipublikasikan | ≥ 50 |
| Airdrop guides | ≥ 20 |
| AI Tools listed | ≥ 100 |
| Google AdSense Revenue | ≥ $50/bulan |
| Lighthouse Score | ≥ 90 semua kategori |

### Tracking Events
- Page views per konten
- AI Writer usage (berapa kali generate, provider apa)
- Airdrop step completion rate
- Tool directory clicks (affiliate link clicks)
- AI Chat interactions di Learn module
- Search queries

---

## 8. Dependencies & Risiko

### Dependencies Eksternal
| Dependensi | Risiko | Mitigasi |
|------------|--------|----------|
| OpenAI API | Rate limit, cost naik | Multi-provider fallback, cache response |
| Google AdSense | Akun suspended | Patuhi policies, backup revenue stream |
| Vercel | Free tier limits | Monitor usage, upgrade plan jika perlu |
| Neon PostgreSQL | Cold start latency | Connection pooling, edge caching |

### Risiko Teknis
- **MDX parsing performance** → Cache compiled MDX di filesystem + Redis
- **AI streaming timeout** → Implement timeout handler dan fallback message
- **Database cost** → Gunakan caching agresif, optimize queries dengan Prisma

---

## 9. Open Questions

1. Apakah user registration diperlukan di v1.0, atau cukup admin-only? Cukup Admin Only
2. Apakah comment system (Giscus/Disqus) perlu di v1.0? ya, gunakan Giscus
3. Bahasa default konten: Indonesia, Inggris, atau bilingual? gunakan Indonesia dan Inggris saja
4. Apakah perlu newsletter integration (Resend/Mailchimp) di v1.0? Gunakan Resend
5. Submission airdrop oleh komunitas — perlu moderation queue? boleh oleh komunitas, buatkan halaman dan form khusus

---

*Dokumen ini akan diupdate seiring dengan perkembangan proyek.*
