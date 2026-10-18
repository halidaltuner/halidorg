package i18n

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFromRequest(t *testing.T) {
	cases := []struct {
		name, cookie, accept, want string
	}{
		{"browser language", "", "tr-TR,tr;q=0.9", "tr"},
		{"cookie beats browser", "en", "tr-TR,tr;q=0.9", "en"},
		{"unknown cookie ignored", "de", "tr-TR", "tr"},
		{"nothing set", "", "", Default},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/admin/login", nil)
		r.Header.Set("Accept-Language", tc.accept)
		if tc.cookie != "" {
			r.AddCookie(&http.Cookie{Name: Cookie, Value: tc.cookie})
		}
		if got := FromRequest(r); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSwitcher(t *testing.T) {
	opts := Switcher("en", func(l string) string { return "/" + l + "/" })
	if len(opts) != 2 || opts[0].Lang != "tr" || opts[0].Active || !opts[1].Active {
		t.Fatalf("got %+v", opts)
	}
	if opts[0].Href != "/lang/tr?next=%2Ftr%2F" {
		t.Errorf("href %q", opts[0].Href)
	}
}

// TestCatalogsMatch: every key exists in every language, so no page shows a
// raw key in one language only.
func TestCatalogsMatch(t *testing.T) {
	for _, a := range Supported {
		for k := range catalog[a] {
			for _, b := range Supported {
				if _, ok := catalog[b][k]; !ok {
					t.Errorf("%q is in %s but not in %s", k, a, b)
				}
			}
		}
	}
}
