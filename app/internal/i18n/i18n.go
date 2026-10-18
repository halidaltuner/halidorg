// Package i18n holds the UI string catalog for the supported languages.
package i18n

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Default is used when Accept-Language names no supported language.
const Default = "en"

// Supported lists site languages in display order.
var Supported = []string{"tr", "en"}

var catalog = map[string]map[string]string{
	"tr": {
		"admin.login.title":        "Giriş yap",
		"admin.login.pitch":        "Yaz, yayınla, kurcala.",
		"admin.login.lede":         "Yönetim erişimi yalnızca şu alan adındaki Google Workspace hesaplarına açık:",
		"admin.login.cta":          "Google ile devam et",
		"admin.login.loading":      "giriş yapılıyor…",
		"admin.login.denied":       "hesabına izin verilmiyor. @halid.org hesabıyla giriş yapın.",
		"admin.login.failed":       "Giriş tamamlanamadı. Lütfen tekrar deneyin.",
		"admin.login.note_hd":      "hd=halid.org sunucu tarafında doğrulanır",
		"admin.login.note_session": "oturum 12 saat · yayın ve silmede yeniden doğrulama",
		"admin.login.note_2sv":     "Google ile 2 adımlı doğrulama",
		"admin.login.back":         "← halid.org'a dön",
		"admin.login.deploy":       "deploy",
		"admin.home.title":         "Yönetim",
		"admin.home.signed_in":     "Giriş yapılan hesap:",
		"admin.logout":             "Çıkış yap",
		"footer.admin":             "admin",
		"nav.label":                "Ana menü",
		"nav.menu":                 "menü",
		"nav.more":                 "diğer",
		"nav.posts":                "yazılar",
		"feed.posts":               "halid.org · yazılar",
		"home.eyebrow":             "mühendislik defteri",
		"home.title1":              "Web için",
		"home.title2":              "üretiyor",
		"home.title3":              "ve",
		"home.title4":              "homelab’de kurcalıyorum",
		"home.sub":                 "Yazılar, notlar, snippetler ve yapım günlükleri. Çeviriye vakit olduğunda Türkçe ve İngilizce.",
		"home.cta_posts":           "yazılar",
		"home.featured":            "öne çıkan",
		"home.latest":              "Son yazılar",
		"home.all":                 "tüm yazılar",
		"home.stream":              "Akış",
		"home.empty":               "Henüz yayınlanmış yazı yok.",
		"stream.all":               "hepsi",
		"list.posts":               "Yazılar",
		"list.tag":                 "etiket",
		"list.all_posts":           "tüm yazılar",
		"list.empty":               "Burada henüz yazı yok.",
		"post.contents":            "içindekiler",
		"post.minutes":             "%d dk okuma",
		"post.min_short":           "%d dk",
		"post.series":              "seri",
		"post.not_yet":             "henüz yok",
		"post.untranslated":        "Bu yazı henüz Türkçeye çevrilmedi. Özgün sürümü aşağıda.",
		"post.read_original":       "özgün sürüm",
		"post.backlinks":           "geri bağlantılar",
		"post.backlinks_empty":     "Henüz bu yazıya bağlantı veren bir sayfa yok.",
		"error.404.title":          "Sayfa bulunamadı",
		"error.404.body":           "Aradığın adres burada değil. Taşınmış ya da hiç var olmamış olabilir.",
		"error.410.title":          "Bu sayfa kaldırıldı",
		"error.410.body":           "Bu içerik bilerek yayından kaldırıldı ve geri gelmeyecek.",
		"error.home":               "ana sayfa",
		"nav.projects":             "projeler",
		"nav.homelab":              "homelab",
		"nav.presentations":        "sunumlar",
		"nav.photos":               "fotoğraflar",
		"nav.radar":                "radar",
		"nav.now":                  "şimdi",
		"nav.about":                "hakkımda",
		"nav.notes":                "notlar",
		"nav.snippets":             "snippetler",
		"nav.links":                "bağlantılar",
		"nav.favs":                 "favoriler",
		"nav.certificates":         "sertifikalar",
		"search.label":             "ara",
		"wordmark.home":            "halid ana sayfa",
		"hello.title":              "Merhaba",
		"hello.body":               "Bu sayfa Go tarafından render edildi ve edge'de önbelleğe alındı.",
		"lang.tr":                  "Türkçe",
		"lang.en":                  "English",
		"lang.switch":              "Dil",
	},
	"en": {
		"admin.login.title":        "Sign in",
		"admin.login.pitch":        "Write, publish, tinker.",
		"admin.login.lede":         "Admin access is limited to Google Workspace accounts on",
		"admin.login.cta":          "Continue with Google",
		"admin.login.loading":      "signing in…",
		"admin.login.denied":       "is not allowed. Sign in with an @halid.org account.",
		"admin.login.failed":       "Sign-in did not complete. Try again.",
		"admin.login.note_hd":      "hd=halid.org enforced server-side",
		"admin.login.note_session": "session 12h · re-auth for publish & delete",
		"admin.login.note_2sv":     "2-step verification via Google",
		"admin.login.back":         "← back to halid.org",
		"admin.login.deploy":       "deploy",
		"admin.home.title":         "Admin",
		"admin.home.signed_in":     "Signed in as",
		"admin.logout":             "Sign out",
		"footer.admin":             "admin",
		"nav.label":                "Main",
		"nav.menu":                 "menu",
		"nav.more":                 "more",
		"nav.posts":                "posts",
		"feed.posts":               "halid.org · posts",
		"home.eyebrow":             "engineering notebook",
		"home.title1":              "Building for the",
		"home.title2":              "web",
		"home.title3":              "and tinkering in the",
		"home.title4":              "homelab",
		"home.sub":                 "Posts, notes, snippets and build logs. Published in Turkish and English when there is time to translate.",
		"home.cta_posts":           "posts",
		"home.featured":            "featured",
		"home.latest":              "Latest posts",
		"home.all":                 "all posts",
		"home.stream":              "Stream",
		"home.empty":               "No posts published yet.",
		"stream.all":               "all",
		"list.posts":               "Posts",
		"list.tag":                 "tag",
		"list.all_posts":           "all posts",
		"list.empty":               "No posts here yet.",
		"post.contents":            "contents",
		"post.minutes":             "%d min read",
		"post.min_short":           "%d min",
		"post.series":              "series",
		"post.not_yet":             "not yet",
		"post.untranslated":        "This post has not been translated into English yet. The original is below.",
		"post.read_original":       "original",
		"post.backlinks":           "backlinks",
		"post.backlinks_empty":     "No pages link here yet.",
		"error.404.title":          "Page not found",
		"error.404.body":           "Nothing lives at this address. It may have moved, or never existed.",
		"error.410.title":          "This page was removed",
		"error.410.body":           "This content was taken down on purpose and is not coming back.",
		"error.home":               "home",
		"nav.projects":             "projects",
		"nav.homelab":              "homelab",
		"nav.presentations":        "presentations",
		"nav.photos":               "photos",
		"nav.radar":                "radar",
		"nav.now":                  "now",
		"nav.about":                "about",
		"nav.notes":                "notes",
		"nav.snippets":             "snippets",
		"nav.links":                "links",
		"nav.favs":                 "favs",
		"nav.certificates":         "certificates",
		"search.label":             "search",
		"wordmark.home":            "halid home",
		"hello.title":              "Hello",
		"hello.body":               "This page was rendered by Go and cached at the edge.",
		"lang.tr":                  "Türkçe",
		"lang.en":                  "English",
		"lang.switch":              "Language",
	},
}

// IsSupported reports whether lang has a catalog.
func IsSupported(lang string) bool {
	_, ok := catalog[lang]
	return ok
}

// T returns the string for key in lang. A missing key returns the key itself,
// so gaps are visible on the page instead of rendering empty.
func T(lang, key string) string {
	if s, ok := catalog[lang][key]; ok {
		return s
	}
	return key
}

// Negotiate picks the best supported language from an Accept-Language header.
func Negotiate(header string) string {
	best, bestQ := Default, -1.0
	for _, part := range strings.Split(header, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
		if !IsSupported(base) {
			continue
		}
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				q = f
			}
		}
		if q > bestQ {
			best, bestQ = base, q
		}
	}
	return best
}

// Cookie holds an explicit language choice from the tr/en switcher. It wins
// over Accept-Language, so a choice sticks across visits.
const Cookie = "lang"

// FromRequest picks the language for a page without a language in its URL:
// the switcher cookie if valid, else the browser's Accept-Language.
func FromRequest(r *http.Request) string {
	if c, err := r.Cookie(Cookie); err == nil && IsSupported(c.Value) {
		return c.Value
	}
	return Negotiate(r.Header.Get("Accept-Language"))
}

// Option is one segment of the tr/en switcher.
type Option struct {
	Lang   string
	Href   string
	Active bool
}

// Switcher builds the switcher for a page in lang. target maps a language to
// the page to land on: its translation for public pages, the same path for admin.
func Switcher(lang string, target func(string) string) []Option {
	opts := make([]Option, 0, len(Supported))
	for _, l := range Supported {
		href := "/lang/" + l + "?next=" + url.QueryEscape(target(l))
		opts = append(opts, Option{Lang: l, Href: href, Active: l == lang})
	}
	return opts
}
