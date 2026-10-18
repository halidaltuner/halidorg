package web

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
)

var assetURLs sync.Map // path -> versioned URL

// Asset returns the URL of a file under /static/ with a hash of its
// content as ?v=, so a changed CSS or JS file gets a new URL and browsers
// never pair new HTML with an old stylesheet. Files that do not exist keep
// their path.
func Asset(path string) string {
	if u, ok := assetURLs.Load(path); ok {
		return u.(string)
	}
	u := path
	if b, err := Static.ReadFile(strings.TrimPrefix(path, "/")); err == nil {
		sum := sha256.Sum256(b)
		u = path + "?v=" + hex.EncodeToString(sum[:5])
	}
	assetURLs.Store(path, u)
	return u
}
