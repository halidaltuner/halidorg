package render

import (
	"strings"
	"testing"
)

func TestYouTubeID(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ":        "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ?t=42":                  "dQw4w9WgXcQ",
		"https://m.youtube.com/shorts/dQw4w9WgXcQ":           "dQw4w9WgXcQ",
		"https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=short":              "",
		"https://example.com/watch?v=dQw4w9WgXcQ":            "",
		"javascript:alert(1)":                                "",
	} {
		if got := YouTubeID(in); got != want {
			t.Errorf("YouTubeID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmbeds(t *testing.T) {
	src := "Intro.\n\nhttps://www.youtube.com/watch?v=dQw4w9WgXcQ\n\n[My CV](/media/2026/10/cv.pdf)\n\n" +
		"See https://youtu.be/dQw4w9WgXcQ inline and [the CV](/media/2026/10/cv.pdf) too.\n"
	out, err := Markdown([]byte(src), Options{Lang: "tr"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.HTML, "youtube-nocookie.com/embed/dQw4w9WgXcQ") != 1 || !strings.Contains(out.HTML, `loading="lazy"`) {
		t.Errorf("video embed: %s", out.HTML)
	}
	if strings.Count(out.HTML, `class="file-card"`) != 1 || !strings.Contains(out.HTML, `<span class="file-name">My CV</span>`) || !strings.Contains(out.HTML, "indir ↓") {
		t.Errorf("file card: %s", out.HTML)
	}
	if !strings.Contains(out.HTML, `<a href="https://youtu.be/dQw4w9WgXcQ">`) || !strings.Contains(out.HTML, `<a href="/media/2026/10/cv.pdf">the CV</a>`) {
		t.Errorf("inline links must stay links: %s", out.HTML)
	}
}

func TestFenceWithoutLanguageIsGuessed(t *testing.T) {
	out, err := Markdown([]byte("```\n#!/bin/bash\necho \"hi\"\n```\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.HTML, `<span class="s2">`) && !strings.Contains(out.HTML, `<span class="s">`) {
		t.Errorf("no string token: %s", out.HTML)
	}
}
