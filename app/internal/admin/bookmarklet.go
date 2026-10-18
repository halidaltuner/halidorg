package admin

import (
	"html/template"
	"os"
	"strings"
)

// Bookmarklet opens Quick Capture (/admin/quick) for the current tab in a
// phone-sized window, with the tab's URL and title. Built from
// PUBLIC_ORIGIN only, never from request data, so it is safe as a URL.
func Bookmarklet() template.URL {
	origin := strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/")
	if origin == "" {
		origin = "http://localhost:8787"
	}
	js := `javascript:(()=>{window.open('` + origin + `/admin/quick?url='+encodeURIComponent(location.href)+'&title='+encodeURIComponent(document.title),'quick','width=420,height=780')})()`
	return template.URL(js) // a constant script with the origin, see above
}
