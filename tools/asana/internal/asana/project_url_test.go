package asana

import "testing"

func TestProjectURLs(t *testing.T) {
	for _, value := range []string{
		"https://app.asana.com/1/100/project/200",
		"https://app.asana.com/1/100/project/200/",
		"https://app.asana.com/1/100/project/200/list",
		"https://app.asana.com/1/100/project/200/board",
		"https://app.asana.com/1/100/project/200?view=board",
		"https://app.asana.com/1/100/project/200/list?view=list",
		"https://app.asana.com/1/100/project/200/board?view=board",
		"https://app.asana.com/0/200",
		"https://app.asana.com/0/200/",
		"https://app.asana.com/0/200/list",
		"https://app.asana.com/0/200/300?focus=true",
	} {
		t.Run(value, func(t *testing.T) {
			if gid, err := ParseProjectURL(value); err != nil || gid != "200" {
				t.Errorf("gid = %q, err = %v", gid, err)
			}
		})
	}
	for _, value := range []string{
		"", "200", "work", "https://app.asana.com", "/0/200/list",
		"http://app.asana.com/0/200/list", "https://example.com/0/200/list",
		"https://app.asana.com.evil.test/0/200/list", "https://app.asana.com:443/0/200/list",
		"https://user@app.asana.com/0/200/list", "https://app.asana.com/0/not-a-gid/list",
		"https://app.asana.com/0//list", "https://app.asana.com/0/200x/list",
		"https://app.asana.com/1/100/project/", "https://app.asana.com/1/100/project/abc",
		"https://app.asana.com/1/workspace/project/200", "https://app.asana.com/1/100/task/200",
		"https://app.asana.com/1/100/project/200/timeline", "https://app.asana.com/1/100/project/200/list/300",
		"https://app.asana.com/1/100/project/200#task", "https://app.asana.com/%zz",
	} {
		t.Run(value, func(t *testing.T) {
			if gid, err := ParseProjectURL(value); err == nil {
				t.Errorf("accepted %q as %q", value, gid)
			}
		})
	}
}
