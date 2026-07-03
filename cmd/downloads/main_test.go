package main

import "testing"

// TestNextPageURL covers the GitHub Link-header parsing that drives pagination:
// a header with a rel="next" link yields that URL, and a last page (no "next")
// yields "" so fetchReleases stops.
func TestNextPageURL(t *testing.T) {
	cases := []struct {
		name string
		link string
		want string
	}{
		{
			name: "next and last present",
			link: `<https://api.github.com/repos/o/r/releases?per_page=100&page=2>; rel="next", <https://api.github.com/repos/o/r/releases?per_page=100&page=5>; rel="last"`,
			want: "https://api.github.com/repos/o/r/releases?per_page=100&page=2",
		},
		{
			name: "last page has prev/first but no next",
			link: `<https://api.github.com/repos/o/r/releases?per_page=100&page=4>; rel="prev", <https://api.github.com/repos/o/r/releases?per_page=100&page=1>; rel="first"`,
			want: "",
		},
		{
			name: "empty header",
			link: "",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nextPageURL(c.link); got != c.want {
				t.Errorf("nextPageURL(%q) = %q, want %q", c.link, got, c.want)
			}
		})
	}
}
