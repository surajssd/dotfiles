package cmd

import (
	"fmt"

	"github.com/surajssd/dotfiles/clawbox/internal/registry"
	"github.com/surajssd/dotfiles/clawbox/internal/runtime"
)

// ensureLatestImage compares the local image digest with the digest currently
// published in the upstream registry for the same tag. If they differ (or the
// image is not present locally) it pulls a fresh copy.
//
// Failures are never fatal: clawbox recreate must keep working even when the
// registry is unreachable. In that case we print a warning and leave whatever
// image is already on disk in place.
func ensureLatestImage(rt runtime.Runtime, image string) {
	if image == "" {
		return
	}

	local, _ := rt.LocalImageDigest(image)

	remote, err := registry.RemoteDigest(image)
	if err != nil {
		fmt.Printf("⚠️  could not reach registry after retries, proceeding with local image: %v\n", err)
		return
	}

	if local != "" && local == remote {
		fmt.Printf("✅ Image %s is up to date (%s)\n", image, shortDigest(remote))
		return
	}

	if local == "" {
		fmt.Printf("⏳ Image %s not present locally, pulling %s\n", image, shortDigest(remote))
	} else {
		fmt.Printf("⏳ Image %s has changed upstream (local=%s remote=%s), pulling\n",
			image, shortDigest(local), shortDigest(remote))
	}
	if err := rt.PullImage(image); err != nil {
		fmt.Printf("⚠️  pull failed, proceeding with existing local image: %v\n", err)
	}
}

// shortDigest trims a "sha256:..." digest to a 12-char prefix for display.
func shortDigest(d string) string {
	const prefix = "sha256:"
	if len(d) <= len(prefix) {
		return d
	}
	body := d[len(prefix):]
	if len(body) > 12 {
		body = body[:12]
	}
	return prefix + body
}
