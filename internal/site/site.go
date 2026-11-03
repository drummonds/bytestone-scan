// Package site asks a published site, i.bytestone.uk or i.bytestone.org,
// whether it has an item's page. life-weaver builds the page at
// /<home>/items/<id>/, ids in their URL form; its task publish sends the
// site to i.bytestone.uk and task promote copies that over
// i.bytestone.org, so an item reaches the second site after the first.
package site

import (
	"fmt"
	"net/http"
	"time"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Site is a published site: its name, as shown, and the base URL of its
// pages, without a trailing slash.
type Site struct {
	Name string
	URL  string
}

// Status is a site's answer for one item.
type Status string

const (
	Published   Status = "published"   // the page is there
	NotYet      Status = "not yet"     // the site answers, but has no such page
	Unreachable Status = "unreachable" // the site cannot be asked, or answers with an error
)

// Defaults are the sites in the order items reach them.
func Defaults() []Site {
	return []Site{{Name: "i.bytestone.uk", URL: "https://i.bytestone.uk"}, {Name: "i.bytestone.org", URL: "https://i.bytestone.org"}}
}

// Item is the URL of the item's page on the site, under its home person.
func (s Site) Item(home, id pid.ID) string {
	return fmt.Sprintf("%s/%s/items/%s/", s.URL, home, id)
}

var client = &http.Client{Timeout: 5 * time.Second}

// Status asks the site for the item's page, by a HEAD; nothing is
// downloaded.
func (s Site) Status(home, id pid.ID) Status {
	resp, err := client.Head(s.Item(home, id))
	if err != nil {
		return Unreachable
	}
	resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return Published
	case resp.StatusCode == http.StatusNotFound:
		return NotYet
	default:
		return Unreachable
	}
}
